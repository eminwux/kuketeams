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

// Package initialize applies the root team's configuration without managing cells.
package initialize

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/eminwux/kukeon/pkg/api/kukeonv1"
	"github.com/eminwux/kuketeams/internal/bootstrap"
	"github.com/eminwux/kuketeams/pkg/compose"
	"github.com/eminwux/kuketeams/pkg/config"
	"gopkg.in/yaml.v3"
)

const (
	rootName         = "root"
	resourcesPerPair = 2 // One blueprint and one config.
)

// Options supplies operator paths and optional test/embedding dependencies.
// An injected Client remains caller-owned; otherwise Run opens and closes it.
type Options struct {
	Root, Home, Host, Configuration string
	Sources                         map[string]fs.FS
	Client                          Client
}

type Result struct {
	Bootstrap bootstrap.Result
	EntryPath string
	Resources int
}

// Run preserves authored configuration and the last successfully accepted entry.
// Partial daemon application is not rolled back; a rerun converges the same team.
func Run(ctx context.Context, opts Options) (Result, error) {
	boot, err := bootstrap.Run(ctx, bootstrap.Options{Root: opts.Root})
	result := Result{Bootstrap: boot}
	if err != nil {
		return result, err
	}
	entryPath := filepath.Join(opts.Root, "kuketeam.d", "root.yaml")
	team, err := readTeam(entryPath)
	if err != nil {
		return result, err
	}
	effective, err := config.Resolve(boot.Config, team)
	if err != nil {
		return result, err
	}
	sources := opts.Sources
	if sources == nil {
		var cleanup func()
		sources, cleanup, err = materialize(ctx, effective)
		if err != nil {
			return result, err
		}
		defer cleanup()
	}
	composed, err := compose.Compose(boot.Config, team, compose.Options{
		Sources: sources, HomeDir: opts.Home, TeamDir: filepath.Join(opts.Root, "teams", rootName),
	})
	if err != nil {
		return result, fmt.Errorf("preflight composition: %w", err)
	}
	if len(composed.Pairs) == 0 {
		return result, errors.New("preflight: root team selects no role/harness pairs")
	}
	// Encode and validate local entry publication before any daemon mutation.
	entryBytes, err := yaml.Marshal(composed.Entry)
	if err != nil {
		return result, fmt.Errorf("encode root TeamEntry: %w", err)
	}
	stage, err := stageEntry(entryPath, entryBytes)
	if err != nil {
		return result, err
	}
	defer func() { _ = os.Remove(stage) }()
	client := opts.Client
	if client == nil {
		host, hostErr := resolveHost(opts)
		if hostErr != nil {
			return result, hostErr
		}
		client, err = kukeonv1.Dial(ctx, host)
		if err != nil {
			return result, fmt.Errorf("kukeond connection configuration: %w", err)
		}
		defer func() { _ = client.Close() }()
	}
	if err = Apply(ctx, client, composed); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = os.Rename(stage, entryPath); err != nil {
		return result, fmt.Errorf(
			"resources applied but root registration failed at %s: %w; rerun kuketeams init",
			entryPath,
			err,
		)
	}
	result.EntryPath, result.Resources = entryPath, len(composed.Pairs)*resourcesPerPair
	return result, nil
}

func readTeam(path string) (*config.Team, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return config.Root(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read root registration %s: %w", path, err)
	}
	entry, err := config.DecodeEntry(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("read root registration %s: %w", path, err)
	}
	if entry.Metadata.Name != rootName {
		return nil, errors.New("root registration must name team root")
	}
	return entry.Team(), nil
}

func stageEntry(path string, data []byte) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("prepare root registration: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".root-*.yaml")
	if err != nil {
		return "", fmt.Errorf("stage root registration: %w", err)
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("stage root registration: %w", err)
	}
	return f.Name(), nil
}
