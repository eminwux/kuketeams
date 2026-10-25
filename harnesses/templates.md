> Historical template contract copied from [agents at a5dbb59c3942621c5d52ba321b1d81e5987a5f3a](https://github.com/eminwux/agents/tree/a5dbb59c3942621c5d52ba321b1d81e5987a5f3a). Runtime role materialization still uses the external agents checkout. Standalone composition is a later step.

# Harness blueprint templates

Each `harnesses/<h>/` directory carries one harness's team-distribution
declarations:

- `harness.yaml` — `skillPath`, `makeTarget`, and the blueprint `template` name.
- `blueprint.tmpl.yaml` — the Go `text/template` the kukeon renderer
  (teamrender, eminwux/kukeon#1110) executes into a v1beta1 `CellBlueprint`,
  one per `(role × harness)`.
- `partials.tmpl.yaml` — the named-template `{{ define }}` blocks the blueprint
  references via `{{ template "name" . }}`.

## Named-template partials

The renderer parses **every** sibling `*.tmpl.yaml` in the blueprint's own
directory into the blueprint's template set, so a `{{ template "name" . }}`
call resolves against a `{{ define "name" }}…{{ end }}` block in
`partials.tmpl.yaml`. The renderer only scans the blueprint's _own_ directory —
there is no shared/parent partials lookup — so `partials.tmpl.yaml` is
duplicated per harness (`claude/`, `opencode/`, `codex/`) rather than sourced
from one shared file. Keep the three copies in sync.

### Partial calling contract

Each partial receives the **bare selector string** as dot `.` — the current
`{{ range .needs.repos }}` / `{{ range .needs.mounts }}` entry, e.g. `"ssh"`,
`"project"`, `"agents"`. The renderer exposes only the `upper` and `replace`
functions (plus Go template builtins like `eq`/`if`); a partial **cannot reach
`.operator`, `.project`, or any other root-context leaf** — the only datum it
sees is the selector. Operator-derived values a partial therefore cannot
compute (the SSH mount's host home) are composed in the blueprint body via
`$.operator.*`, which _is_ in scope there.

| Partial          | Dot        | Returns                                                                    |
| ---------------- | ---------- | -------------------------------------------------------------------------- |
| `repo_url`       | repo name  | Always empty — see "Repo lookup" below.                                    |
| `repo_target`    | repo name  | In-cell clone dir under `/home/claude/`.                                   |
| `mount_source`   | mount name | Host source path (system mounts only; `ssh` is composed in the blueprint). |
| `mount_target`   | mount name | In-cell mount path.                                                        |
| `mount_readonly` | mount name | `true`/`false`.                                                            |

## Repo lookup

Repos resolve as **structural slots**, not template-time URLs. `repo_url`
always returns empty; an empty `url:` makes teamrender treat the repo as a slot
its `BindConfig` step fills from operator facts (eminwux/kukeon#1115):

| Repo      | Filled from                                                                                                                  |
| --------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `project` | `Inputs.ProjectRepoURL` (the per-project clone URL `kuke team init` reads from the project's `origin`).                      |
| `agents`  | `tc.spec.sources["<owner>/<repo>"]`, or the SSH default derived from the materialized agents source when no override is set. |

`repo_target` maps every non-project repo to `/home/claude/<name>` (e.g.
`agents` → `/home/claude/agents`). The `project` repo's in-cell dir is
per-project and known only at render time, and a partial cannot reach it (it
sees only the selector) — so the blueprint **body** composes the project target
from `.project.NAME` (the team label, e.g. `dezot` → `/home/claude/dezot`),
baking the dir at render time, the same way the SSH mount's host home is
composed from `$.operator.HOME_DIR`. `NAME` — not `.project.PROJECT_DIR` — is
the basename source: `PROJECT_DIR` is the **host-absolute** project path
(`os.Getwd()` at `kuke team init`, e.g. `/home/inwx/projects/dezot`), so
`{{ $home }}/{{ .project.PROJECT_DIR }}` would join into the double-slashed
`/home/claude//home/inwx/projects/dezot` (issue #746). `PROJECT_DIR` is kept
only as the project-bound guard (`{{- if .project.PROJECT_DIR }}`), never
composed into an in-cell path. Baking is correct because both the project dir
and the team label are fixed per team at `kuke team init` time; it also keeps
`${PROJECT_DIR}` out of the resolved blueprint, where it would otherwise
survive as an unfillable literal (the team-rendered `CellConfig.spec.values`
never carries `PROJECT_DIR`).

This is the issue's recommended **operator-supplied-via-TeamsConfig** option:
the agents clone URL lives in the operator's `~/.kuke/kuketeams.yaml`, never
baked into this shared repo.

> **Scope note.** teamrender's `BindConfig` only fills the `project` and
> `agents` slots. Repos some non-default roles declare — `kukeon` —
> would render as unfilled
> slots until teamrender learns to fill them (or the blueprint carries an
> inline URL for them). Those roles are outside the `[dev, pm, pr-reviewer] ×
[claude, opencode]` surface this step targets; wiring them is follow-up work.

## Shared-reference delivery and loading

All seven roles declare the existing `agents` repository slot. Its checkout
supplies [shared references](https://github.com/eminwux/agents/blob/a5dbb59c3942621c5d52ba321b1d81e5987a5f3a/common/references/README.md) directly at
`<home>/agents/common/references/`; the templates select `<home>` from the root
capability. Verify the effective `HOME` agrees with the resolved paths. The
[agents Makefile](https://github.com/eminwux/agents/blob/a5dbb59c3942621c5d52ba321b1d81e5987a5f3a/Makefile) copies common skills, role skills, and role `CLAUDE.md`
into the selected skill tree; it does not copy references. From a generated
`skills/CLAUDE.md`, follow the source-checkout path in the role pointer. Local
invocations use the checkout that supplied `make`.

Filesystem delivery, instruction discovery, and actual reading are separate:

| Harness  | Source-inspected loading mechanism                                                                                                        | Deployment evidence still needed                                                                                                                                                      |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Claude   | Role settings and `--add-dir`, with `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1`.                                                     | Effective role/project loading and native-tool read of the sibling `common/references/` path; the role's add-dir does not itself name that sibling.                                   |
| Codex    | Shared Makefile materialization, managed `$CODEX_HOME/AGENTS.md`, and project fallback configuration. See [Codex cells](codex/README.md). | Live instruction/skill discovery and volume ownership still require cell smoke tests.                                                                                                 |
| OpenCode | Startup generates a config with a role `instructions` entry.                                                                              | Final rendered/bound parameter substitution, generated config, and actual role/reference reads. Shell heredoc quoting alone does not establish whether earlier substitution occurred. |

These are source contracts, not runtime certification. Verify the selected
image digest/CLI version, effective user/home/cwd, final settings and paths,
and native reads using isolated fixtures before claiming a launch path works.
Test both assigned-task and empty interactive modes. Diagnose any baseline
loading failure separately before proposing a harness correction; availability
of files or a successful shell read alone is insufficient.

References summarize policy and carry no writable shared memory or live tracker
state. Existing state scoping and permissions remain authoritative. Refresh
references and role pointers together through the existing source mechanism,
then use a new session or controlled restart/rematerialization before claiming
the updated pointer is loaded. Keep source and generated skills on a compatible
revision; do not assume hot reload during an active session. This documentation
change requires no image rebuild or profile regeneration. Roll back references
and pointers together through the previous source revision and rematerialize,
preserving owned state. Template changes, if separately needed, require their
own installed-configuration refresh and validation.

## Mount lookup

Mounts are resolved entirely at template time (teamrender has no volume-slot
fill step). The SSH mount is universal across roles and is the one
operator-relative mount, so its host source is composed in the blueprint from
`$.operator.HOME_DIR`; the remaining mounts (declared only by `orchestrator`)
carry fixed host paths.

| Mount            | Host source                                   | In-cell target             | Read-only       |
| ---------------- | --------------------------------------------- | -------------------------- | --------------- |
| `ssh`            | `<HOME_DIR>/.ssh` (via `$.operator.HOME_DIR`) | `/home/claude/.ssh`        | yes             |
| `kuke-binary`    | `/usr/local/bin/kuke`                         | `/usr/local/bin/kuke`      | yes             |
| `kukeond-socket` | `/run/kukeon/kukeond.sock`                    | `/run/kukeon/kukeond.sock` | **no** (dialed) |
| `kukeon-install` | `/opt/kukeon`                                 | `/opt/kukeon`              | yes             |

The `orchestrator` mount paths reproduce the retired hand-written
`crew/agents/orchestrator/blueprints/orchestrator.yaml` (eminwux/crew, retired
in step 9 / #720). They are fixed host conventions, not operator facts; if the
operator install layout diverges, a dedicated operator-fact contract for these
mounts is follow-up work. `orchestrator` is outside this step's render surface.

## State volume

Each `claude`/`codex` cell's durable agent state — `~/.claude` (memory,
learnings, caches) and `~/.codex` — is mounted from a **daemon-managed
`kind: Volume`** (eminwux/kukeon#1016), not a raw host bind. The blueprint
emits a `kind: volume` mount whose `source` is a Volume name resolved in the
cell's own scope (realm/space/stack, most-specific first) at container start;
the Volume's on-disk directory survives both container recreation and the
mounting cell's deletion. This replaces the prior unmanaged
`TEAM_ROOT/${ROLE}-{claude,codex}` host binds, which had no lifecycle and no
delete-gate.

| Harness  | Mount kind    | Volume name (project-bound role)    | Volume name (singleton role) | In-cell target |
| -------- | ------------- | ----------------------------------- | ---------------------------- | -------------- |
| `claude` | `volume` (rw) | `<project.NAME>-<role.name>-claude` | `<role.name>-claude`         | `~/.claude`    |
| `codex`  | `volume` (rw) | `<project.NAME>-<role.name>-codex`  | `<role.name>-codex`          | `~/.codex`     |

The name is keyed by `(project, role)` and fixed at **render time** (from the
Role + ProjectTeam), so it is a static template — no per-cell dynamic identity
(kukeon#1017) is needed. Multiple cells of the same `(role, project)` share one
Volume, exactly as multiple instances shared the host dir before. Project-bound
roles (`pm`, `dev`, `pr-reviewer`) carry the `(project, role)` name; singleton
roles (`meta-*`, `orchestrator`, which have no `.project`) fall back to the
per-role-only name under the same `{{ if .project.PROJECT_DIR }}` guard the rest
of the blueprint uses. Distinct `(role, project)` Volumes keep `pm`/`dev`/
`pr-reviewer` memory isolated rather than merged into one shared `~/.claude`
(the shared-profile + auth alternative is tracked in kukeon#1164).

The read-only `claude.json` **seed** mount stays a host bind — it is
operator-provided config, not durable agent state.

> **Provisioning.** kukeon#1016 shipped **without** auto-create — an unresolved
> volume reference is a hard error at cell start (kukeon#1290), so the
> `(project, role)` Volumes must be **pre-created** before the cell starts (e.g.
> `kuke create volume <name>` in each cell's scope, or as part of `kuke team
init`). Wiring this pre-provision into team init is tracked as a follow-up.

### SSH configuration in root cells

Root blueprints set `GIT_SSH_COMMAND` to explicitly load `/root/.ssh/config`
with `ssh -F` when that file exists, falling back to `-F /dev/null` otherwise.
Git evaluates the selection inside the cell, including for required repository
clones before the interactive terminal starts. This lets root use the read-only
host-user-owned config without OpenSSH rejecting its implicit user-config
ownership check. No host SSH ownership or permissions are changed; identity-file
and host-key options remain the same. Non-root blueprints are unchanged.

Explicit `-F` suppresses `/etc/ssh/ssh_config` in root cells. With no mounted
config, SSH uses built-in defaults plus the command-line options, not system
configuration. If needed, include system configuration explicitly from the
mounted config (for example, `Include /etc/ssh/ssh_config`).
