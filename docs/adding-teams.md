# Adding a team from a Git repository

After `kuketeams init`, register a named team without a local project checkout or
Team file:

```sh
kuketeams add kukeon --source git@github.com:eminwux/kukeon.git
kuketeams add kukeon --source ssh://git@work-alias:2222/eminwux/kukeon.git --branch develop
kuketeams add kukeon --source https://github.com/eminwux/kukeon.git --tag v1.0.0
kuketeams add kukeon --source github.com/eminwux/kukeon --commit <full-commit-object-id>
```

The positional argument names the team; `root` is reserved for init. Exactly one
of `--branch`, `--tag`, or `--commit` may be supplied. Commits require a full
40- or 64-character hexadecimal object ID. Without a selector, native
`git ls-remote --symref` discovers the remote HEAD branch and saves that branch
name. A detached/missing remote HEAD requires an explicit selector. Branches
remain floating selectors; tags select `refs/tags/<name>` and commits select the
object ID. All selections are fetched and checked out before application,
including the project source when it supplies no role/harness definitions.

## Prerequisites and defaults

Git, an existing valid `~/.kuke/kuketeams.yaml`, kukeond, and the parent
`default/default/default` realm/space/stack metadata are required. Add uses the
same [Unix socket and client configuration precedence](initialization.md) as init:
`--host`, `KUKEON_HOST`, `ClientConfiguration.spec.host`, then the default socket.
`--configuration` selects the client file, followed by `KUKE_CONFIGURATION` and
`~/.kuke/kuke.yaml`. Use kuke to provision the daemon and parent scopes first.

Add reads global role/harness selections, Git identity, parameters, and scoped
secret bindings. It selects published images from the selected definition
source's image catalog. It does not build images or create/start cells. Existing
runtime secrets are reused, including when their host-side creation source is
unavailable; missing secrets use the existing explicit env/file creation source
or report a kuke provisioning command. Fix all preflight errors before rerunning.

The additional project source is named `project`, with clone destination
`/home/user/<team-name>`. It is combined with resolved definition sources, so
inherited roles/harnesses retain their repositories. Any existing source named
`project` is a preflight error: rename that inherited/authored source and its
references before adding the team. Duplicate clone destinations also fail rather
than replacing another checkout.

The accepted TeamEntry stores an additive top-level `projectSource` and the
original omitted/explicit `spec` selections:

```yaml
apiVersion: kuketeams.io/v1
kind: TeamEntry
metadata:
  name: kukeon
projectSource:
  name: project
  url: git@github.com:eminwux/kukeon.git
  branch: main # Example only: actual remote HEAD is discovered.
  clonePath: /home/user/kukeon
spec: {}
```

`projectSource` is also supported on a Team declaration. It is added after
resolving `spec.sources`; explicitly authored source/role/harness lists retain
their existing whole-list replacement semantics. Omitted fields are not frozen
into the entry. Future recomposition therefore inherits current global defaults.
Repeated add replaces the project source/revision, retaining any explicit
selections in an existing valid named registration. It never resets global
configuration or reads a local `kuketeam.yaml`.

## Git transports and host credentials

Supported remote forms are SCP-style `[user@]host:path`, explicit
`ssh://[user@]host[:port]/path`, HTTPS, and `github.com/owner/repo` HTTPS shorthand.
SSH URLs are retained as SSH. Host aliases and custom ports use native Git/SSH
settings. `git@github.com/owner/repo` is malformed: use
`git@github.com:owner/repo` or `ssh://git@github.com/owner/repo` instead. Local
paths, `file://`, HTTP, and other schemes are rejected by add.

Do not put passwords, tokens, query parameters, or fragments in source URLs.
HTTPS userinfo is rejected even without a password. SSH usernames such as `git@`
are allowed; SSH passwords in URLs are rejected. URL validation happens before
Git invocation or persistence. Git output and credential-helper output are never
included in diagnostics or generated documents.

