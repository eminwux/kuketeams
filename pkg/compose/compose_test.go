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

package compose_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/eminwux/kuketeams/internal/bootstrap"
	"github.com/eminwux/kuketeams/pkg/compose"
	"github.com/eminwux/kuketeams/pkg/config"
	"gopkg.in/yaml.v3"
)

type noGit struct{}

func (noGit) Lookup(context.Context, string, bool) (string, error) { return "", bootstrap.ErrMissing }

func fixture(t *testing.T) (*config.TeamsConfig, compose.Options) {
	t.Helper()
	boot, err := bootstrap.Run(t.Context(), bootstrap.Options{Root: t.TempDir(), Git: noGit{}})
	if err != nil {
		t.Fatal(err)
	}
	return boot.Config, compose.Options{
		Sources: map[string]fs.FS{"kuketeams": os.DirFS("../..")},
		TeamDir: "/operator/teams/root",
	}
}

func TestRootDefaults(t *testing.T) {
	cfg, opts := fixture(t)
	before, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	result, err := compose.Compose(cfg, config.Root(), opts)
	if err != nil {
		t.Fatal(err)
	}
	assertRootResources(t, result)
	raw, err := result.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"gh-token", "GH_TOKEN", "<no value>", "sourceDir", "secretRef:", "ROLE=${ROLE}"} {
		if strings.Contains(string(raw), absent) {
			t.Fatalf("unexpected %q in documents", absent)
		}
	}
	if bytes.Count(raw, []byte("kind: CellBlueprint")) != 1 || bytes.Count(raw, []byte("kind: CellConfig")) != 1 {
		t.Fatal("incorrect document stream")
	}
	again, err := compose.Compose(cfg, config.Root(), opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := again.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, second) {
		t.Fatal("nondeterministic rendering")
	}
	after, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || result.Entry.Spec.Roles != nil {
		t.Fatal("composition mutated/materialized authored selection")
	}
}

func allHarnesses(cfg *config.TeamsConfig) {
	cfg.Spec.Defaults.Harnesses = []config.Harness{
		{Name: "codex", Source: "kuketeams", Path: "harnesses/codex", Default: true},
		{Name: "claude", Source: "kuketeams", Path: "harnesses/claude", Default: true},
		{Name: "opencode", Source: "kuketeams", Path: "harnesses/opencode", Default: true},
	}
}

func routerBinding() config.SecretBinding {
	return config.SecretBinding{
		Name: "openrouter-api-key", EnvName: "OPENROUTER_API_KEY",
		SecretRef: config.SecretRef{Name: "router-account", Realm: "auth", Space: "shared", Stack: "agents"},
	}
}

