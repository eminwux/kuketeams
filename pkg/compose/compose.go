// Copyright 2026 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

// Package compose produces team-owned kukeon documents without contacting a
// daemon, fetching sources, reading credentials, or writing files.
package compose

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/distribution/reference"
	model "github.com/eminwux/kukeon/pkg/api/model/kuketeams"
	api "github.com/eminwux/kukeon/pkg/api/model/v1beta1"
	"github.com/eminwux/kuketeams/pkg/config"
	"gopkg.in/yaml.v3"
)

// Options supplies materialized definition sources. Each key is a selected
// source name; callers own fetching/pinning. Composition only reads these FSes.
type Options struct {
	Sources map[string]fs.FS
	Scope   Scope
	// HomeDir is needed only for an explicitly requested ssh mount; TeamDir is
	// needed for harness seed paths (Claude). Neither is discovered from the host.
	HomeDir string
	TeamDir string
}

const defaultScope = "default"

type Scope struct{ Realm, Space, Stack string }

// Result keeps the authored declaration separate from resolved resources.
// Entry is a candidate for #13 to persist only after successful application.
type Result struct {
	Entry *config.TeamEntry
	Pairs []Pair
}

type Pair struct {
	Role      string
	Harness   string
	Image     model.ImageCatalogEntry
	Blueprint *api.CellBlueprintDoc
	Config    *api.CellConfigDoc
	Secrets   []SecretRequirement
	Seeds     []model.HarnessSeed
}

// SecretRequirement contains declarations only; Source is an optional creation
// fallback to read at application time, after runtime existence is checked.
type SecretRequirement struct {
	Identifier  string
	Container   string
	Destination api.BlueprintSecretSlot
	Reference   config.SecretRef
	Source      *config.SecretSource
}

// Compose resolves whole-list inheritance and selected role/harness pairs. The
// returned objects are independent of inputs and ordered by authored selections.
func Compose(cfg *config.TeamsConfig, team *config.Team, opts Options) (*Result, error) {
	effective, err := config.Resolve(cfg, team)
	if err != nil {
		return nil, err
	}
	scope, err := normalizeScope(opts.Scope)
	if err != nil {
		return nil, err
	}
	opts.Scope = scope
	entry, err := config.Entry(team)
	if err != nil {
		return nil, err
	}
	result := &Result{Entry: entry}
	for _, role := range effective.Roles {
		if !config.ValidName(role.Name) {
			return nil, errors.New("role: invalid selection name")
		}
		for _, h := range effective.Harnesses {
			if (role.Harness != "" && role.Harness != h.Name) || (role.Harness == "" && !h.Default) {
				continue
			}
			pair, pairErr := composePair(cfg, team.Metadata.Name, role, h, effective.Sources, opts)
			if pairErr != nil {
				return nil, fmt.Errorf("compose %s/%s: %w", role.Name, h.Name, pairErr)
			}
			result.Pairs = append(result.Pairs, pair)
		}
	}
	parameters := map[string]bool{}
	for _, pair := range result.Pairs {
		for _, param := range pair.Blueprint.Spec.Parameters {
			parameters[param.Name] = true
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Spec.Parameters)) {
		if !parameters[name] {
			return nil, fmt.Errorf("spec.parameters: unsupported parameter %q", name)
		}
	}
	return result, nil
}

func normalizeScope(s Scope) (Scope, error) {
	if s.Realm == "" {
		s.Realm = defaultScope
	}
	if s.Space == "" {
		s.Space = defaultScope
	}
	if s.Stack == "" {
		s.Stack = defaultScope
	}
	if !config.ValidName(s.Realm) || !config.ValidName(s.Space) || !config.ValidName(s.Stack) {
		return Scope{}, errors.New("scope: require valid realm, space, and stack names")
	}
	return s, nil
}

