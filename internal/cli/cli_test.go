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

package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/eminwux/kuketeams/internal/cli"
)

func TestCommands(t *testing.T) {
	for _, tc := range []struct {
		name          string
		args          []string
		want, wantErr string
	}{
		{"root", nil, "Usage:", ""},
		{"help", []string{"help"}, "Usage:", ""},
		{"help flag", []string{"--help"}, "Usage:", ""},
		{"short help", []string{"-h"}, "Usage:", ""},
		{"version", []string{"version"}, "kuketeams test-version\n", ""},
		{"unknown help topic", []string{"help", "unknown"}, "", "unknown command"},
		{"help version", []string{"help", "version"}, "Show the version", ""},
		{"version help", []string{"version", "--help"}, "Show the version", ""},
		{"unknown", []string{"unknown"}, "", "unknown command"},
		{"unknown flag", []string{"--unknown"}, "", "unknown flag"},
		{"version unknown flag", []string{"version", "--unknown"}, "", "unknown flag"},
		{"version argument", []string{"version", "extra"}, "", "unknown command"},
		{"init help", []string{"init", "--help"}, "configuration only", ""},
		{"init build unsupported", []string{"init", "--build"}, "", "unknown flag"},
		{"no placeholder apply", []string{"apply"}, "", "unknown command"},
		{"no placeholder update", []string{"update"}, "", "unknown command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			cmd := cli.NewCommand(&out, &stderr, "test-version")
			cmd.SetArgs(append([]string{}, tc.args...))
			err := cmd.Execute()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("stdout = %q, want %q", out.String(), tc.want)
			}
			if tc.wantErr != "" && out.Len() != 0 {
				t.Fatalf("unexpected stdout: %q", out.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("CLI must leave error reporting to entrypoint: %q", stderr.String())
			}
		})
	}
}

func TestHelpIsDeterministic(t *testing.T) {
	var first, second bytes.Buffer
	for _, out := range []*bytes.Buffer{&first, &second} {
		cmd := cli.NewCommand(out, out, "dev")
		cmd.SetArgs([]string{"--help"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	if first.String() != second.String() {
		t.Fatal("help changed between invocations")
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestVersionOutputError(t *testing.T) {
	sentinel := errors.New("broken output")
	cmd := cli.NewCommand(failingWriter{sentinel}, &bytes.Buffer{}, "dev")
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v", err)
	}
}
