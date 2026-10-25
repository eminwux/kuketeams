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
	"fmt"
	"os"
	"path/filepath"

	"github.com/eminwux/kuketeams/internal/bootstrap"
	"github.com/spf13/cobra"
)

// newInitCommand keeps home resolution and result formatting in the CLI. The
// injected functions let tests exercise outcomes without changing operator HOME.
func newInitCommand(
	homeDir func() (string, error),
	run func(context.Context, bootstrap.Options) (bootstrap.Result, error),
) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize operator configuration only",
		Long:  "Initialize ~/.kuke/kuketeams.yaml with default selections and available global Git settings.\nExisting valid configuration is preserved. Edit this file to customize selections.\nThis initializes configuration only; no team is created or registered.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := homeDir()
			if err != nil {
				return fmt.Errorf("resolve operator home directory: %w", err)
			}
			result, err := run(cmd.Context(), bootstrap.Options{Root: filepath.Join(home, ".kuke")})
			if err != nil {
				return err
			}
			for _, diagnostic := range result.Diagnostics {
				if _, err = fmt.Fprintln(cmd.ErrOrStderr(), diagnostic); err != nil {
					return err
				}
			}
			status := "Existing valid configuration"
			if result.Created {
				status = "Created configuration"
			}
			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"%s: %s\nConfiguration only; no team has been created or registered.\n",
				status,
				result.Path,
			)
			return err
		},
	}
}

func defaultInitCommand() *cobra.Command {
	return newInitCommand(os.UserHomeDir, bootstrap.Run)
}
