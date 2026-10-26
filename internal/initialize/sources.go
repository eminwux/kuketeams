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
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/eminwux/kuketeams/internal/gitremote"
	"github.com/eminwux/kuketeams/pkg/config"
)

const (
	sha256ObjectFormat = "sha256"
	sha256CommitLength = 64
)

// Fetch only sources used for definitions, at their declared revision. Temporary
// checkouts do not become runtime clone paths and are removed after composition.
func materialize(ctx context.Context, d config.Defaults) (map[string]fs.FS, func(), error) {
	return materializeSources(ctx, d, false)
}

func materializeSources(ctx context.Context, d config.Defaults, project bool) (map[string]fs.FS, func(), error) {
	root, err := os.MkdirTemp("", "kuketeams-sources-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	needed := map[string]bool{}
	for _, source := range d.Sources {
		if project && source.Name == "project" {
			needed[source.Name] = true
		}
	}
	for _, role := range d.Roles {
		needed[role.Source] = true
		for _, harness := range d.Harnesses {
			if role.Harness == harness.Name || (role.Harness == "" && harness.Default) {
				needed[harness.Source] = true
			}
		}
	}
	sources := map[string]fs.FS{}
	for i, source := range d.Sources {
		if !needed[source.Name] {
			continue
		}
		dir := filepath.Join(root, strconv.Itoa(i))
		url := source.URL
		if strings.HasPrefix(url, "github.com/") {
			url = "https://" + url
		}
		ref := source.Commit
		if source.Branch != "" {
			ref = "refs/heads/" + source.Branch
		}
		if source.Tag != "" {
			ref = "refs/tags/" + source.Tag
		}
		// Full commit IDs identify the repository's object format. Pin it so
		// Git's configured default cannot select an incompatible hash format.
		objectFormat := "sha1"
		if len(source.Commit) == sha256CommitLength {
			objectFormat = sha256ObjectFormat
		}
		commands := [][]string{
			{"init", "--quiet", "--object-format=" + objectFormat, dir},
			{"-C", dir, "remote", "add", "--", "origin", url},
			{"-C", dir, "fetch", "--quiet", "--depth=1", "--", "origin", ref},
			{"-C", dir, "checkout", "--quiet", "--detach", "FETCH_HEAD"},
		}
		for _, args := range commands {
			if _, runErr := gitremote.Run(ctx, args...); runErr != nil {
				cleanup()
				// Git stderr and URLs may contain credentials; never forward them.
				return nil, nil, fmt.Errorf(
					"materialize source %q: %w",
					source.Name, runErr,
				)
			}
		}
		sources[source.Name] = os.DirFS(dir)
	}
	return sources, cleanup, nil
}
