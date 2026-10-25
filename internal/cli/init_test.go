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

package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eminwux/kuketeams/internal/bootstrap"
)

func TestInitOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		created bool
		failure bool
	}{
		{name: "created", created: true}, {name: "existing"}, {name: "failed", failure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, ".kuke", "kuketeams.yaml")
			sentinel := errors.New("read configuration " + path + ": permission denied")
			cmd := newInitCommand(func() (string, error) { return home, nil },
				func(ctx context.Context, opts bootstrap.Options) (bootstrap.Result, error) {
					if ctx != t.Context() || opts.Root != filepath.Join(home, ".kuke") {
						t.Fatal("dependencies not forwarded")
					}
					if tc.failure {
						return bootstrap.Result{}, sentinel
					}
					return bootstrap.Result{
						Path:        path,
						Created:     tc.created,
						Diagnostics: []string{"discovery diagnostic"},
					}, nil
				})
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{})
			err := cmd.ExecuteContext(t.Context())
			if tc.failure {
				if !errors.Is(err, sentinel) || stdout.Len() != 0 || stderr.Len() != 0 {
					t.Fatalf("error=%v stdout=%q stderr=%q", err, &stdout, &stderr)
				}
				return
			}
			status := "Existing valid configuration"
			if tc.created {
				status = "Created configuration"
			}
			if err != nil || !strings.Contains(stdout.String(), status+": "+path) ||
				!strings.Contains(
					stdout.String(),
					"no team has been created or registered",
				) || stderr.String() != "discovery diagnostic\n" {
				t.Fatalf("error=%v stdout=%q stderr=%q", err, &stdout, &stderr)
			}
		})
	}
}

func TestInitHelpAndUnsupportedFlagsHaveNoEffects(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--build"}, {"--config", "elsewhere"}, {"extra"}} {
		cmd := newInitCommand(func() (string, error) {
			t.Fatal("home resolution must be lazy")
			return "", nil
		}, func(context.Context, bootstrap.Options) (bootstrap.Result, error) {
			t.Fatal("bootstrap must not run")
			return bootstrap.Result{}, nil
		})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if args[0] == "--help" {
			if err != nil || !strings.Contains(out.String(), "configuration only") {
				t.Fatalf("error=%v output=%q", err, &out)
			}
		} else if err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestInitHomeFailure(t *testing.T) {
	sentinel := errors.New("home unavailable")
	cmd := newInitCommand(func() (string, error) { return "", sentinel }, bootstrap.Run)
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "home directory") {
		t.Fatalf("error = %v", err)
	}
}
