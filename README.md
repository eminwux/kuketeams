# kuketeams

A standalone CLI and configuration library for composing agent teams into
[kukeon](https://github.com/eminwux/kukeon) blueprints and configs.

This repository will own the basic operator role, public harnesses, rendering
templates, and harness image definitions. Specialized roles and skills can come
from external repositories.

## Status

The configuration library, CLI help/version, and configuration-only `init` are
available. Team composition remains planned; the delivery steps are tracked
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
commands and flags fail with a diagnostic on stderr. The planned `apply` and
`update` commands are not yet available.

### Linux distribution builds and Actions artifacts

Distribution builds require Linux amd64, Go, Git, Make, Bash, jq, GNU coreutils,
and binutils (`readelf`). These tools are available on the Ubuntu CI runner.
Run the same entrypoints locally:

```sh
make test
version="dev-$(git rev-parse --short=12 HEAD)"
make dist GOOS=linux GOARCH=amd64 VERSION="$version"
make dist GOOS=linux GOARCH=arm64 VERSION="$version"
```

`make dist` uses `make kuketeams` with `-trimpath` and `CGO_ENABLED=0`, then
generates and verifies the binary, relative SHA-256 manifest, and `build-info.json`
under `bin/dist/linux-amd64/` or `bin/dist/linux-arm64/`. The JSON records the
full checked-out commit SHA, OS, architecture, Go toolchain, and injected version.
Omitting `VERSION` retains the local `dev` default. Both binaries have their ELF
machine types and embedded Go build settings checked. On Linux amd64, help and
version run with an empty environment and temporary home, which must remain empty.
The arm64 output is cross-built and structurally checked; it is not runtime-tested.
Tests run natively, with the race detector, before either CI build starts.

The [Binary CI workflow](https://github.com/eminwux/kuketeams/actions/workflows/build.yaml)
runs on PRs targeting `main`, pushes to `main`, and manual dispatch, with read-only
repository permissions. Open a successful run and download its artifact for your
architecture: `kuketeams-linux-amd64-` or `kuketeams-linux-arm64-` followed by the
first 12 characters of the **built** commit SHA. Each artifact contains exactly
the corresponding binary, its `.sha256` file, and `build-info.json`. Artifacts
expire after **14 days**. CI downloads each uploaded artifact and repeats its
checks, including native help/version, to validate the upload/download path.

These are development Actions artifacts, not tagged GitHub Releases. PR artifacts
may build GitHub's merge commit; `build-info.json` identifies the actual source,
and the CLI reports `dev-` followed by that commit's first 12 characters. Binary
CI does not publish harness/container images; image publication is a separate
delivery step.

For example, extract the Linux amd64 artifact into an empty directory, open a
terminal there, and verify it before use:

```sh
sha256sum --check kuketeams-linux-amd64.sha256
cat build-info.json
chmod +x kuketeams-linux-amd64
./kuketeams-linux-amd64 --help
./kuketeams-linux-amd64 version
```

On a Linux arm64 machine, use its artifact instead:

```sh
sha256sum --check kuketeams-linux-arm64.sha256
cat build-info.json
chmod +x kuketeams-linux-arm64
./kuketeams-linux-arm64 --help
./kuketeams-linux-arm64 version
```

Extraction may lose executable permissions, so keep the `chmod` step. To install
the verified binary for your own user (choose the command for your machine):

```sh
mkdir -p "$HOME/.local/bin"
# Linux amd64:
install -m 0755 kuketeams-linux-amd64 "$HOME/.local/bin/kuketeams"
# Linux arm64:
install -m 0755 kuketeams-linux-arm64 "$HOME/.local/bin/kuketeams"
```

Add `$HOME/.local/bin` to your shell's `PATH` if needed. Installation is optional;
help/version create no operator configuration.

## Initialize operator configuration

Run `kuketeams init` from any directory, with no project checkout or daemon:

```sh
./bin/kuketeams init
```

This creates `~/.kuke/kuketeams.yaml` with the kuketeams source on `main`, the
`operator` role (requesting the `go` image), and the default `codex` harness. Edit
this file to customize configuration. This delivery initializes configuration
only: it does not fetch assets, compose or register a team, generate manifests,
or build images. `init --build` is rejected until build support is implemented.

Available **global** Git settings seed separate author/committer identities, the
signing key, commit/tag signing selections, and allowed signers. Missing values
are optional; incomplete identities and signing enabled without a key are omitted
with diagnostics. If Git is unavailable, initialization proceeds with defaults and
a diagnostic. Malformed Git settings and invalid signing booleans fail without
publishing configuration. Key and allowed-signers paths are preserved as configured;
bootstrap does not read keys, check their existence, or sign anything.

Reruns validate and preserve existing configuration byte-for-byte, including
comments, independent identities, and permissions, without rediscovering Git.
Invalid YAML, wrong kind/version, and filesystem failures produce path-qualified
errors and a nonzero exit. Newly created directories use `0700` and the file uses
`0600`. Existing directory permissions are preserved. Publication hard-links a
completed temporary file in the same directory to the final name, atomically
refusing to overwrite a competing creator; the winner is then read and validated.
This requires a local filesystem supporting hard links; unsupported publication
fails safely and removes temporary files. No configuration-path flag or environment
override is provided.

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