func composePair(
	cfg *config.TeamsConfig,
	team string,
	selection config.Role,
	hsel config.Harness,
	sources []config.Source,
	opts Options,
) (Pair, error) {
	if !config.ValidName(hsel.Name) {
		return Pair{}, errors.New("invalid harness selection name")
	}
	rfs := opts.Sources[selection.Source]
	hfs := opts.Sources[hsel.Source]
	if rfs == nil || hfs == nil {
		return Pair{}, errors.New("missing materialized role or harness source")
	}
	var role model.Role
	if err := load(rfs, path.Join(selection.Path, "role.yaml"), &role); err != nil {
		return Pair{}, err
	}
	if role.APIVersion != config.APIVersion || role.Kind != "Role" || !config.ValidName(role.Metadata.Name) {
		return Pair{}, errors.New("role.yaml: require kuketeams.io/v1 Role and valid metadata.name")
	}
	settings, ok := role.Spec.Harnesses[hsel.Name]
	if !ok {
		return Pair{}, errors.New("role does not support selected harness")
	}
	var harness model.Harness
	if err := load(hfs, path.Join(hsel.Path, "harness.yaml"), &harness); err != nil {
		return Pair{}, err
	}
	if harness.APIVersion != config.APIVersion || harness.Kind != "Harness" || harness.Metadata.Name != hsel.Name ||
		!config.ValidName(harness.Spec.MakeTarget) || harness.Spec.SkillPath == "" {
		return Pair{}, errors.New("harness.yaml: invalid header, makeTarget, or skillPath")
	}
	if err := checkAssets(rfs, selection.Path, &role, settings, hsel.Name); err != nil {
		return Pair{}, err
	}
	var catalog model.ImageCatalog
	if err := load(hfs, "harnesses/images.yaml", &catalog); err != nil {
		return Pair{}, err
	}
	var extra []string
	if selection.Needs != nil {
		extra = selection.Needs.Image
	}
	needs := union(role.Spec.Needs.Image, extra)
	image, err := SelectImage(&catalog, hsel.Name, needs)
	if err != nil {
		return Pair{}, err
	}
	repo, ok := runtimeSource(sources, selection.Source)
	if !ok {
		return Pair{}, fmt.Errorf("sources: missing clonePath for required role source %q", selection.Source)
	}
	if settingsErr := checkSettings(hsel.Name, settings); settingsErr != nil {
		return Pair{}, settingsErr
	}
	bp, err := render(hfs, hsel.Path, &harness, &role, selection, repo, image, needs, sources, opts)
	if err != nil {
		return Pair{}, err
	}
	name := resourceName(team, selection.Name, hsel.Name)
	bp.Metadata = api.CellBlueprintMetadata{
		Name: name, Realm: opts.Scope.Realm, Space: opts.Scope.Space, Stack: opts.Scope.Stack,
		Labels: map[string]string{api.LabelTeam: team},
	}
	bp.Spec.Prefix = name
	cc := &api.CellConfigDoc{
		APIVersion: api.APIVersionV1Beta1, Kind: api.KindCellConfig,
		Metadata: api.CellConfigMetadata{
			Name: name, Realm: opts.Scope.Realm, Space: opts.Scope.Space, Stack: opts.Scope.Stack,
			Labels: map[string]string{api.LabelTeam: team},
		},
		Spec: api.CellConfigSpec{
			Blueprint: api.CellConfigBlueprintRef{
				Name:  name,
				Realm: opts.Scope.Realm,
				Space: opts.Scope.Space,
				Stack: opts.Scope.Stack,
			},
		},
	}
	pair := Pair{Role: selection.Name, Harness: hsel.Name, Image: image, Blueprint: bp, Config: cc}
	if bindErr := bind(&pair, cfg, selection, &role, sources, team); bindErr != nil {
		return Pair{}, bindErr
	}
	pair.Seeds, err = resolveSeeds(&harness, hsel.Name, opts.TeamDir)
	if err != nil {
		return Pair{}, err
	}

	return pair, nil
}

// resourceName escapes hyphens within components before joining them with single
// hyphens. Validated team/role/harness names start and end in alphanumerics, so
// component separators cannot be confused with escaped runs inside a name.
func resourceName(parts ...string) string {
	for i, part := range parts {
		parts[i] = strings.ReplaceAll(part, "-", "--")
	}
	return strings.Join(parts, "-")
}

