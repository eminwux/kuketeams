# Applying the root operator team

`kuketeams init` preserves or bootstraps `~/.kuke/kuketeams.yaml`, fetches only
sources used by selected role/harness definitions into temporary Git checkouts,
and composes the root team's blueprints/configs. Git must be installed and able
to read those sources without a terminal prompt. `branch`, `tag`, and `commit`
selectors are fetched explicitly; no checkout in the current directory is used.
Fetched definitions are removed after the invocation. Runtime repository URLs and
clone destinations still come from the authored configuration.

The command reaches kukeond with the public `kukeonv1` Unix JSON-RPC client.
Endpoint precedence matches kuke:

1. `kuketeams init --host unix:///absolute/path/to/kukeond.sock`
2. `KUKEON_HOST`
3. `ClientConfiguration.spec.host`
4. `unix:///run/kukeon/kukeond.sock`

The configuration path is `init --configuration`, then `KUKE_CONFIGURATION`,
then `~/.kuke/kuke.yaml`. An absent default file is allowed; an explicitly selected
missing file, unreadable file, invalid YAML, or wrong kind/version is an error.
Only the supported Unix transport is accepted. Init does not start the daemon.

Blueprints/configs target the `default` realm, space, and stack. Preflight checks
that the parent metadata exists, checks secret prerequisites, and stages a
complete `0600` TeamEntry before mutating the daemon. Missing parent scopes must
be provisioned with kuke. Init never creates, starts, restarts, or deletes cells.
`ApplyDocumentsForTeam` receives the stable team name `root`, so reruns update
that team's resources and prune obsolete team-owned blueprints/configs using the
existing daemon contract. It does not prune credentials or running cells.

## Authentication and secrets

Only selected role/harness requirements contribute secret lookups. The base
operator with Codex or Claude does not require `gh` or `gh-token`. Selecting
OpenCode requires its configured provider binding, as described in
[composition](composition.md#secret-declarations).

Each runtime secret is identified by all explicit reference coordinates, without
replacing realm-only references with stack defaults. A successful metadata lookup
with `MetadataExists: true` reuses the existing credential even when the local
creation source is absent, unreadable, or contains a different value. A failed
lookup is an error; it never authorizes secret creation.

For a missing secret, init checks the required parent scope and resolves only an
explicit `source.env` variable or `source.file` path. Missing/empty values fail
preflight. File sources follow kuke's trailing-newline removal. All required
sources are checked before the first create, and shared runtime identities are
created once. Creation happens before dependent blueprints/configs are applied.
Credentials and remote secret-error text are never printed or saved in TeamEntry
or the blueprint/config request. Diagnostics identify the operation and whether
the failure was a transport failure or daemon rejection.

A missing secret without a creation source reports a command such as:

```sh
kuke create secret provider-account --realm auth --space shared --stack operators --from-file /path/to/token
kuketeams init
```

The actual command uses the binding's resolved name/scope. Replace the file path
with your own credential file and use the same kuke `--host`/`--configuration`
endpoint. Parent scopes must exist first. The pinned kuke CLI has no secret
`--cell` flag; a cell-scoped reference instead requires a `Secret` manifest with
its exact metadata coordinates, applied with `kuke apply -f /path/to/secret.yaml`.

Init rechecks existence immediately before each create and never retries a
secret write, including when a transport failure leaves its outcome ambiguous.
On a rerun, a secret accepted by the previous attempt is discovered and reused.
This uses the existing kukeon API contract: `CreateSecret` is a write-through
operation. A different daemon client can still create a secret between the final
lookup and write; atomic create-if-absent semantics are not supplied by that API.
Secret rotation is not an intended init action; rotate explicitly with kuke.

## Registration and recovery

The accepted authored declaration is saved atomically at
`~/.kuke/kuketeam.d/root.yaml`. It retains explicit selections and omitted/empty
list semantics, with no resolved credentials or duplicated effective defaults.
Later init runs read this declaration and resolve its inherited defaults from the
current global configuration. An invalid existing registration fails preflight
rather than resetting authored configuration.

Registration follows full blueprint/config success only. A secret failure, RPC
failure, total resource failure, partial apply, prune failure, or incomplete
response returns nonzero and leaves any prior TeamEntry unchanged. First-time
failure creates no registration. Successful resources and created secrets are
not rolled back: rerun init after fixing the prerequisite to converge the same
root identity. A registration write failure after application is explicitly
reported as such and can also be recovered by rerunning init. A previous entry
records the last accepted declaration, not a claim that the latest failed update
succeeded.

## Validation

`make test` includes injectable-client success/failure/rerun tests and real Unix
JSON-RPC transport tests. A separate integration check verifies actual daemon
storage, reads back blueprints/configs, repeats application, and confirms that
cells are unchanged:

```sh
KUKETEAMS_INTEGRATION_HOST=unix:///path/to/disposable/kukeond.sock make test-integration
```

Use a disposable kukeond with existing `default/default/default` scope metadata.
The check uses a unique `init-check-*` team and deletes only its generated
blueprints/configs on cleanup. It does not test image pulls, live harness
credentials, or agent launch; those belong to the subsequent launch step.
