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

// Package harnessassets contains packaging checks, not a production compositor.
package harnessassets_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/template"

	"github.com/eminwux/kuketeams/pkg/harness"
	"gopkg.in/yaml.v3"
)

const root = "../../harnesses"

func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCatalogAndBuildClosure(t *testing.T) {
	expected := map[string]bool{
		"claude":         true,
		"claude-root":    true,
		"claude-android": true,
		"codex":          true,
		"codex-root":     true,
		"opencode":       true,
		"opencode-root":  true,
	}
	var tag string
	for _, name := range []string{"claude", "codex", "opencode"} {
		h, err := harness.Load(os.DirFS(root), name)
		if err != nil {
			t.Fatal(err)
		}
		catalog, err := harness.LoadCatalog(os.DirFS(root), name, h.Metadata.Name)
		if err != nil {
			t.Fatal(err)
		}
		if len(h.Spec.Images) != len(catalog.Spec.Images) {
			t.Fatal("build/catalog count mismatch")
		}
		for i, image := range h.Spec.Images {
			if !expected[image.Name] {
				t.Fatalf("duplicate or unexpected image %s", image.Name)
			}
			delete(expected, image.Name)
			published := catalog.Spec.Images[i]
			prefix := "ghcr.io/eminwux/kuketeams/" + image.Name + ":"
			if !strings.HasPrefix(published.Image, prefix) {
				t.Fatalf("wrong namespace: %s", published.Image)
			}
			current := strings.TrimPrefix(published.Image, prefix)
			if tag != "" && tag != current {
				t.Fatal("mixed catalog tags")
			}
			tag = current
			if !slices.Equal(image.Capabilities, published.Capabilities) {
				t.Fatal("capability drift")
			}
			if image.Build.Context != "harnesses/"+name {
				t.Fatal("context escapes harness directory")
			}
			checkDockerfile(t, name, filepath.Join(name, image.Build.Dockerfile))
		}
	}
	if len(expected) != 0 {
		t.Fatalf("missing images: %v", expected)
	}
	for _, name := range []string{"root", "user"} {
		if !strings.Contains(string(read(t, "base/Dockerfile."+name)), "FROM ${IMAGE_PREFIX}base:${TAG}") {
			t.Fatal("broken base dependency")
		}
	}
	read(t, "base/Dockerfile")
}

func TestTemplatesWithSiblingPartials(t *testing.T) {
	for _, harness := range []string{"claude", "codex", "opencode"} {
		t.Run(harness, func(t *testing.T) {
			var definition struct {
				Kind string `yaml:"kind"`
				Spec struct {
					Template string `yaml:"template"`
				} `yaml:"spec"`
			}
			if err := yaml.Unmarshal(read(t, harness+"/harness.yaml"), &definition); err != nil {
				t.Fatal(err)
			}
			if definition.Kind != "Harness" || definition.Spec.Template != "blueprint.tmpl.yaml" {
				t.Fatal("invalid harness template reference")
			}
			funcs := template.FuncMap{
				"upper":   strings.ToUpper,
				"replace": func(old, replacement, value string) string { return strings.ReplaceAll(value, old, replacement) },
			}
			tmpl, parseErr := template.New(definition.Spec.Template).
				Funcs(funcs).
				Option("missingkey=error").
				ParseGlob(filepath.Join(root, harness, "*.tmpl.yaml"))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			for _, rootUser := range []bool{false, true} {
				for _, projectBound := range []bool{false, true} {
					imageNeeds := []string{harness, "git"}
					home := "/home/user"
					if rootUser {
						imageNeeds = append(imageNeeds, "root")
					}
					projectDir := ""
					if projectBound {
						projectDir = "/host/project"
					}
					data := map[string]any{
						"makeTarget": harness,
						"role": map[string]any{
							"path": "roles/operator", "sourceDir": "/home/user/kuketeams", "initialPath": `"$HOME"`,
							"name": "fixture",
						}, "realm": "default", "space": "default", "stack": "default", "image": "fixture:latest",
						"needs": map[string]any{
							"image":  imageNeeds,
							"repos":  []string{"agents", "project"},
							"mounts": []string{"ssh", "kukeond-socket", "kuke-binary", "kukeon-install"},
							"params": []string{"ROLE", "PROMPT", "CUSTOM"},
						},
						"project": map[string]any{
							"X":           "fixture",
							"PROJECT_DIR": projectDir,
							"CLONE_DIR":   "/home/user/example",
							"NAME":        "example",
							"AGENTS_REPO": "eminwux/agents",
						},
						"operator": map[string]any{
							"X":              "fixture",
							"HOME_DIR":       "/host/home",
							"TEAM_ROOT":      "/host/team",
							"GIT_USER_NAME":  "Fixture",
							"GIT_USER_EMAIL": "fixture@example.org",
						},
						"harnesses": map[string]any{
							harness: map[string]any{
								"secrets":     []string{"test-token"},
								"settings":    "config/claude.settings.json",
								"sandbox":     "danger-full-access",
								"approval":    "never",
								"permissions": "allow",
							},
						},
					}
					var out bytes.Buffer
					if err := tmpl.Execute(&out, data); err != nil {
						t.Fatal(err)
					}
					checkTemplateOutput(t, &out, home, rootUser)
				}
			}
		})
	}
}

