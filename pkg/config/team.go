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

package config

import (
	"errors"
	"io"
	"slices"
)

const (
	KindTeam      = "Team"
	KindTeamEntry = "TeamEntry"
)

// Root returns the initial authored declaration. Selections remain omitted so
// later updates inherit the operator's current defaults.
func Root() *Team {
	return &Team{APIVersion: APIVersion, Kind: KindTeam, Metadata: Metadata{Name: "root"}}
}

// Resolve applies whole-list inheritance, validates effective references, and
// retains the original declaration. Explicit empty slices select none.
func Resolve(cfg *TeamsConfig, team *Team) (Defaults, error) {
	if err := Validate(cfg); err != nil {
		return Defaults{}, err
	}
	if team == nil || team.APIVersion != APIVersion || team.Kind != KindTeam || !ValidName(team.Metadata.Name) {
		return Defaults{}, errors.New("team: require kuketeams.io/v1 Team and a valid metadata.name")
	}
	d := team.Spec
	if d.Sources == nil {
		d.Sources = cfg.Spec.Defaults.Sources
	}
	if d.Roles == nil {
		d.Roles = cfg.Spec.Defaults.Roles
	}
	if d.Harnesses == nil {
		d.Harnesses = cfg.Spec.Defaults.Harnesses
	}
	return d, validateDefaults(&d)
}

func DecodeTeam(r io.Reader) (*Team, error) {
	var team Team
	if err := decodeDocument(r, &team); err != nil {
		return nil, err
	}
	if team.APIVersion != APIVersion || team.Kind != KindTeam || !ValidName(team.Metadata.Name) {
		return nil, errors.New("team: require kuketeams.io/v1 Team and a valid metadata.name")
	}
	return &team, nil
}

// Entry copies the authored declaration without materializing inherited fields.
// It must only be persisted after application has succeeded.
func Entry(team *Team) (*TeamEntry, error) {
	if team == nil || team.APIVersion != APIVersion || team.Kind != KindTeam || !ValidName(team.Metadata.Name) {
		return nil, errors.New("team: invalid declaration")
	}
	return &TeamEntry{
		APIVersion: APIVersion,
		Kind:       KindTeamEntry,
		Metadata:   team.Metadata,
		Spec:       cloneSelections(team.Spec),
	}, nil
}

func DecodeEntry(r io.Reader) (*TeamEntry, error) {
	var entry TeamEntry
	if err := decodeDocument(r, &entry); err != nil {
		return nil, err
	}
	if entry.APIVersion != APIVersion || entry.Kind != KindTeamEntry || !ValidName(entry.Metadata.Name) {
		return nil, errors.New("entry: require kuketeams.io/v1 TeamEntry and a valid metadata.name")
	}
	return &entry, nil
}

func (e TeamEntry) Team() *Team {
	return &Team{APIVersion: e.APIVersion, Kind: KindTeam, Metadata: e.Metadata, Spec: cloneSelections(e.Spec)}
}

func cloneSelections(d Defaults) Defaults {
	d.Sources = slices.Clone(d.Sources)
	d.Harnesses = slices.Clone(d.Harnesses)
	d.Roles = slices.Clone(d.Roles)
	for i := range d.Roles {
		if d.Roles[i].Needs != nil {
			needs := *d.Roles[i].Needs
			needs.Image = slices.Clone(needs.Image)
			d.Roles[i].Needs = &needs
		}
	}
	return d
}
