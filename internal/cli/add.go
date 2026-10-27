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
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eminwux/kuketeams/internal/initialize"
	"github.com/spf13/cobra"
)

func newAddCommand(
	homeDir func() (string, error),
	run func(context.Context, initialize.AddOptions) (initialize.Result, error),
) *cobra.Command {
	var opts initialize.AddOptions
	cmd := &cobra.Command{
		Use:   "add NAME --source REPOSITORY",
		Short: "Apply and register a project team from a Git source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, name := range []string{"branch", "tag", "commit"} {
				if cmd.Flags().Changed(name) {
					value, _ := cmd.Flags().GetString(name)
					if strings.TrimSpace(value) == "" {
						return errors.New("add: explicit revision selector must not be empty")
					}
				}
			}
			home, err := homeDir()
			if err != nil {
				return fmt.Errorf("resolve operator home directory: %w", err)
			}
			opts.Name, opts.Home, opts.Root = args[0], home, filepath.Join(home, ".kuke")
			result, err := run(cmd.Context(), opts)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"Applied %d resources; registered team %s: %s\nNo cells created or started.\n",
				result.Resources,
				opts.Name,
				result.EntryPath,
			)
			return err
		},
	}
	cmd.Flags().StringVar(&opts.Source, "source", "", "Project Git remote (SCP-style SSH, ssh://, or HTTPS)")
	cmd.Flags().StringVar(&opts.Branch, "branch", "", "Select a branch (default: remote HEAD branch)")
	cmd.Flags().StringVar(&opts.Tag, "tag", "", "Select a tag")
	cmd.Flags().StringVar(&opts.Commit, "commit", "", "Select a full commit object ID")
	cmd.Flags().StringVar(&opts.Host, "host", "", "kukeond Unix endpoint (overrides KUKEON_HOST and kuke.yaml)")
	cmd.Flags().StringVar(&opts.Configuration, "configuration", "", "kuke ClientConfiguration path")
	_ = cmd.MarkFlagRequired("source")
	cmd.MarkFlagsMutuallyExclusive("branch", "tag", "commit")
	return cmd
}

func defaultAddCommand() *cobra.Command { return newAddCommand(os.UserHomeDir, initialize.Add) }
