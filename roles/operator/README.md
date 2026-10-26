# Operator role

The operator inspects kukeon runtime/configuration and carries out explicitly
requested operations with verified outcomes. Its initial
[`create-cell-config`](skills/create-cell-config/SKILL.md) skill inspects an
existing blueprint, produces a CellConfig draft, and applies it when requested.
Config creation never implicitly creates or starts a cell.

[`AGENTS.md`](AGENTS.md) is the canonical role policy. [`CLAUDE.md`](CLAUDE.md) is
a relative symlink adapter for harnesses expecting that filename. The tree is
self-contained: skill paths in [`role.yaml`](role.yaml) are **source-checkout
relative**, matching agents conventions; Claude's `settings` is **role-relative**.
No common or specialized agents roles, hooks, status lines, GitHub CLI, or GitHub
credentials are required. The `go` image capability matches the initial
TeamsConfig selection; it does not dynamically install runtime prerequisites.

## Prerequisites and authority

- A compatible `kuke` client must be provisioned on PATH, configured for the
  intended running kukeond endpoint, with access to that endpoint. The role and
  public harness images do not provision that connection. Do not assume that
  a socket is mounted or that a CLI install alone grants daemon access.
- The blueprint and destination realm/space/stack must exist. Repository sources
  and secret identities must satisfy the selected blueprint. Inspect the actual
  schema and the installed CLI help; the skill records its inspected kukeon
  revision and source links in a bundled reference.
- Only the selected harness needs its provider credential: Claude uses
  `claude-code-oauth-token`, Codex `openai-api-key`, OpenCode
  `openrouter-api-key`. These names become `CLAUDE_CODE_OAUTH_TOKEN`,
  `OPENAI_API_KEY`, and `OPENROUTER_API_KEY` in the packaged templates. Supply
  credentials through the configured secret mechanism, never role files.
- `root` is the initial team's name. This role requests no `root` capability,
  privileged runtime, sudo, host filesystem/SSH mount, or GitHub token. An
  authorized daemon connection can still mutate runtime resources: target and
  operation authorization come from the user's request.

## Materialize into a dedicated cell home

The installer needs POSIX sh, make, and standard filesystem utilities. It can
run from a source checkout without the agents repository, any daemon, provider
credentials, or network access:

```sh
make -C /path/to/kuketeams claude ROLE=roles/operator
make -C /path/to/kuketeams codex ROLE=roles/operator
make -C /path/to/kuketeams opencode-openrouter ROLE=roles/operator
```

Choose the target for the cell's harness. Equivalently run `make -C
/path/to/kuketeams/roles/operator TARGET`; the role-local Makefile works when the
role tree alone is copied. Installation creates symlinks back to this tree, so
keep it at the same absolute path while the cell runs. Reruns are idempotent;
conflicting instructions, settings, or a same-name skill are reported and
preserved. Other skills, settings, and credentials are untouched. A nonempty
Codex `AGENTS.override.md` is rejected because it shadows `AGENTS.md`.

| Harness  | Always loaded instructions                             | Skill location                               | Configuration                                                                                          |
| -------- | ------------------------------------------------------ | -------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| Claude   | `${CLAUDE_CONFIG_DIR:-$HOME/.claude}/CLAUDE.md`        | same directory's `skills/create-cell-config` | Pass role-local `config/claude.settings.json` with `--settings`                                        |
| Codex    | `${CODEX_HOME:-$HOME/.codex}/AGENTS.md`                | `$HOME/.agents/skills/create-cell-config`    | `role.yaml` supplies sandbox/approval to the launcher; existing Codex config is preserved              |
| OpenCode | `${XDG_CONFIG_HOME:-$HOME/.config}/opencode/AGENTS.md` | `$HOME/.agents/skills/create-cell-config`    | Installs `config/opencode.json` at `$OPENCODE_CONFIG` if set, otherwise in the native config directory |

These are native user instruction locations, independent of the launch working
directory. Copying `AGENTS.md` into a skills directory would not do the same
job. Claude's filename adapter also supports the copied templates' `--add-dir`
contract when additional-directory memory loading is enabled; the installer
uses user instructions so that loading does not depend on that switch.
See the official [Claude memory](https://code.claude.com/docs/en/memory),
[Codex instructions](https://developers.openai.com/codex/guides/agents-md), and
[OpenCode rules](https://opencode.ai/docs/rules/) and
[skills](https://opencode.ai/docs/skills/) contracts.

After installation, a manually provisioned dedicated cell can launch with:

```sh
claude --settings /path/to/kuketeams/roles/operator/config/claude.settings.json
codex --sandbox workspace-write --ask-for-approval on-request
# Set OPENROUTER_MODEL to the selected provider/model; authentication is supplied separately.
opencode
```

The adapters do not bypass permission prompts or add privileges. Access outside
Codex's workspace sandbox may need an approved escalation or an explicitly
configured runtime policy. Materialization does not launch an agent or a cell.

## Delivery boundary and provenance

Structure was adapted from `eminwux/agents` at
`a5dbb59c3942621c5d52ba321b1d81e5987a5f3a` (`dev/role.yaml`, `pm/role.yaml`, role
skill directories, and root Makefile). Operator instructions/configs are new.
Harness paths/targets were checked against PR #11 at
`848417202285ee7847ce8e2e7500414ff3754ec5`: `claude` installs `.claude/skills`,
`codex` and `opencode-openrouter` install `.agents/skills`.

This issue proceeded before #8 merged under an explicit user override. The
copied templates in #11 still invoke `make -C ~/agents` with agents-relative
roles. They cannot directly launch this new role yet. The adapters here are the
materialization interface for the composition work in #12; rendering source
locations/launch settings, supplying runtime/auth bindings, and live remote-image
launch verification remain #12–#14. No successful agent inference, daemon apply,
or cell startup is claimed by the structural tests.

`make test` checks the actual bootstrap selection, declared YAML/paths,
frontmatter/reference closure, settings, and isolated installation for all three
harnesses (including overrides, reruns, and conflicts). Contract tests use the
packaged harness manifests when available, otherwise the complete pinned #11
manifests in `internal/operatorassets/testdata/`.
