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

package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutable(t *testing.T) {
	for _, version := range []string{"dev", "v1.2.3-test"} {
		t.Run(version, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "kuketeams")
			args := []string{"build", "-o", binary}
			if version != "dev" {
				args = append(args, "-ldflags", "-X main.version="+version)
			}
			args = append(args, ".")
			build := exec.CommandContext(t.Context(), "go", args...)
			if output, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build: %v\n%s", err, output)
			}
			home := t.TempDir()
			for _, tc := range []struct {
				args []string
				want string
				fail bool
			}{
				{[]string{"--help"}, "Usage:", false},
				{[]string{"help"}, "Usage:", false},
				{[]string{"init", "--help"}, "No cells are created or started", false},
				{[]string{"help", "init"}, "No cells are created or started", false},
				{[]string{"init", "--build"}, "unknown flag", true},
				{[]string{"version"}, "kuketeams " + version + "\n", false},
				{[]string{"unknown"}, "unknown command", true},
				{[]string{"--unknown"}, "unknown flag", true},
				{[]string{"version", "--unknown"}, "unknown flag", true},
			} {
				checkCommand(t, binary, home, tc.args, tc.want, tc.fail)
			}
			entries, err := os.ReadDir(home)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatal("commands wrote configuration or other files")
			}
		})
	}
}

func checkCommand(t *testing.T, binary, home string, args []string, want string, fail bool) {
	t.Helper()
	command := exec.CommandContext(t.Context(), binary, args...)
	// A temporary home and empty executable search path prevent using
	// operator configuration or invoking Git/daemon subprocesses.
	command.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + home}
	command.Dir = home
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if fail {
		if err == nil || !strings.Contains(stderr.String(), want) || stdout.Len() != 0 {
			t.Fatalf("%v: error=%v stdout=%q stderr=%q", args, err, stdout.String(), stderr.String())
		}
	} else if err != nil || !strings.Contains(stdout.String(), want) || stderr.Len() != 0 {
		t.Fatalf("%v: error=%v stdout=%q stderr=%q", args, err, stdout.String(), stderr.String())
	}
}

func TestInitExecutable(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "kuketeams")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	home := t.TempDir()
	// Run outside HOME and outside a project checkout; PATH isolates missing Git.
	cwd := t.TempDir()
	for _, invalid := range []bool{false, false, true} {
		path := filepath.Join(home, ".kuke", "kuketeams.yaml")
		if invalid {
			if err := os.WriteFile(path, []byte("kind: Wrong\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		command := exec.CommandContext(t.Context(), binary, "init")
		command.Dir = cwd
		command.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + home}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		want := "materialize source"
		if invalid {
			want = path
		}
		if err == nil || !strings.Contains(stderr.String(), want) || stdout.Len() != 0 {
			t.Fatalf("invalid=%v error=%v stdout=%q stderr=%q", invalid, err, &stdout, &stderr)
		}
		if _, err = os.Stat(filepath.Join(home, ".kuke", "kuketeam.d", "root.yaml")); !os.IsNotExist(err) {
			t.Fatal("registered before successful application")
		}
	}

	entries, err := os.ReadDir(cwd)
	if err != nil || len(entries) != 0 {
		t.Fatalf("working directory changed: %v, %v", entries, err)
	}
	entries, err = os.ReadDir(home)
	if err != nil || len(entries) != 1 || entries[0].Name() != ".kuke" {
		t.Fatalf("unexpected home artifacts: %v, %v", entries, err)
	}
	entries, err = os.ReadDir(filepath.Join(home, ".kuke"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "kuketeams.yaml" {
		t.Fatalf("unexpected config artifacts: %v, %v", entries, err)
	}
}
