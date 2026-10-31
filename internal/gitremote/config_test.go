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

package gitremote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryTransportConfigPrecedence(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	global := filepath.Join(t.TempDir(), "global")
	if err := os.WriteFile(global, []byte("[credential]\n helper = global-helper\n[http]\n extraHeader = X-Global: once\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_PARAMETERS", "'credential.helper=command-helper'")
	repo, temporary := t.TempDir(), t.TempDir()
	for _, dir := range []string{repo, temporary} {
		if _, err := Run(t.Context(), "init", "--quiet", dir); err != nil {
			t.Fatal(err)
		}
	}
	// Empty helpers reset the global helper list; a valueless boolean is true.
	local := "[credential]\n helper =\n helper = local-helper\n useHttpPath\n[http]\n extraHeader = X-Local: once\n[core]\n worktree = /must-not-travel\n"
	if err := os.WriteFile(filepath.Join(repo, ".git", "config"), []byte(local), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, err := WithRepositoryConfig(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, want string }{
		{"credential.helper", "global-helper\n\nlocal-helper\ncommand-helper\n"},
		{"http.extraheader", "X-Global: once\nX-Local: once\n"},
		{"credential.usehttppath", "true\n"},
	} {
		output, runErr := Run(ctx, "-C", temporary, "config", "--get-all", tc.key)
		if runErr != nil || string(output) != tc.want {
			t.Fatalf("transport configuration %s did not retain precedence", tc.key)
		}
	}
	output, err := Run(ctx, "-C", temporary, "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(string(output)) != temporary {
		t.Fatal("original checkout layout leaked into temporary repository")
	}
}
