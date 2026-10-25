# kuketeams

A standalone CLI and configuration library for composing agent teams into
[kukeon](https://github.com/eminwux/kukeon) blueprints and configs.

This repository will own the basic operator role, public harnesses, rendering
templates, and harness image definitions. Specialized roles and skills can come
from external repositories.

## Status

The configuration library and CLI help/version are available. Configuration
bootstrapping and team composition remain planned; the delivery steps are tracked
in [the implementation epic](https://github.com/eminwux/kuketeams/issues/1).

The planned workflow is:

- `kuketeams init`: bootstrap operator configuration under `~/.kuke` and compose
  the initial root team.
- `kuketeams apply -f kuketeam.yaml`: apply a `Team` declaration and register it
  as a `TeamEntry`.
- `kuketeams update`: regenerate and apply registered teams' blueprints and configs.
- `--build`: build selected harness images locally before applying resources.

Public harness images will also be built and published through GitHub Actions.
The tool will manage blueprints and configs; cell startup and lifecycle remain
under kukeon. The eventual goal is to replace `kuke team`.

## Build and test

Go 1.24 or later is required (the minimum version is declared in `go.mod`).

```sh
make kuketeams
./bin/kuketeams --help
./bin/kuketeams version                 # kuketeams dev
make kuketeams VERSION=v0.1.0          # release version
make test                             # vet and full race-enabled test suite
```

`GO` and `BINARY` can override the Go executable and output path. For direct
builds, inject version metadata with:

```sh
go build -ldflags '-X main.version=v0.1.0' -o bin/kuketeams ./cmd/kuketeams
```

Help and version need no configuration, Git settings, or daemon. Unknown
commands and flags fail with a diagnostic on stderr. The planned `init`, `apply`,
and `update` commands are not yet available.

## Configuration library

Import `github.com/eminwux/kuketeams/pkg/config`. Callers supply readers/writers
and own file handling; the library never discovers host configuration or fetches
sources. `Decode` validates its result, `Validate` also accepts models constructed
in code, and `Encode` validates before writing. Neither validation nor encoding
mutates the model.

```go
package example

import (
    "io"

    "github.com/eminwux/kuketeams/pkg/config"
)

func roundTrip(input io.Reader, output io.Writer) error {
    cfg, err := config.Decode(input)
    if err != nil {
        return err
    }
    if err := config.Validate(cfg); err != nil {
        return err
    }
    return config.Encode(output, cfg)
}
```

See [the global configuration fixture](testdata/config/global.yaml) for Git
identities/signing, named sources, roles, and harnesses. An omitted Git block and
empty defaults are valid. Selection lists preserve omission (`nil`, available for
later inheritance) versus explicit empty lists (non-nil, select none) on YAML
round trips. This package does not implement inheritance or composition yet.

The reader accepts exactly one `kuketeams.io/v1` / `TeamsConfig` YAML document.
It rejects unknown fields, duplicate keys, incorrect types (including null fields),
and trailing documents. String fields must be YAML strings; quote numeric-looking
revision values. Diagnostics identify fields without printing the document.
YAML merge keys are not schema fields and are rejected; ordinary acyclic aliases
are checked against the same schema as inline values.

Sources accept nonblank URLs, including `github.com/owner/repository`, and exactly
one nonblank `branch`, `tag`, or `commit`. Selectors are literal: `branch: v1.4.0`
selects a branch, not a tag. Definition paths use repository-relative slash syntax
and must stay within the source root; validation checks syntax and references
without accessing sources or signing-key files. An explicit role harness selects a
declared harness; otherwise at least one harness must be marked `default: true`.
Multiple default harnesses are supported.

## Pre-commit checks

Install [pre-commit](https://pre-commit.com/), then enable both hook stages:

```sh
pre-commit install --install-hooks
pre-commit install --hook-type commit-msg
pre-commit run --all-files
```

The hook versions and golangci-lint rules match kukeon: whitespace/newline and
YAML/JSON/TOML checks, private-key detection, Go formatting and module tidying,
golangci-lint, Prettier, gitlint, and Apache license headers for Go files. Go lint
fixes apply to new issues relative to `HEAD`, as in kukeon. Use a single-line
`type: imperative summary` commit message; `.gitlint` disables only the
body-required rule to match that policy.

The hook configuration, `.golangci.yml`, and Apache license were copied from
[kukeon at 58250581d283592cd59bda8c0d083f83940b6ae4](https://github.com/eminwux/kukeon/tree/58250581d283592cd59bda8c0d083f83940b6ae4).
Original license notices are retained. The local module prefix is adapted, and
kukeon-specific path exclusions and its multi-module `GOWORK=off` workaround are
omitted because this repository has a single module. The license hook relies on
pre-commit to supply Go filenames instead of passing a literal shell glob. The
golangci-lint configuration retains its upstream MIT notice and attribution.

## Repository development team

The checked-in [kuketeam.yaml](kuketeam.yaml) uses the existing `ProjectTeam`
format consumed by `kuke team init`. It follows kukeon's development roster,
with this repository's team name, and selects the dev, pm, and pr-reviewer roles
with Claude, OpenCode, and Codex harnesses.

With kukeon installed and initialized, run from this repository:

```sh
kuke team init
```

To build the selected images locally:

```sh
kuke team init --build
```

These commands compose and apply the development team's blueprints and configs;
they do not start agents. This roster is for the existing development workflow.
The future standalone `kuketeams apply` command will consume the new `Team`
format described in the epic.
