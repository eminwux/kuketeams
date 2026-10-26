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

// Package operatorassets_test checks the shipped role and native harness layouts.
package operatorassets_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/eminwux/kuketeams/internal/bootstrap"
	"gopkg.in/yaml.v3"
)

type noGit struct{}

func (noGit) Lookup(context.Context, string, bool) (string, error) {
	return "", bootstrap.ErrMissing
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

type definition struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Skills    []string `yaml:"skills"`
		Harnesses map[string]struct {
			Settings    string   `yaml:"settings"`
			Sandbox     string   `yaml:"sandbox"`
			Approval    string   `yaml:"approval"`
			Permissions string   `yaml:"permissions"`
			Secrets     []string `yaml:"secrets"`
		} `yaml:"harnesses"`
		Needs struct {
			Image   []string `yaml:"image"`
			Repos   []string `yaml:"repos"`
			Mounts  []string `yaml:"mounts"`
			Params  []string `yaml:"params"`
			Secrets []string `yaml:"secrets"`
		} `yaml:"needs"`
	} `yaml:"spec"`
}

func TestDefaultRoleAssets(t *testing.T) {
	// Resolve the actual bootstrap selection, rather than duplicating its path
	// in a fixture that could pass while init points at a different role.
	result, err := bootstrap.Run(t.Context(), bootstrap.Options{Root: t.TempDir(), Git: noGit{}})
	if err != nil {
		t.Fatal(err)
	}
	roles := result.Config.Spec.Defaults.Roles
	if len(roles) != 1 {
		t.Fatalf("default roles: %+v", roles)
	}
	selection := roles[0]
	if selection.Name != "operator" || selection.Source != "kuketeams" {
		t.Fatalf("selection: %+v", selection)
	}
	root := repoRoot(t)
	roleDir := filepath.Join(root, selection.Path)
	var role definition
	decoder := yaml.NewDecoder(bytes.NewReader(read(t, filepath.Join(roleDir, "role.yaml"))))
	decoder.KnownFields(true)
	if decodeErr := decoder.Decode(&role); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if role.APIVersion != "kuketeams.io/v1" || role.Kind != "Role" || role.Metadata.Name != selection.Name {
		t.Fatalf("invalid role identity: %+v", role)
	}
	if !reflect.DeepEqual(role.Spec.Needs.Image, selection.Needs.Image) {
		t.Fatalf("role capabilities %v differ from bootstrap %v", role.Spec.Needs.Image, selection.Needs.Image)
	}
	if len(role.Spec.Needs.Repos)+len(role.Spec.Needs.Mounts)+len(role.Spec.Needs.Secrets) != 0 {
		t.Fatal("operator should not require project repos, mounts, or non-provider secrets")
	}
	if len(role.Spec.Skills) != 1 {
		t.Fatalf("skills: %v", role.Spec.Skills)
	}
	for _, path := range role.Spec.Skills {
		if filepath.IsAbs(path) || !filepath.IsLocal(path) {
			t.Fatalf("nonlocal skill: %s", path)
		}
		skillDir := filepath.Join(root, path)
		rel, relErr := filepath.Rel(roleDir, skillDir)
		if relErr != nil || !filepath.IsLocal(rel) {
			t.Fatalf("skill outside role: %s", path)
		}
		skill := read(t, filepath.Join(skillDir, "SKILL.md"))
		parts := bytes.SplitN(skill, []byte("---\n"), 3)
		if len(parts) != 3 || len(parts[0]) != 0 {
			t.Fatal("missing frontmatter")
		}
		var front struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if frontErr := yaml.Unmarshal(parts[1], &front); frontErr != nil {
			t.Fatal(frontErr)
		}
		if front.Name != filepath.Base(skillDir) || len(front.Description) == 0 {
			t.Fatalf("frontmatter: %+v", front)
		}
		links := regexp.MustCompile(`\]\(([^)]+)\)`).FindAllSubmatch(parts[2], -1)
		if len(links) == 0 {
			t.Fatal("missing grounding reference")
		}
		for _, link := range links {
			target := string(link[1])
			if !filepath.IsLocal(target) {
				t.Fatalf("nonlocal skill reference: %s", target)
			}
			_ = read(t, filepath.Join(skillDir, target))
		}
	}
	if !bytes.Equal(read(t, filepath.Join(roleDir, "AGENTS.md")), read(t, filepath.Join(roleDir, "CLAUDE.md"))) {
		t.Fatal("Claude adapter differs from canonical instructions")
	}
	checkHarnessSettings(t, role, roleDir)
}

