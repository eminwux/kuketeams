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

package compose

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"text/template"

	model "github.com/eminwux/kukeon/pkg/api/model/kuketeams"
	api "github.com/eminwux/kukeon/pkg/api/model/v1beta1"
	"github.com/eminwux/kuketeams/pkg/config"
)

func checkAssets(source fs.FS, rolePath string, role *model.Role, settings model.RoleHarness, harness string) error {
	for _, name := range append([]string{path.Join(rolePath, "AGENTS.md")}, role.Spec.Skills...) {
		if !fs.ValidPath(name) {
			return errors.New("role asset: require source-relative paths")
		}
		file := name
		if name != path.Join(rolePath, "AGENTS.md") {
			file = path.Join(name, "SKILL.md")
		}
		if _, err := fs.Stat(source, file); err != nil {
			return fmt.Errorf("missing role asset %s: %w", file, err)
		}
	}
	if settings.Settings != "" {
		if !fs.ValidPath(settings.Settings) || !config.SafePath("/"+settings.Settings) {
			return errors.New("harness settings: require a safe role-relative path")
		}
		if _, err := fs.Stat(source, path.Join(rolePath, settings.Settings)); err != nil {
			return errors.New("missing selected role harness settings file")
		}
	}
	if harness == "opencode" {
		raw, readErr := fs.ReadFile(source, path.Join(rolePath, "config/opencode.json"))
		if readErr != nil || !json.Valid(raw) {
			return errors.New("opencode requires a valid role config/opencode.json")
		}
	}
	for _, mount := range role.Spec.Needs.Mounts {
		if !slices.Contains([]string{"ssh", "kukeond-socket", "kuke-binary", "kukeon-install"}, mount) {
			return fmt.Errorf("unsupported mount selector %q", mount)
		}
	}
	return nil
}

func checkSettings(h string, s model.RoleHarness) error {
	switch h {
	case "codex":
		if s.Sandbox != "" &&
			!slices.Contains([]string{"read-only", "workspace-write", "danger-full-access"}, s.Sandbox) {
			return errors.New("unsupported Codex sandbox setting")
		}
		if s.Approval != "" &&
			!slices.Contains([]string{"untrusted", "on-failure", "on-request", "never"}, s.Approval) {
			return errors.New("unsupported Codex approval setting")
		}
	case "claude":
		if s.Settings == "" {
			return errors.New("claude requires a role-relative settings path")
		}
	case "opencode":
		if s.Permissions != "" && !slices.Contains([]string{"ask", "skip"}, s.Permissions) {
			return errors.New("unsupported OpenCode permissions setting")
		}
	}
	return nil
}

func render(source fs.FS, hpath string, h *model.Harness, role *model.Role, selection config.Role,
	repo config.Repository, image model.ImageCatalogEntry, needs []string, cfg *config.TeamsConfig, opts Options,
) (*api.CellBlueprintDoc, error) {
	if !fs.ValidPath(h.Spec.Template) {
		return nil, errors.New("harness template: require a source-relative filename")
	}
	filename := path.Join(hpath, h.Spec.Template)
	files, globErr := fs.Glob(source, path.Join(path.Dir(filename), "*.tmpl.yaml"))
	if globErr != nil {
		return nil, fmt.Errorf("list template partials: %w", globErr)
	}
	// Include the declared entry even if it has a different supported filename.
	if !slices.Contains(files, filename) {
		files = append(files, filename)
		slices.Sort(files)
	}
	funcs := template.FuncMap{
		"upper":   strings.ToUpper,
		"replace": func(old, newValue, value string) string { return strings.ReplaceAll(value, old, newValue) },
	}
	tpl := template.New(path.Base(filename)).Funcs(funcs).Option("missingkey=error")
	for _, file := range files {
		raw, err := fs.ReadFile(source, file)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", file, err)
		}
		if _, parseErr := tpl.New(path.Base(file)).Parse(string(raw)); parseErr != nil {
			return nil, fmt.Errorf("parse template %s: %w", file, parseErr)
		}
	}
	settings := role.Spec.Harnesses[h.Metadata.Name]
	secretNames := union(role.Spec.Needs.Secrets, settings.Secrets)
	sourceRepos := union(role.Spec.Needs.Repos, []string{selection.Source})
	for _, name := range sourceRepos {
		if _, ok := repository(cfg, name); !ok {
			return nil, fmt.Errorf("spec.repositories: missing explicit repository binding %q", name)
		}
	}
	project := map[string]string{"PROJECT_DIR": "", "CLONE_DIR": "", "NAME": "", "AGENTS_REPO": "", "X": ""}
	if slices.Contains(role.Spec.Needs.Repos, "project") {
		r, _ := repository(cfg, "project")
		project["PROJECT_DIR"] = r.Target
		project["CLONE_DIR"] = r.Target
		project["NAME"] = path.Base(r.Target)
	}
	for _, mount := range role.Spec.Needs.Mounts {
		if mount == "ssh" && !config.SafePath(opts.HomeDir) {
			return nil, errors.New("HomeDir: explicit absolute path required by ssh mount")
		}
	}
	if !fs.ValidPath(selection.Path) || !config.SafePath("/"+selection.Path) {
		return nil, errors.New("role path: require a safe source-relative path")
	}
	ctx := map[string]any{
		"realm": opts.Scope.Realm, "space": opts.Scope.Space, "stack": opts.Scope.Stack, "image": image.Image,
		"role":       map[string]any{"name": role.Metadata.Name, "path": selection.Path, "sourceDir": repo.Target},
		"makeTarget": h.Spec.MakeTarget,
		"needs": map[string]any{
			"image":  needs,
			"repos":  sourceRepos,
			"mounts": role.Spec.Needs.Mounts,
			"params": role.Spec.Needs.Params,
		},
		"harnesses": map[string]any{h.Metadata.Name: map[string]any{
			"settings": settings.Settings, "sandbox": settings.Sandbox,
			"approval": settings.Approval, "permissions": settings.Permissions, "secrets": secretNames,
		}},
		"project": project, "operator": map[string]string{"HOME_DIR": opts.HomeDir, "TEAM_ROOT": opts.TeamDir, "X": "", "GIT_USER_NAME": "", "GIT_USER_EMAIL": ""},
	}
	var output bytes.Buffer
	if err := tpl.ExecuteTemplate(&output, path.Base(filename), ctx); err != nil {
		return nil, fmt.Errorf("execute template %s: %w", filename, err)
	}
	var bp api.CellBlueprintDoc
	if err := decode(output.Bytes(), filename, &bp); err != nil {
		return nil, err
	}
	if bp.APIVersion != api.APIVersionV1Beta1 || bp.Kind != api.KindCellBlueprint || len(bp.Spec.Cell.Containers) == 0 {
		return nil, errors.New("template must render a v1beta1 CellBlueprint with containers")
	}
	return &bp, nil
}
