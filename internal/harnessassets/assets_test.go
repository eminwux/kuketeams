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
	"strings"
	"testing"
	"text/template"

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
	var catalog struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Spec       struct {
			Images []struct {
				Ref     string `yaml:"ref"`
				Harness string `yaml:"harness"`
				Image   string `yaml:"image"`
				Build   struct {
					Context    string `yaml:"context"`
					Dockerfile string `yaml:"dockerfile"`
				} `yaml:"build"`
				Capabilities []string `yaml:"capabilities"`
			} `yaml:"images"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(read(t, "images.yaml"), &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Kind != "ImageCatalog" || catalog.APIVersion != "kuketeams.io/v1" {
		t.Fatal("wrong catalog kind/version")
	}
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
	for _, image := range catalog.Spec.Images {
		if !expected[image.Ref] {
			t.Fatalf("duplicate or unexpected image: %s", image.Ref)
		}
		delete(expected, image.Ref)
		prefix := "ghcr.io/eminwux/kuketeams/" + image.Ref + ":"
		if !strings.HasPrefix(image.Image, prefix) {
			t.Fatalf("wrong namespace: %s", image.Image)
		}
		currentTag := strings.TrimPrefix(image.Image, prefix)
		if tag != "" && tag != currentTag {
			t.Fatal("mixed catalog tags")
		}
		tag = currentTag
		if image.Harness != strings.Split(image.Ref, "-")[0] {
			t.Fatalf("wrong harness: %s", image.Ref)
		}
		found := false
		for _, capability := range image.Capabilities {
			found = found || capability == image.Harness
		}
		if !found {
			t.Fatalf("missing harness capability: %s", image.Ref)
		}
		if image.Build.Context != "harnesses/"+image.Harness {
			t.Fatal("context escapes harness directory")
		}
		path := filepath.Join(image.Harness, image.Build.Dockerfile)
		checkDockerfile(t, image.Harness, path)
	}
	if len(expected) != 0 {
		t.Fatalf("missing images: %v", expected)
	}
	for _, name := range []string{"root", "user"} {
		if !strings.Contains(string(read(t, "base/Dockerfile."+name)), "FROM ${REGISTRY}/base:${TAG}") {
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
					home := "/home/claude"
					if rootUser {
						imageNeeds = append(imageNeeds, "root")
						home = "/root"
					}
					projectDir := ""
					if projectBound {
						projectDir = "/host/project"
					}
					data := map[string]any{
						"role": map[string]any{
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
			if !strings.HasPrefix(fields[1], "${REGISTRY}/base-") || !strings.HasSuffix(fields[1], ":${TAG}") {
				t.Fatalf("unpackaged base: %s", line)
			}
			base := strings.TrimSuffix(strings.TrimPrefix(fields[1], "${REGISTRY}/base-"), ":${TAG}")
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
	for _, value := range []string{"workingDir: " + home, "target: " + home + "/.ssh", "source: /host/home/.ssh", "envName: TEST_TOKEN"} {
		if !strings.Contains(out.String(), value) {
			t.Fatalf("missing %q", value)
		}
	}
	if strings.Contains(out.String(), "<no value>") || strings.Contains(out.String(), "{{") {
		t.Fatal("unresolved template")
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