func TestSelectedHarnessesAndBindings(t *testing.T) {
	cfg, opts := fixture(t)
	allHarnesses(cfg)
	cfg.Spec.Secrets = []config.SecretBinding{routerBinding()}
	// There is deliberately no local source and no environment lookup. An existing
	// scoped secret reference is enough for structural composition.
	result, err := compose.Compose(cfg, config.Root(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pairs) != 3 {
		t.Fatal("did not render selected defaults")
	}
	claude, router := result.Pairs[1], result.Pairs[2]
	if len(claude.Seeds) != 1 || claude.Seeds[0].Path != "/operator/teams/root/claude.json" {
		t.Fatal("missing seed declaration")
	}
	if len(router.Secrets) != 1 || router.Secrets[0].Reference.Name != "router-account" ||
		router.Secrets[0].Source != nil {
		t.Fatal("invalid existing-secret requirement")
	}
	ref := router.Config.Spec.Secrets["openrouter-api-key"].SecretRef
	if ref.Name != "router-account" || ref.Realm != "auth" || ref.Space != "shared" || ref.Stack != "agents" {
		t.Fatal("secret scope lost")
	}
	if router.Blueprint.Spec.Cell.Containers[0].Secrets[0].EnvName != "OPENROUTER_API_KEY" {
		t.Fatal("incorrect destination")
	}
	script := router.Blueprint.Spec.Cell.Containers[0].Tty.OnInit[0].Script
	if strings.Contains(script, "cat >") || !strings.Contains(script, "opencode-openrouter ROLE=roles/operator") {
		t.Fatal("role settings would be overwritten or materialization missing")
	}
	cfg.Spec.Secrets = nil
	cfg.Spec.Defaults.Roles[0].Harness = "codex"
	// The explicit override must not require OpenCode auth, Claude seeds, or even
	// load the unselected definitions.
	cfg.Spec.Defaults.Harnesses[1].Path = "absent/claude"
	cfg.Spec.Defaults.Harnesses[2].Path = "absent/opencode"
	opts.TeamDir = ""
	result, err = compose.Compose(cfg, config.Root(), opts)
	if err != nil || len(result.Pairs) != 1 {
		t.Fatalf("override: %v", err)
	}
	team := config.Root()
	team.Spec.Roles = []config.Role{}
	team.Spec.Harnesses = []config.Harness{}
	result, err = compose.Compose(cfg, team, opts)
	if err != nil || len(result.Pairs) != 0 {
		t.Fatalf("empty selection: %v", err)
	}
}

func copiedSource(t *testing.T) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	err := fs.WalkDir(os.DirFS("../.."), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && name != "." && name != "harnesses" && name != "roles" &&
			!strings.HasPrefix(name, "harnesses/") &&
			!strings.HasPrefix(name, "roles/") {
			return fs.SkipDir
		}
		if !d.IsDir() {
			raw, e := os.ReadFile("../../" + name)
			if e != nil {
				return e
			}
			out[name] = &fstest.MapFile{Data: raw}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMandatoryRequirementsAndSourceDeclarations(t *testing.T) {
	cfg, opts := fixture(t)
	source := copiedSource(t)
	opts.Sources["kuketeams"] = source
	role := string(source["roles/operator/role.yaml"].Data)
	role = strings.Replace(role, "  needs:\n", "  needs:\n    secrets: [service-auth]\n    params: [REGION]\n", 1)
	source["roles/operator/role.yaml"] = &fstest.MapFile{Data: []byte(role)}
	cfg.Spec.Defaults.Roles[0].Needs = &config.Needs{Image: []string{}} // mandatory go survives
	cfg.Spec.Parameters = map[string]string{"REGION": "eu-test"}
	cfg.Spec.Secrets = []config.SecretBinding{{
		Name: "service-auth", EnvName: "SERVICE_ACCESS_TOKEN",
		SecretRef: config.SecretRef{
			Name:  "service-account",
			Realm: "auth",
		}, Source: &config.SecretSource{File: "/does/not/exist/credentials"},
	}}
	result, err := compose.Compose(cfg, config.Root(), opts)
	if err != nil {
		t.Fatal(err)
	}
	p := result.Pairs[0]
	if p.Config.Spec.Values["REGION"] != "eu-test" || p.Secrets[0].Destination.EnvName != "SERVICE_ACCESS_TOKEN" {
		t.Fatal("parameter/explicit destination not bound")
	}
	if !slices.Contains(p.Image.Capabilities, "go") {
		t.Fatal("mandatory image capability erased")
	}
	if p.Secrets[0].Source.File != "/does/not/exist/credentials" {
		t.Fatal("lost optional creation source")
	}
	cfg.Spec.Secrets[0].Source.File = "changed"
	if p.Secrets[0].Source.File == "changed" {
		t.Fatal("requirement aliases input")
	}
	raw, err := result.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("credentials")) {
		t.Fatal("source declaration persisted in daemon documents")
	}
}

func TestActionableFailures(t *testing.T) {
	cases := []struct {
		name, want string
		change     func(*config.TeamsConfig, *compose.Options, fstest.MapFS)
	}{
		{
			"role-source",
			"missing clonePath for required role source",
			func(c *config.TeamsConfig, _ *compose.Options, _ fstest.MapFS) {
				c.Spec.Defaults.Sources[0].ClonePath = ""
			},
		},
		{"capability", "no published image", func(c *config.TeamsConfig, _ *compose.Options, _ fstest.MapFS) {
			c.Spec.Defaults.Roles[0].Needs.Image = []string{"absent-capability"}
		}},
		{
			"source",
			"missing materialized",
			func(_ *config.TeamsConfig, o *compose.Options, _ fstest.MapFS) { o.Sources = nil },
		},
		{
			"role",
			"read definition",
			func(_ *config.TeamsConfig, _ *compose.Options, s fstest.MapFS) { delete(s, "roles/operator/role.yaml") },
		},
		{"partial", "not defined", func(_ *config.TeamsConfig, _ *compose.Options, s fstest.MapFS) {
			delete(s, "harnesses/codex/partials.tmpl.yaml")
		}},
		{"template", "read template", func(_ *config.TeamsConfig, _ *compose.Options, s fstest.MapFS) {
			delete(s, "harnesses/codex/blueprint.tmpl.yaml")
		}},
		{"unsupported-setting", "unsupported Codex", func(_ *config.TeamsConfig, _ *compose.Options, s fstest.MapFS) {
			s["roles/operator/role.yaml"].Data = bytes.ReplaceAll(
				s["roles/operator/role.yaml"].Data,
				[]byte("workspace-write"),
				[]byte("made-up"),
			)
		}},
		{"parameter", "missing required parameter", func(_ *config.TeamsConfig, _ *compose.Options, s fstest.MapFS) {
			s["roles/operator/role.yaml"].Data = bytes.ReplaceAll(
				s["roles/operator/role.yaml"].Data,
				[]byte("  needs:"),
				[]byte("  needs:\n    params: [REGION]"),
			)
		}},
		{"unknown-parameter", "unsupported parameter", func(c *config.TeamsConfig, _ *compose.Options, _ fstest.MapFS) {
			c.Spec.Parameters = map[string]string{"WRONG": "value"}
		}},
		{"secret", "missing structural binding", func(c *config.TeamsConfig, _ *compose.Options, _ fstest.MapFS) {
			allHarnesses(c)
			c.Spec.Defaults.Roles[0].Harness = "opencode"
		}},
		{"mount", "unsupported mount", func(_ *config.TeamsConfig, _ *compose.Options, s fstest.MapFS) {
			s["roles/operator/role.yaml"].Data = bytes.ReplaceAll(
				s["roles/operator/role.yaml"].Data,
				[]byte("  needs:"),
				[]byte("  needs:\n    mounts: [unknown]"),
			)
		}},
		{"seed", "TeamDir", func(c *config.TeamsConfig, o *compose.Options, _ fstest.MapFS) {
			allHarnesses(c)
			c.Spec.Defaults.Roles[0].Harness = "claude"
			o.TeamDir = ""
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, opts := fixture(t)
			source := copiedSource(t)
			opts.Sources["kuketeams"] = source
			tc.change(cfg, &opts, source)
			result, err := compose.Compose(cfg, config.Root(), opts)
			if err == nil || result != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("result=%v err=%v; wanted %q", result, err, tc.want)
			}
		})
	}
}

func TestEnvironmentIsNeverRead(t *testing.T) {
	cfg, opts := fixture(t)
	allHarnesses(cfg)
	cfg.Spec.Defaults.Roles[0].Harness = "opencode"
	binding := routerBinding()
	binding.Source = &config.SecretSource{Env: "LOCAL_OPERATOR_AUTH"}
	cfg.Spec.Secrets = []config.SecretBinding{binding}
	t.Setenv("LOCAL_OPERATOR_AUTH", "secret-canary-must-never-appear")
	first, err := compose.Compose(cfg, config.Root(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCAL_OPERATOR_AUTH", "")
	second, err := compose.Compose(cfg, config.Root(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("composition depends on credential value")
	}
	raw, err := first.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret-canary") {
		t.Fatal("credential leaked")
	}
}

func assertRootResources(t *testing.T, result *compose.Result) {
	t.Helper()
	if len(result.Pairs) != 1 {
		t.Fatalf("pairs = %d", len(result.Pairs))
	}
	p := result.Pairs[0]
	if p.Config.Metadata.Name != "root-operator-codex" || p.Blueprint.Metadata.Name != p.Config.Metadata.Name ||
		p.Blueprint.Spec.Prefix != p.Config.Metadata.Name || p.Config.Spec.Blueprint.Name != p.Config.Metadata.Name {
		t.Fatal("unstable identity")
	}
	if p.Image.Ref != "codex" || p.Image.Image != "ghcr.io/eminwux/kuketeams/codex:v0.1.0" {
		t.Fatalf("image = %+v", p.Image)
	}
	c := p.Blueprint.Spec.Cell.Containers[0]
	if c.Privileged || p.Blueprint.Spec.Cell.NestedCgroupRuntime || len(p.Secrets) != 0 || c.Git != nil {
		t.Fatal("operator gained privilege, secret, or Git signing requirements")
	}
	if p.Blueprint.Metadata.Labels["kukeon.io/team"] != "root" || p.Config.Metadata.Labels["kukeon.io/team"] != "root" {
		t.Fatal("missing team ownership")
	}
	if p.Config.Metadata.Realm != "default" || p.Config.Metadata.Space != "default" ||
		p.Config.Metadata.Stack != "default" {
		t.Fatal("missing scope")
	}
	if c.Volumes[0].Source != "root-operator-codex" || !c.Volumes[0].Ensure {
		t.Fatal("state volume missing team ownership/ensure")
	}
	if len(p.Config.Spec.Repos) != 1 ||
		p.Config.Spec.Repos["kuketeams"].URL != "https://github.com/eminwux/kuketeams" {
		t.Fatal("missing explicit role source")
	}
	script := c.Tty.OnInit[0].Script
	for _, want := range []string{"make -C /home/user/kuketeams codex ROLE=roles/operator", "cd /home/user/kuketeams/roles/operator", "sandbox=workspace-write", "approval=on-request"} {
		if !strings.Contains(script, want) {
			t.Fatalf("missing %q in launcher", want)
		}
	}
}

func TestRenderedLaunchScriptsAndExplicitProjectRepo(t *testing.T) {
	cfg, opts := fixture(t)
	allHarnesses(cfg)
	cfg.Spec.Secrets = []config.SecretBinding{routerBinding()}
	source := copiedSource(t)
	opts.Sources["kuketeams"] = source
	source["roles/operator/role.yaml"].Data = bytes.ReplaceAll(
		source["roles/operator/role.yaml"].Data,
		[]byte("  needs:"),
		[]byte("  needs:\n    repos: [project]"),
	)
	cfg.Spec.Defaults.Sources = append(
		cfg.Spec.Defaults.Sources,
		config.Source{
			Name:      "project",
			URL:       "https://example.test/work.git",
			ClonePath: "/home/user/work/explicit-repo",
			Tag:       "v2",
		},
	)
	opts.Scope = compose.Scope{Realm: "ops", Space: "agents", Stack: "root"}
	result, err := compose.Compose(cfg, config.Root(), opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range result.Pairs {
		if pair.Config.Spec.Repos["project"].Ref != "v2" || pair.Config.Spec.Blueprint.Realm != "ops" {
			t.Fatal("repo revision or scope lost")
		}
		var scripts strings.Builder
		for _, c := range pair.Blueprint.Spec.Cell.Containers {
			for _, stage := range c.Tty.OnInit {
				scripts.WriteString(stage.Script)
				command := exec.CommandContext(t.Context(), "bash", "-n")
				command.Stdin = strings.NewReader(stage.Script)
				if output, checkErr := command.CombinedOutput(); checkErr != nil {
					t.Fatalf("%s shell syntax: %v: %s", pair.Harness, checkErr, output)
				}
			}
		}
		if !strings.Contains(scripts.String(), "cd /home/user/work/explicit-repo") {
			t.Fatalf("%s inferred project directory", pair.Harness)
		}
	}
}

func TestSecretDestinationConflicts(t *testing.T) {
	cfg, opts := fixture(t)
	source := copiedSource(t)
	opts.Sources["kuketeams"] = source
	source["roles/operator/role.yaml"].Data = bytes.ReplaceAll(
		source["roles/operator/role.yaml"].Data,
		[]byte("  needs:"),
		[]byte("  needs:\n    secrets: [first-auth, second-auth]"),
	)
	cfg.Spec.Secrets = []config.SecretBinding{
		{Name: "first-auth", EnvName: "AUTH_TOKEN", SecretRef: config.SecretRef{Name: "a", Realm: "auth"}},
		{Name: "second-auth", EnvName: "AUTH_TOKEN", SecretRef: config.SecretRef{Name: "b", Realm: "auth"}},
	}
	result, err := compose.Compose(cfg, config.Root(), opts)
	if err == nil || result != nil || !strings.Contains(err.Error(), "conflicting secret") {
		t.Fatalf("result=%v err=%v", result, err)
	}
	cfg.Spec.Secrets = cfg.Spec.Secrets[:1]
	cfg.Spec.Secrets[0].EnvName = "PROMPT"
	source["roles/operator/role.yaml"].Data = bytes.ReplaceAll(
		source["roles/operator/role.yaml"].Data,
		[]byte("[first-auth, second-auth]"),
		[]byte("[first-auth]"),
	)
	result, err = compose.Compose(cfg, config.Root(), opts)
	if err == nil || result != nil || !strings.Contains(err.Error(), "conflicts with template") {
		t.Fatalf("result=%v err=%v", result, err)
	}
}

func TestParametersAcrossSelectedHarnesses(t *testing.T) {
	cfg, opts := fixture(t)
	allHarnesses(cfg)
	cfg.Spec.Defaults.Harnesses = cfg.Spec.Defaults.Harnesses[:2]
	cfg.Spec.Parameters = map[string]string{"ANTHROPIC_MODEL": "sonnet", "PROMPT": "hello"}
	result, err := compose.Compose(cfg, config.Root(), opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range result.Pairs {
		if pair.Config.Spec.Values["PROMPT"] != "hello" {
			t.Fatal("shared parameter lost")
		}
		value, ok := pair.Config.Spec.Values["ANTHROPIC_MODEL"]
		if pair.Harness == "claude" {
			if !ok || value != "sonnet" {
				t.Fatal("Claude parameter lost")
			}
		} else if ok {
			t.Fatal("Claude parameter bound to Codex")
		}
	}
	cfg.Spec.Defaults.Roles[0].Harness = "codex"
	if result, err = compose.Compose(cfg, config.Root(), opts); err == nil || result != nil ||
		!strings.Contains(err.Error(), `unsupported parameter "ANTHROPIC_MODEL"`) {
		t.Fatalf("unselected harness accepted parameter: %v", err)
	}
}

func TestEffectiveSourcesControlRuntimeClones(t *testing.T) {
	cfg, opts := fixture(t)
	allHarnesses(cfg)
	cfg.Spec.Secrets = []config.SecretBinding{routerBinding()}
	cfg.Spec.Defaults.Sources = append(cfg.Spec.Defaults.Sources,
		config.Source{Name: "definitions-only", URL: "https://example.test/defs", Tag: "v1"})
	team := config.Root()
	team.Spec.Sources = []config.Source{
		{Name: "kuketeams", URL: "https://example.test/roles", Commit: "abc123", ClonePath: "/opt/roles"},
		{Name: "extra", URL: "https://example.test/extra", Branch: "topic", ClonePath: "/opt/extra"},
		{Name: "project", URL: "https://example.test/project", Tag: "v3", ClonePath: "/opt/project"},
		{Name: "definitions-only", URL: "https://example.test/defs", Tag: "v1"},
	}
	result, err := compose.Compose(cfg, team, opts)
	if err != nil {
		t.Fatal(err)
	}
	pair := result.Pairs[0]
	if len(pair.Config.Spec.Repos) != 3 || pair.Config.Spec.Repos["kuketeams"].URL != "https://example.test/roles" ||
		pair.Config.Spec.Repos["kuketeams"].Ref != "abc123" || pair.Config.Spec.Repos["extra"].Branch != "topic" {
		t.Fatalf("effective source clone declarations lost: %+v", pair.Config.Spec.Repos)
	}
	if !strings.Contains(pair.Blueprint.Spec.Cell.Containers[0].Tty.OnInit[0].Script, "make -C /opt/roles") {
		t.Fatal("global source path used instead of team override")
	}
	for _, selected := range result.Pairs {
		if len(selected.Config.Spec.Repos) != 3 || len(selected.Blueprint.Spec.Cell.Containers[0].Repos) != 3 {
			t.Fatalf("%s omitted explicitly requested clones", selected.Harness)
		}
	}
	team.Spec.Sources[0].ClonePath = ""
	if result, err = compose.Compose(cfg, team, opts); err == nil || result != nil ||
		!strings.Contains(err.Error(), "clonePath") {
		t.Fatalf("required role clone silently inferred: %v", err)
	}
}

// Execute the rendered launch stages in a real local checkout. Harness binaries
// and materialization are stubbed; Git and the launcher's working directory are real.
func TestLaunchPreservesSourceRevision(t *testing.T) {
	for _, revision := range []string{"branch", "tag", "commit"} {
		t.Run(revision, func(t *testing.T) {
			cfg, opts := fixture(t)
			allHarnesses(cfg)
			cfg.Spec.Secrets = []config.SecretBinding{routerBinding()}
			root := t.TempDir()
			repoDir := filepath.Join(root, "project")
			runGit := func(args ...string) string {
				t.Helper()
				cmd := exec.CommandContext(t.Context(), "git", args...)
				cmd.Dir = root
				raw, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v: %s", args, err, raw)
				}
				return strings.TrimSpace(string(raw))
			}
			sourceDir, err := filepath.Abs("../..")
			if err != nil {
				t.Fatal(err)
			}
			runGit("clone", "--quiet", "--shared", "--no-checkout", sourceDir, repoDir)
			sha := runGit("-C", repoDir, "rev-parse", "HEAD")
			project := config.Source{Name: "project", URL: "https://example.test/project", ClonePath: repoDir}
			wantBranch := ""
			switch revision {
			case "branch":
				project.Branch, wantBranch = "selected-topic", "selected-topic"
				runGit("-C", repoDir, "checkout", "-b", project.Branch, sha)
			case "tag":
				project.Tag = "selected-v2"
				runGit("-C", repoDir, "tag", "--no-sign", project.Tag, sha)
				runGit("-C", repoDir, "checkout", "--detach", project.Tag)
			case "commit":
				project.Commit = sha
				runGit("-C", repoDir, "checkout", "--detach", sha)
			}
			cfg.Spec.Defaults.Sources = append(cfg.Spec.Defaults.Sources, project)
			source := copiedSource(t)
			source["roles/operator/role.yaml"].Data = bytes.ReplaceAll(source["roles/operator/role.yaml"].Data,
				[]byte("  needs:"), []byte("  needs:\n    repos: [project]"))
			opts.Sources["kuketeams"] = source
			result, err := compose.Compose(cfg, config.Root(), opts)
			if err != nil {
				t.Fatal(err)
			}
			binDir := filepath.Join(root, "bin")
			if err = os.Mkdir(binDir, 0o700); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"make", "codex", "claude", "opencode"} {
				script := "#!/bin/sh\nexit 0\n"
				if name != "make" {
					script = "#!/bin/sh\n(git symbolic-ref --quiet --short HEAD || true) > \"$LAUNCH_STATE\"\ngit rev-parse HEAD >> \"$LAUNCH_STATE\"\n"
				}
				if err = os.WriteFile(filepath.Join(binDir, name), []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			for _, pair := range result.Pairs {
				fill := pair.Config.Spec.Repos["project"]
				if fill.Branch != project.Branch || fill.Ref != project.Tag+project.Commit {
					t.Fatalf("%s lost source revision: %+v", pair.Harness, fill)
				}
				var launch strings.Builder
				for _, stage := range pair.Blueprint.Spec.Cell.Containers[0].Tty.OnInit {
					if strings.Contains(stage.Script, "make -C") || strings.Contains(stage.Script, "cd "+repoDir) {
						launch.WriteString(stage.Script + "\n")
					}
				}
				state := filepath.Join(root, pair.Harness+"-state")
				cmd := exec.CommandContext(t.Context(), "bash", "-eu", "-c", launch.String())
				cmd.Env = append(
					os.Environ(),
					"PATH="+binDir+":"+os.Getenv("PATH"),
					"PROMPT=",
					"OPENROUTER_MODEL=test",
					"LAUNCH_STATE="+state,
				)
				if output, launchErr := cmd.CombinedOutput(); launchErr != nil {
					t.Fatalf("%s launch: %v: %s", pair.Harness, launchErr, output)
				}
				raw, readErr := os.ReadFile(state)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if strings.TrimSpace(string(raw)) != strings.TrimSpace(wantBranch+"\n"+sha) {
					t.Fatalf("%s changed checkout at launch: %q", pair.Harness, raw)
				}
			}
		})
	}
}