Both discovery and fetch use native Git with the same caller environment and
configuration. HTTPS uses configured Git credential helpers; public repositories
need no credentials. For private HTTPS, configure a helper for the host and verify
that it can return credentials noninteractively. An already configured
`gh auth setup-git` helper can serve GitHub, but neither gh nor a `gh-token`
runtime secret is required for host-side fetching.

For SSH, configure the host/alias, identities, port, and trusted host keys through
your normal OpenSSH configuration. Unlock/load the intended identity into your
SSH agent, for example `ssh-add ~/.ssh/id_ed25519`, before running add. Check the
server's host-key fingerprint through a trusted channel before installing it in
`known_hosts`; investigate changed keys rather than bypassing verification.
The host command preserves SSH agent/configuration and trust settings and adds
`BatchMode=yes` to an OpenSSH-compatible command (`GIT_SSH_COMMAND`,
`core.sshCommand`, `GIT_SSH`, or ssh). Git/GCM and askpass prompting are disabled, and Git runs without a controlling
terminal. Each Git
subprocess has a one-minute deadline, also bounded by caller cancellation.
Credential helpers must themselves support noninteractive use.

Authentication, host-trust, unavailable-revision, and network failures report
safe guidance without guessing a precise cause from unsafe Git stderr. Check
transport configuration, trusted keys/unlocked identities or HTTPS helpers,
network access, and the requested revision, then repeat the command.

## Host access and runtime access

Successful host-side fetch validates the source for composition; it does not
provision clone credentials for future cells. Generated CellConfig repository
bindings carry the URL and selected revision, and blueprints request the project
checkout. A role explicitly declaring `needs.repos: [project]` also selects its
project launch directory; otherwise the role/harness's existing launch behavior
applies. Adding a repository does not extend the role's onboarding behavior.

A future cell still needs access to every requested repository, the published
image, and its harness/provider authentication. Use the existing supported role
mounts and scoped secret bindings where applicable; they are explicit role/global
configuration, not automatic host-credential forwarding. Add does not request an
SSH mount, forward the host agent, copy private keys, or acquire/provision clone
credentials. A private SSH/HTTPS project can pass host preflight while a future
cell cannot clone it. Consult the selected role/harness and
[composition's binding contract](composition.md#runtime-repository-and-parameter-bindings)
before explicitly launching with kuke.

## Registration and recovery

Before daemon mutation, add validates source references, selected definitions,
images, required secrets/scopes, and stages a complete `0600` entry beside
`~/.kuke/kuketeam.d/<name>.yaml`. It calls `ApplyDocumentsForTeam` with the named
team's stable ownership identity and publishes registration by atomic rename only
after full application success. Repeated add converges those same named resources
and one registration. The existing daemon contract may prune obsolete team-owned
blueprints/configs; it does not roll back successful mutations.

Failure returns nonzero and preserves any previous accepted entry. A partial
apply or ambiguous RPC failure can leave some daemon resources changed. A
registration failure after successful application is reported explicitly. Fix
the prerequisite and rerun the same add command (including your explicit revision
flags when used). Do not interpret an older registration as proof that the failed
update was applied. Add does not roll back created secrets/resources, launch
cells, or retry ambiguous writes.

## Verification

`make test` runs native Git through controlled SSH fixtures and a local TLS smart
HTTP Git server. It covers remote HEAD discovery, branches/tags/commits, host
aliases/ports, configured SSH commands, credential-helper authentication, public
HTTPS without credentials, noninteractive/redacted failures, named Unix RPC
application, inherited defaults, secret reuse, staging, partial apply, publication
failure, and repeated registration.

Live private-repository verification is optional and requires a disposable team,
existing parent scope, accessible published images, independently configured
host credentials/trust, and separately configured runtime credentials if launching
later. Run the example with your own repository and selected Unix endpoint,
inspect the named resources and TeamEntry, then repeat. Do not save credential
values, helper output, raw Git stderr, or private keys in reports. The automated
fixtures establish the command contract, not access to your private host or a
successful future cell launch.
