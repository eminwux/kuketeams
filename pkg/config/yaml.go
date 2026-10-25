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
	"fmt"
	"io"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Decode accepts exactly one strict YAML document and returns a validated model.
func Decode(r io.Reader) (*TeamsConfig, error) {
	input := &errorReader{Reader: r}
	decoder := yaml.NewDecoder(input)
	var doc yaml.Node
	err := decoder.Decode(&doc)
	if input.err != nil {
		return nil, fmt.Errorf("config: read YAML: %w", input.err)
	}
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("config: empty YAML input")
		}
		return nil, fmt.Errorf("config: invalid YAML: %w", err)
	}
	if len(doc.Content) != 1 {
		return nil, errors.New("config: empty YAML document")
	}
	if err := checkNode(doc.Content[0], reflect.TypeOf(TeamsConfig{}), "", map[*yaml.Node]bool{}); err != nil {
		return nil, err
	}
	var cfg TeamsConfig
	if err := doc.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	var extra yaml.Node
	err = decoder.Decode(&extra)
	if input.err != nil {
		return nil, fmt.Errorf("config: read trailing YAML: %w", input.err)
	}
	if !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("config: trailing YAML: %w", err)
		}
		return nil, errors.New("config: expected exactly one YAML document")
	}
	if err := Validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// checkNode validates the schema before decoding, retaining full field paths in
// diagnostics and preventing aliases from bypassing unknown/duplicate-key checks.
func checkNode(n *yaml.Node, typ reflect.Type, field string, active map[*yaml.Node]bool) error {
	if active[n] {
		return fmt.Errorf("%s: cyclic YAML alias", location(field))
	}
	active[n] = true
	defer delete(active, n)
	if n.Kind == yaml.AliasNode {
		return checkNode(n.Alias, typ, field, active)
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	// The schema uses only structs, slices, strings, and booleans.
	switch typ.Kind() { //nolint:exhaustive // Other kinds cannot occur in this schema.
	case reflect.Struct:
		if n.Kind != yaml.MappingNode || n.Tag != "!!map" {
			return fmt.Errorf("%s: expected mapping", location(field))
		}
		fields := map[string]reflect.Type{}
		for i := range typ.NumField() {
			f := typ.Field(i)
			fields[strings.Split(f.Tag.Get("yaml"), ",")[0]] = f.Type
		}
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			key, value := n.Content[i], n.Content[i+1]
			name := key.Value
			next := name
			if field != "" {
				next = field + "." + name
			}
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return fmt.Errorf("%s: expected string field name (line %d)", location(field), key.Line)
			}
			if seen[name] {
				return fmt.Errorf("%s: duplicate key (line %d)", next, key.Line)
			}
			seen[name] = true
			childType, ok := fields[name]
			if !ok {
				return fmt.Errorf("%s: unknown field (line %d)", next, key.Line)
			}
			if err := checkNode(value, childType, next, active); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode || n.Tag != "!!seq" {
			return fmt.Errorf("%s: expected list", location(field))
		}
		for i, child := range n.Content {
			if err := checkNode(child, typ.Elem(), fmt.Sprintf("%s[%d]", field, i), active); err != nil {
				return err
			}
		}
	case reflect.String:
		if n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
			return fmt.Errorf("%s: expected string", location(field))
		}
		if strings.HasPrefix(field, "spec.defaults.roles[") && strings.HasSuffix(field, ".harness") {
			if err := required(field, n.Value); err != nil {
				return err
			}
		}
	case reflect.Bool:
		if n.Kind != yaml.ScalarNode || n.Tag != "!!bool" {
			return fmt.Errorf("%s: expected boolean", location(field))
		}
	}
	return nil
}

func location(field string) string {
	if field == "" {
		return "config"
	}
	return field
}

// Encode validates before writing and preserves semantic selection-list values.
// Neither validation nor encoding mutates cfg.
func Encode(w io.Writer, cfg *TeamsConfig) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	output := &errorWriter{Writer: w}
	encoder := yaml.NewEncoder(output)
	const yamlIndent = 2
	encoder.SetIndent(yamlIndent)
	err := encoder.Encode(cfg)
	if err == nil {
		err = encoder.Close()
	}
	if output.err != nil {
		return fmt.Errorf("config: write YAML: %w", output.err)
	}
	if err != nil {
		return fmt.Errorf("config: encode YAML: %w", err)
	}
	return nil
}

// The YAML library stringifies stream errors. Capture their original values so
// callers can still inspect them with errors.Is and errors.As.
type errorReader struct {
	io.Reader

	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && !errors.Is(err, io.EOF) && r.err == nil {
		r.err = err
	}
	return n, err
}

type errorWriter struct {
	io.Writer

	err error
}

func (w *errorWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if n < len(p) && err == nil {
		err = io.ErrShortWrite
	}
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
}
