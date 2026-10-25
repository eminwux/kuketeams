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

// Package cli owns command construction and output, independently of process exit.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// NewCommand constructs an isolated command tree with explicit output and version.
func NewCommand(stdout, stderr io.Writer, version string) *cobra.Command {
	root := &cobra.Command{
		Use:               "kuketeams",
		Short:             "Compose agent teams with kuketeams",
		Args:              cobra.NoArgs,
		SilenceUsage:      true,
		SilenceErrors:     true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE:              func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show the version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "kuketeams %s\n", version)
			return err
		},
	})
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		RunE: func(_ *cobra.Command, args []string) error {
			target, remaining, err := root.Find(args)
			if err != nil {
				return err
			}
			if len(remaining) != 0 {
				return fmt.Errorf("unknown command %q", remaining[0])
			}
			return target.Help()
		},
	})
	root.AddCommand(defaultInitCommand())
	return root
}