func load(source fs.FS, filename string, out any) error {
	raw, err := fs.ReadFile(source, path.Clean(filename))
	if err != nil {
		return fmt.Errorf("read definition %s: %w", filename, err)
	}
	return decode(raw, filename, out)
}

func decode(raw []byte, name string, out any) error {
	d := yaml.NewDecoder(bytes.NewReader(raw))
	d.KnownFields(true)
	if err := d.Decode(out); err != nil {
		return fmt.Errorf("%s: invalid document: %w", name, err)
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: expected exactly one YAML document", name)
	}
	return nil
}

// runtimeSource resolves only explicitly requested clones from effective selections.
func runtimeSource(sources []config.Source, name string) (config.Source, bool) {
	for _, source := range sources {
		if source.Name == name && source.ClonePath != "" {
			return source, true
		}
	}
	return config.Source{}, false
}

// SelectImage preserves catalog order and requires one published image to carry
// the selected harness plus all mandatory and additive capabilities.
func SelectImage(c *model.ImageCatalog, harness string, needs []string) (model.ImageCatalogEntry, error) {
	if c == nil || c.APIVersion != config.APIVersion || c.Kind != "ImageCatalog" {
		return model.ImageCatalogEntry{}, errors.New("invalid ImageCatalog header")
	}
	required := union(needs, []string{harness})
	for _, image := range c.Spec.Images {
		if image.Harness != harness ||
			(slices.Contains(image.Capabilities, "root") && !slices.Contains(needs, "root")) {
			continue
		}
		if !slices.ContainsFunc(required, func(s string) bool { return !slices.Contains(image.Capabilities, s) }) {
			if !published(image.Image) {
				return model.ImageCatalogEntry{}, errors.New(
					"matching catalog image requires a registry-qualified published reference",
				)
			}
			return image, nil
		}
	}
	return model.ImageCatalogEntry{}, fmt.Errorf(
		"no published image for harness %q with all capabilities %v; update harnesses/images.yaml",
		harness,
		required,
	)
}

func published(ref string) bool {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return false
	}
	// Require an authored registry-qualified reference, not Docker's implicit
	// docker.io/library expansion or an unversioned local image name.
	if !strings.Contains(ref, "/") || !strings.HasPrefix(ref, reference.Domain(named)+"/") {
		return false
	}
	_, tagged := named.(reference.Tagged)
	_, pinned := named.(reference.Canonical)
	return tagged || pinned
}

func union(a, b []string) []string {
	out := append(append([]string{}, a...), b...)
	slices.Sort(out)
	return slices.Compact(out)
}

// MarshalYAML returns only Blueprints then Configs. Secret values and creation
// source declarations are never part of the daemon document stream.
func (r *Result) MarshalYAML() ([]byte, error) {
	if r == nil {
		return nil, errors.New("nil composition result")
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	const yamlIndent = 2
	enc.SetIndent(yamlIndent)
	for _, p := range r.Pairs {
		if err := enc.Encode(p.Blueprint); err != nil {
			return nil, err
		}
	}
	for _, p := range r.Pairs {
		if err := enc.Encode(p.Config); err != nil {
			return nil, err
		}
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func resolveSeeds(harness *model.Harness, harnessName, teamDir string) ([]model.HarnessSeed, error) {
	var out []model.HarnessSeed
	for _, seed := range harness.Spec.Seeds {
		if !config.SafePath(teamDir) {
			return nil, errors.New("TeamDir: explicit absolute team-state path required by harness seeds")
		}
		seed.Path = strings.NewReplacer("${TEAM_ROOT}", teamDir, "${HARNESS}", harnessName).Replace(seed.Path)
		if !config.SafePath(seed.Path) || !strings.HasPrefix(seed.Path, teamDir+"/") {
			return nil, errors.New("harness seed escapes TeamDir")
		}
		out = append(out, seed)
	}
	return out, nil
}
