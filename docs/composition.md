# Structural operator composition

`pkg/compose.Compose` produces blueprints, configs, and application requirements.
It reads caller-supplied definition filesystems. It does not fetch sources, read
credential values, contact kukeond, apply resources, register a team, or start a
cell. CLI `init` still performs configuration bootstrap; daemon integration is
issue #13. Live image/authentication/agent loading verification belongs to #14.

## Selections and persistence

`TeamsConfig.spec.defaults` and `Team.spec` share `sources`, `roles`, and
`harnesses`. An omitted list inherits the corresponding global default. An
explicit list replaces it; `[]` selects none. References are checked against the
effective lists. Each source has exactly one of `branch`, `tag`, or `commit`.
Each role selects its explicit `harness`, or all declared harnesses marked
`default: true`. Unselected definitions are not loaded or rendered.

```yaml
apiVersion: kuketeams.io/v1
kind: Team
metadata:
  name: root
# All selections omitted: inherit the operator's defaults.
```

`config.Root()` returns this declaration. `config.Entry(team)` creates a
`TeamEntry` with the same `metadata` and `spec`, changing only `kind` to
`TeamEntry`. The declaration is copied; effective defaults are not persisted.
`config.DecodeTeam` / `DecodeEntry` read strict single-document YAML;
`yaml.Marshal` writes either model and preserves omitted versus explicit empty
lists. `entry.Team()` reconstructs accepted input without the original file.
Only persist the candidate entry after application succeeds.

Authored `roles[].needs.image` contains **capability names**, never image URLs.
These additions cannot erase requirements in `Role.spec.needs.image`. The
selected harness is also a mandatory image capability. One catalog image must
satisfy the complete set. Catalog order breaks ties; a root-capable image is not
selected unless `root` is requested. The team name `root` grants no privilege.
Published registry references are read from `harnesses/images.yaml` in the
selected harness's source. No registry lookup is made. `Team` and `TeamEntry`
have no resolved-image field: the selected catalog entry is returned in
`Pair.Image`, and its published reference is bound to the blueprint container.
An update re-resolves it from the current definitions/catalog. Local builds
remain epic step 4.

