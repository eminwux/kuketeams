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

package initialize

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/kukeon/pkg/api/kukeonv1"
	"github.com/eminwux/kuketeams/internal/bootstrap"
	"github.com/eminwux/kuketeams/pkg/config"
	"gopkg.in/yaml.v3"
)

func sourceFS() map[string]fs.FS { return map[string]fs.FS{"kuketeams": os.DirFS("../..")} }

func setupRun(t *testing.T) Options {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".kuke")
	_, err := bootstrap.Run(t.Context(), bootstrap.Options{Root: root, Git: emptyGit{}})
	if err != nil {
		t.Fatal(err)
	}
	return Options{Root: root, Home: filepath.Dir(root), Sources: sourceFS(), Client: newFake()}
}

type emptyGit struct{}

func (emptyGit) Lookup(context.Context, string, bool) (string, error) {
	return "", bootstrap.ErrMissing
}

func TestRunPreservesAuthoredConfigurationAndEntry(t *testing.T) {
	opts := setupRun(t)
	path := filepath.Join(opts.Root, "kuketeams.yaml")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	original = append([]byte("# authored comment\n"), original...)
	if err = os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		result, runErr := Run(t.Context(), opts)
		if runErr != nil {
			t.Fatal(runErr)
		}
		if result.Resources != 2 || result.EntryPath != filepath.Join(opts.Root, "kuketeam.d", "root.yaml") {
			t.Fatalf("result=%+v", result)
		}
		after, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(after, original) {
			t.Fatal("configuration overwritten")
		}
		entryBytes, readErr := os.ReadFile(result.EntryPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		entry, decodeErr := config.DecodeEntry(bytes.NewReader(entryBytes))
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if entry.Metadata.Name != "root" || entry.Spec.Roles != nil || entry.Spec.Sources != nil {
			t.Fatal("authored inheritance was flattened")
		}
		info, statErr := os.Stat(result.EntryPath)
		if statErr != nil || info.Mode().Perm() != 0o600 {
			t.Fatal("unsafe entry permissions")
		}
	}
	f := opts.Client.(*fakeClient)
	if f.applies != 2 || f.team != "root" {
		t.Fatal("rerun did not update stable identity")
	}
}

func TestRegistrationOnlyChangesAfterFullSuccess(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, mode := range []string{"rpc", "total", "partial", "preflight"} {
			t.Run(fmt.Sprintf("existing=%t/%s", existing, mode), func(t *testing.T) {
				opts := setupRun(t)
				entryPath := filepath.Join(opts.Root, "kuketeam.d", "root.yaml")
				if existing {
					if _, err := Run(t.Context(), opts); err != nil {
						t.Fatal(err)
					}
				}
				before, _ := os.ReadFile(entryPath)
				f := newFake()
				opts.Client = f
				switch mode {
				case "rpc":
					f.applyErr = io.EOF
				case "total":
					f.resources = []kukeonv1.ApplyResourceResult{
						{Action: "failed", Error: "bad blueprint"},
						{Action: "failed", Error: "bad config"},
					}
				case "partial":
					f.resources = []kukeonv1.ApplyResourceResult{
						{Action: "updated"},
						{Action: "failed", Error: "bad config"},
					}
				case "preflight":
					f.missingRealm = true
				}
				result, err := Run(t.Context(), opts)
				if err == nil || result.EntryPath != "" {
					t.Fatalf("claimed successful registration: %+v, %v", result, err)
				}
				after, readErr := os.ReadFile(entryPath)
				if existing {
					if readErr != nil || !bytes.Equal(before, after) {
						t.Fatal("prior registration changed")
					}
				} else if !os.IsNotExist(readErr) {
					t.Fatal("failed team registered")
				}
				entries, readErr := os.ReadDir(filepath.Join(opts.Root, "kuketeam.d"))
				if readErr != nil {
					t.Fatal(readErr)
				}
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".root-") {
						t.Fatal("staging file leaked")
					}
				}
			})
		}
	}
}

func TestRunUsesExistingRootDeclaration(t *testing.T) {
	opts := setupRun(t)
	team := config.Root()
	team.Spec.Roles = []config.Role{{Name: "custom", Source: "kuketeams", Path: "roles/operator", Harness: "codex"}}
	entry, err := config.Entry(team)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := yaml.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(opts.Root, "kuketeam.d", "root.yaml")
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = Run(t.Context(), opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(opts.Client.(*fakeClient).raw), "root-custom-codex") {
		t.Fatal("authored root selection reset")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := config.DecodeEntry(bytes.NewReader(after))
	if err != nil || accepted.Spec.Roles[0].Name != "custom" {
		t.Fatal("entry did not retain declaration")
	}
}

func TestInvalidExistingEntryFailsBeforeApplication(t *testing.T) {
	for _, raw := range []string{"kind: Wrong\n", "apiVersion: kuketeams.io/v1\nkind: TeamEntry\nmetadata:\n  name: different\nspec: {}\n"} {
		opts := setupRun(t)
		path := filepath.Join(opts.Root, "kuketeam.d", "root.yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Run(t.Context(), opts); err == nil {
			t.Fatal("accepted invalid root entry")
		}
		if opts.Client.(*fakeClient).applies != 0 {
			t.Fatal("applied before entry validation")
		}
	}
}