func checkDockerfile(t *testing.T, harness, path string) {
	t.Helper()
	content := string(read(t, path))
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "COPY":
			for _, source := range fields[1 : len(fields)-1] {
				read(t, filepath.Join(harness, source))
			}
		case "FROM":
			if !strings.HasPrefix(fields[1], "${IMAGE_PREFIX}base-") || !strings.HasSuffix(fields[1], ":${TAG}") {
				t.Fatalf("unpackaged base: %s", line)
			}
			base := strings.TrimSuffix(strings.TrimPrefix(fields[1], "${IMAGE_PREFIX}base-"), ":${TAG}")
			read(t, "base/Dockerfile."+base)
		}
	}
}

func checkTemplateOutput(t *testing.T, out *bytes.Buffer, home string, rootUser bool) {
	t.Helper()
	var blueprint map[string]any
	if err := yaml.Unmarshal(out.Bytes(), &blueprint); err != nil {
		t.Fatalf("invalid rendered YAML: %v", err)
	}
	if blueprint["kind"] != "CellBlueprint" {
		t.Fatal("wrong rendered kind")
	}
	for _, value := range []string{"workingDir: " + home, "target: " + home + "/.ssh", "source: /host/home/.ssh", "envName: TEST_TOKEN", "signingKey:     /home/user/.ssh/id_ed25519.pub", "target: /home/user/agents"} {
		if !strings.Contains(out.String(), value) {
			t.Fatalf("missing %q", value)
		}
	}
	if strings.Contains(out.String(), "<no value>") || strings.Contains(out.String(), "{{") {
		t.Fatal("unresolved template")
	}
	if strings.Contains(out.String(), "/root") || strings.Contains(out.String(), "/home/claude") {
		t.Fatal("legacy home path in rendered blueprint")
	}
	if strings.Contains(out.String(), "nestedCgroupRuntime: true") != rootUser {
		t.Fatal("nested runtime capability changed")
	}
	if strings.Contains(out.String(), `ssh -F "$(if [ -f /home/user/.ssh/config ]`) != rootUser {
		t.Fatal("root SSH config handling changed")
	}
	if strings.Contains(out.String(), "privileged: true") != rootUser {
		t.Fatal("root privilege changed")
	}
}

func TestSourceInventoryIsPackaged(t *testing.T) {
	for _, line := range strings.Split(strings.TrimSpace(string(read(t, "UPSTREAM.tsv"))), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			t.Fatalf("invalid upstream inventory row: %q", line)
		}
		// Keep the deleted central catalog's original blob in the historical
		// inventory, even though release catalogs now live beside each Harness.
		if fields[3] == "harnesses/images.yaml" {
			continue
		}
		path := filepath.Join("../..", fields[3])
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fields[0] == "100755" && info.Mode()&0o111 == 0 {
			t.Fatalf("lost executable mode: %s", path)
		}
	}
}