The inspected kukeon revision
[`58250581d283592cd59bda8c0d083f83940b6ae4`](https://github.com/eminwux/kukeon/tree/58250581d283592cd59bda8c0d083f83940b6ae4)
still uses `ProjectTeam` and a locator-only `TeamEntry`; neither has a resolved
image field. These standalone models implement epic #1's greenfield contract,
without retaining that input model or inventing redundant image state.

## Runtime repository and parameter bindings

A source is the single declaration of its URL and branch/tag/commit. Optional
`sources[].clonePath` requests a runtime clone at an absolute in-cell destination;
omission requests no runtime clone. Definition materialization on the host does
not itself request cloning inside a workload. Bootstrap opts the role source in:

```yaml
apiVersion: kuketeams.io/v1
kind: TeamsConfig
spec:
  defaults:
    sources:
      - name: kuketeams
        url: github.com/eminwux/kuketeams
        branch: main
        clonePath: /home/user/kuketeams
    roles:
      - name: operator
        source: kuketeams
        path: roles/operator
        needs:
          image: [go]
    harnesses:
      - name: codex
        source: kuketeams
        path: harnesses/codex
        default: true
```

Existing configurations are preserved by bootstrap. Add `clonePath` to required
runtime sources in older configurations before composing. Source names used for
clones and absolute Unix destinations must be safe in copied shell templates;
clone destinations must be unique. `branch`, `tag`, and `commit` remain mutually
exclusive. The `github.com/owner/repo` URL shorthand becomes HTTPS in runtime
config fills; authored declarations retain their spelling.

The selected role's source and every `Role.spec.needs.repos` slot must resolve to
an effective source with `clonePath`. Other effective sources with `clonePath`
also request clones; sources without it contribute no runtime repository slot.
Team source overrides replace the whole global list, including clone destinations
and revisions, and authored `TeamEntry` round trips preserve these declarations.
No project URL is inferred from the team name or definition source. Only a role
declaring the `project` repo slot gets a project launch directory; otherwise the
working directory is the selected role tree.

Destinations are bound in blueprint repository slots; URLs and revisions are
bound through `CellConfig.spec.repos`. Source branches map to `branch`, and tags
or commits map to `ref`. All three harness launchers preserve this checkout:
they do not switch branches or pull newer revisions during launch.

`TeamsConfig.spec.parameters` is a string-to-string map for declared scalar
parameters. Each composition binds only keys its blueprint declares. Required
parameters without defaults must be supplied. Unknown keys fail against the union
of selected blueprints; a key declared only by an unselected harness also fails.
`AGENT_NAME` defaults to the full resource identity; legacy `ROLE` parameter
requests receive the selected role path. Parameters must not hold credentials:
use structural secret bindings instead. Undeclared role parameters, unsupported
mount selectors, missing settings/assets/partials, and unsupported harness
settings fail composition with the role/harness context.

Role launchers use `make -C <role-source-target> <harness.makeTarget>
ROLE=<selected-role-path>`. Claude's settings path is relative to that role.
Codex keeps sandbox/approval settings (root images retain their explicit root
launch behavior). OpenCode materializes the role's configuration instead of
overwriting it with a generated heredoc, and only requests skipped permissions
when the selected role explicitly declares `permissions: skip`.

Host Git author/committer identities may be copied into the container. Signing
paths are mapped into `/home/user/.ssh` only when the role explicitly requests
an `ssh` mount; composition requires `Options.HomeDir` for that host mount and
never checks or reads key files. No SSH mount or Git signing prerequisite is
introduced for the base operator.

## Secret declarations

Only selected role/harness requirements contribute bindings. The shipped
operator has no `gh` or `gh-token` requirement. Claude and Codex can reuse an
existing authenticated session and declare no mandatory token. OpenCode's
selected provider requires `openrouter-api-key`:

```yaml
# Add under TeamsConfig.spec when selecting OpenCode for the operator.
secrets:
  - name: openrouter-api-key # identifier in Role.spec.harnesses.opencode.secrets
    secretRef:
      name: provider-account
      realm: auth
      space: shared
      stack: operators
    envName: OPENROUTER_API_KEY
    # Optional fallback if application finds no runtime secret:
    source:
      env: OPERATOR_OPENROUTER_KEY
      # Alternatively: file: /absolute/path/to/private/provider-key
```

`name`, scoped `secretRef`, and `envName` are required and independent. Explicit
bindings override the template's historical identifier-to-env-name convention.
`source` is optional; when present, it declares exactly one env **variable name**
or absolute file **path**. There is no inline value field. A valid reference
composes without a local value, source file, environment variable, or runtime
secret lookup. Deeper scope coordinates require their parents; cell-scoped
secret references are supported by the existing runtime schema.

Mandatory `Role.spec.needs.secrets` and selected
`Role.spec.harnesses.<name>.secrets` are combined. Other harness requirements are
ignored. Missing bindings, missing template secret slots, destination conflicts,
and unsupported destination modes fail before any result is returned. This
slice supports environment destinations; file destinations require a subsequent
contract extension. Extra configured bindings impose no requirement and are not
included in output. Image capability selection supplies no authentication.

Composition emits secret **references** in configs and consumption slots in
blueprints. It never reads env/file values, emits `Secret` documents, removes
requirements because a local value is absent, or copies creation sources into
daemon documents. Application (#13) must check runtime existence and distinguish
lookup failures from missing secrets, reuse existing values, and read an explicit
creation source only when needed. Keep resolved bytes separate from these models.
Specialized-role onboarding and remediation remain epic step 5.

## Library handoff and validation limits

```go
result, err := compose.Compose(cfg, config.Root(), compose.Options{
    Sources: map[string]fs.FS{"kuketeams": os.DirFS(checkout)},
    Scope:   compose.Scope{Realm: "default", Space: "default", Stack: "default"},
    TeamDir: "/operator/.kuke/teams/root", // required by Claude's seed declaration
})
```

The caller resolves/fetches/pins the effective sources and supplies each named
filesystem. Definition paths are relative to those roots. Harness manifests name
their template; all sibling `*.tmpl.yaml` partials are parsed with `upper` and
pipeline-compatible `replace`. Missing template context fails rather than
silently rendering `<no value>`.

`Result.Pairs` follows role order then harness order. Each pair exposes:

- `Blueprint` and `Config`: the existing public kukeon `v1beta1` types, using
  `<team>-<role-selection>-<harness>` names, matching scope and `kukeon.io/team`
  labels. Defaults are realm/space/stack `default`; initial config identity is
  `root-operator-codex`.
- `Image`: the resolved catalog entry, suitable for a later build/apply consumer.
- `Secrets`: required identifier, container, explicit destination, runtime
  name/scope, and optional creation-source declaration. No credential bytes.
- `Seeds`: noncredential harness seed declarations with resolved host paths;
  composition does not write them. Claude requires an explicit team-state root.

`Result.Entry` is the accepted declaration candidate. `result.MarshalYAML()`
returns only all blueprints followed by all configs for #13's apply operation.
State-volume names include team ownership and own-scope references carry
`ensure: true`, matching the existing daemon auto-provisioning primitive.

Preflight covers strict document shape, selection/reference resolution, role
assets, template/partial execution, image capabilities, parameter completeness,
repository bindings, and structural secret requirements. It does not prove
published image availability, runtime scope/secret existence, source credentials,
host mount or seed accessibility, daemon acceptance, installed CLI behavior, or
live authentication. Those checks require application or launch verification.
Tests use shipped assets and isolated modified filesystems; no live daemon,
network, or runtime credentials are needed.
