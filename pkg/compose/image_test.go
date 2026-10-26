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
	"testing"

	model "github.com/eminwux/kukeon/pkg/api/model/kuketeams"
	"github.com/eminwux/kuketeams/pkg/compose"
)

func TestImageSelectionRequiresCompleteCapabilities(t *testing.T) {
	catalog := &model.ImageCatalog{
		APIVersion: "kuketeams.io/v1", Kind: "ImageCatalog",
		Spec: model.ImageCatalogSpec{Images: []model.ImageCatalogEntry{
			{
				Ref:          "wrong-harness",
				Harness:      "claude",
				Image:        "registry.test/claude:v1",
				Capabilities: []string{"codex", "gh", "go"},
			},
			{
				Ref:          "root",
				Harness:      "codex",
				Image:        "registry.test/root:v1",
				Capabilities: []string{"codex", "gh", "go", "root"},
			},
			{Ref: "go", Harness: "codex", Image: "registry.test/go:v1", Capabilities: []string{"codex", "go"}},
			{Ref: "gh", Harness: "codex", Image: "registry.test/gh:v1", Capabilities: []string{"codex", "gh"}},
		}},
	}
	if _, err := compose.SelectImage(catalog, "codex", []string{"go", "gh"}); err == nil {
		t.Fatal("selected privileged image or combined capabilities across images")
	}
	selected, err := compose.SelectImage(catalog, "codex", []string{"go"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Ref != "go" {
		t.Fatal("wrong harness or gratuitous root selection")
	}
	selected, err = compose.SelectImage(catalog, "codex", []string{"go", "root"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Ref != "root" {
		t.Fatal("root capability ignored")
	}
	catalog.Spec.Images[2].Image = "local-go"
	if _, selectionErr := compose.SelectImage(catalog, "codex", []string{"go"}); selectionErr == nil {
		t.Fatal("accepted nonpublished image reference")
	}
	catalog.Spec.Images[2].Image = "registry.test/go:v1"
	catalog.Spec.Images[2].Capabilities = []string{"go"}
	if _, selectionErr := compose.SelectImage(catalog, "codex", []string{"go"}); selectionErr == nil {
		t.Fatal("harness capability not required")
	}
}