func checkHarnessSettings(t *testing.T, role definition, roleDir string) {
	t.Helper()
	wantSecrets := map[string]string{
		"claude":   "claude-code-oauth-token",
		"codex":    "openai-api-key",
		"opencode": "openrouter-api-key",
	}
	if len(role.Spec.Harnesses) != len(wantSecrets) {
		t.Fatal("unexpected harness selection")
	}
	for name, secret := range wantSecrets {
		h, ok := role.Spec.Harnesses[name]
		if !ok || !reflect.DeepEqual(h.Secrets, []string{secret}) {
			t.Fatalf("%s provider secrets: %v", name, h.Secrets)
		}
	}
	var settings struct {
		Permissions struct {
			DefaultMode string `json:"defaultMode"`
		} `json:"permissions"`
	}
	settingsPath := role.Spec.Harnesses["claude"].Settings
	if !filepath.IsLocal(settingsPath) {
		t.Fatal("Claude settings must be role-relative")
	}
	if err := json.Unmarshal(read(t, filepath.Join(roleDir, settingsPath)), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Permissions.DefaultMode != "default" {
		t.Fatal("Claude settings bypass permissions")
	}
	codex := role.Spec.Harnesses["codex"]
	if codex.Sandbox != "workspace-write" || codex.Approval != "on-request" {
		t.Fatal("unexpected Codex permissions")
	}
	if role.Spec.Harnesses["opencode"].Permissions != "ask" {
		t.Fatal("OpenCode permissions should ask")
	}
	var oc struct {
		Permission string `json:"permission"`
		Model      string `json:"model"`
		Provider   map[string]struct {
			Options struct {
				APIKey string `json:"apiKey"`
			} `json:"options"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(read(t, filepath.Join(roleDir, "config/opencode.json")), &oc); err != nil {
		t.Fatal(err)
	}
	if oc.Permission != "ask" || oc.Model != "{env:OPENROUTER_MODEL}" ||
		oc.Provider["openrouter"].Options.APIKey != "{env:OPENROUTER_API_KEY}" {
		t.Fatal("OpenCode config should use permission checks and environment credential references")
	}
}

func runMake(t *testing.T, dir, home, target string, overrides ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "make", "-C", dir, target)
	cmd.Env = append(
		os.Environ(),
		"HOME="+home,
		"CODEX_HOME=",
		"CLAUDE_CONFIG_DIR=",
		"XDG_CONFIG_HOME=",
		"OPENCODE_CONFIG=",
	)
	cmd.Env = append(cmd.Env, overrides...)
	return cmd.CombinedOutput()
}

func TestMaterialization(t *testing.T) {
	for _, harness := range []string{"claude", "codex", "opencode-openrouter"} {
		t.Run(harness, func(t *testing.T) {
			// A copied role alone must work, even with spaces in its path and
			// no agents checkout, harness catalog, Go executable, or daemon.
			roleDir := filepath.Join(t.TempDir(), "operator source")
			copyCmd := exec.CommandContext(
				t.Context(),
				"cp",
				"-a",
				filepath.Join(repoRoot(t), "roles/operator"),
				roleDir,
			)
			if output, err := copyCmd.CombinedOutput(); err != nil {
				t.Fatalf("copy: %s: %v", output, err)
			}
			for _, custom := range []bool{false, true} {
				t.Run(map[bool]string{false: "native", true: "overrides"}[custom], func(t *testing.T) {
					home := filepath.Join(t.TempDir(), "cell home")
					configDir := filepath.Join(home, ".claude")
					policyName := "CLAUDE.md"
					skills := filepath.Join(configDir, "skills")
					var overrides []string
					switch harness {
					case "claude":
						if custom {
							configDir = filepath.Join(home, "custom claude")
							overrides = []string{"CLAUDE_CONFIG_DIR=" + configDir}
							skills = filepath.Join(configDir, "skills")
						}
					case "codex":
						configDir = filepath.Join(home, ".codex")
						if custom {
							configDir = filepath.Join(home, "custom codex")
							overrides = []string{"CODEX_HOME=" + configDir}
						}
						policyName = "AGENTS.md"
						skills = filepath.Join(home, ".agents/skills")
					case "opencode-openrouter":
						configDir = filepath.Join(home, ".config/opencode")
						if custom {
							configDir = filepath.Join(home, "custom xdg/opencode")
							overrides = []string{
								"XDG_CONFIG_HOME=" + filepath.Dir(configDir),
								"OPENCODE_CONFIG=" + filepath.Join(home, "custom settings.json"),
							}
						}
						policyName = "AGENTS.md"
						skills = filepath.Join(home, ".agents/skills")
					}
					for range 2 {
						if output, err := runMake(t, roleDir, home, harness, overrides...); err != nil {
							t.Fatalf("install: %s: %v", output, err)
						}
					}
					if !bytes.Equal(
						read(t, filepath.Join(configDir, policyName)),
						read(t, filepath.Join(roleDir, "AGENTS.md")),
					) {
						t.Fatal("native policy differs")
					}
					installed := filepath.Join(skills, "create-cell-config")
					if !bytes.Equal(
						read(t, filepath.Join(installed, "SKILL.md")),
						read(t, filepath.Join(roleDir, "skills/create-cell-config/SKILL.md")),
					) {
						t.Fatal("skill differs")
					}
					_ = read(t, filepath.Join(installed, "references/kukeon.md"))
					if harness == "opencode-openrouter" {
						settings := filepath.Join(configDir, "opencode.json")
						if custom {
							settings = filepath.Join(home, "custom settings.json")
						}
						if !bytes.Equal(read(t, settings), read(t, filepath.Join(roleDir, "config/opencode.json"))) {
							t.Fatal("OpenCode settings differ")
						}
					}
				})
			}
		})
	}
}

func TestConflictsPreserveHome(t *testing.T) {
	roleDir := filepath.Join(repoRoot(t), "roles/operator")
	for _, tc := range []struct{ target, conflict, absent string }{
		{"claude", ".claude/CLAUDE.md", ".claude/skills/create-cell-config"},
		{"codex", ".codex/AGENTS.md", ".agents/skills/create-cell-config"},
		{"codex", ".codex/AGENTS.override.md", ".agents/skills/create-cell-config"},
		{"codex", ".agents/skills/create-cell-config", ".codex/AGENTS.md"},
		{"opencode-openrouter", ".config/opencode/opencode.json", ".config/opencode/AGENTS.md"},
	} {
		t.Run(tc.conflict, func(t *testing.T) {
			home := t.TempDir()
			conflict := filepath.Join(home, tc.conflict)
			if err := os.MkdirAll(filepath.Dir(conflict), 0o700); err != nil {
				t.Fatal(err)
			}
			original := []byte("user-owned content\n")
			if err := os.WriteFile(conflict, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if output, err := runMake(t, roleDir, home, tc.target); err == nil {
				t.Fatalf("expected conflict, got %s", output)
			}
			if !bytes.Equal(read(t, conflict), original) {
				t.Fatal("user asset changed")
			}
			if _, err := os.Lstat(filepath.Join(home, tc.absent)); !os.IsNotExist(err) {
				t.Fatalf("partial install before conflict: %v", err)
			}
		})
	}
}

// The fixtures are the complete manifests from PR #11 at the revision recorded
// in the role guide. Prefer the actual packaged files as soon as they land.
func TestHarnessContracts(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{"claude", "codex", "opencode"} {
		t.Run(name, func(t *testing.T) {
			manifest := filepath.Join(root, "harnesses", name, "harness.yaml")
			if _, err := os.Stat(manifest); os.IsNotExist(err) {
				manifest = filepath.Join("testdata", name+".harness.yaml")
			}
			var harness struct {
				Spec struct {
					SkillPath  string `yaml:"skillPath"`
					MakeTarget string `yaml:"makeTarget"`
				} `yaml:"spec"`
			}
			if err := yaml.Unmarshal(read(t, manifest), &harness); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(harness.Spec.SkillPath, "$HOME/") {
				t.Fatalf("unexpected skill path: %s", harness.Spec.SkillPath)
			}
			home := t.TempDir()
			if output, err := runMake(t, root, home, harness.Spec.MakeTarget); err != nil {
				t.Fatalf("root make target: %s: %v", output, err)
			}
			installed := filepath.Join(
				home,
				strings.TrimPrefix(harness.Spec.SkillPath, "$HOME/"),
				"create-cell-config/SKILL.md",
			)
			source := filepath.Join(root, "roles/operator/skills/create-cell-config/SKILL.md")
			if !bytes.Equal(read(t, installed), read(t, source)) {
				t.Fatal("skill not installed at harness-declared path")
			}
		})
	}
}
