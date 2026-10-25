# Public harness assets

Kuketeams owns these Dockerfiles, catalogs, harness definitions, blueprint
templates, sibling partials, and Codex policy/hooks. They were copied from
[eminwux/agents at a5dbb59c3942621c5d52ba321b1d81e5987a5f3a](https://github.com/eminwux/agents/tree/a5dbb59c3942621c5d52ba321b1d81e5987a5f3a).
The source checkout was re-inventoried at that revision before copying; every
tracked `harnesses/` file is included. [UPSTREAM.tsv](UPSTREAM.tsv) records the
original paths, modes, and Git blob IDs, including the source publication
workflow. Original license notices are retained under the repository's
[Apache 2.0 license](../LICENSE). Agents originals are unchanged.

The packaging adaptations are the kuketeams image namespace, versioned base
references, a shared Buildx Bake graph/entrypoint, release checks, and the tag
publication workflow. Hook tests compare against recorded outputs from the six
upstream Claude role hooks; those fixtures avoid copying specialized roles.
The review-required home adaptation uses account `user` in non-root images
and the root identity in root images, both with `/home/user` as HOME, passwd
home, and WORKDIR. CLI paths, Android ownership, templates, and SSH/state
mounts follow that shared path; root privilege and nested-runtime settings
remain capability-gated. Packages, CLI versions, and managed policy are retained. `/opt/agents/codex/hooks` is an image-internal policy path
and stays unchanged.

| Images                      | Parent                   | Purpose                                |
| --------------------------- | ------------------------ | -------------------------------------- |
| `base`                      | `debian:trixie-slim`     | Shared runtime and development tools   |
| `base-user`, `base-root`    | `base`                   | User/group or root/Docker tooling      |
| `claude`, `claude-root`     | `base-user`, `base-root` | Claude native CLI                      |
| `claude-android`            | `base-user`              | Claude, JDK 21, Android SDK 35         |
| `codex`, `codex-root`       | `base-user`, `base-root` | Codex 0.163.0, managed policy and hook |
| `opencode`, `opencode-root` | `base-user`, `base-root` | OpenCode CLI                           |

All ten images target Linux amd64 and arm64. Android SDK packages retain the
upstream Linux distribution; a successful arm64 image build does not certify
that every SDK executable supports arm64. Agent CLI `latest` defaults and
upstream apt/install endpoints are preserved, so source pinning alone does not
make image bytes reproducible.

[images.yaml](images.yaml) catalogs the seven selectable leaf images; bases
are build intermediates. [templates.md](templates.md) preserves the upstream
template/partial contract. Each harness's `harness.yaml` resolves its template
within the same directory. Role repositories, role `makeTarget` installation,
operator mounts, credentials, and daemon volumes remain runtime inputs supplied
by the external agents checkout and operator. They are not Docker COPY inputs.
Production CLI composition, `--build`, cell startup, and role migration belong
to later epic steps.

## Local builds and checks

Install Docker with Buildx/Bake and Python 3; Go 1.24+ runs the source checks.
Use a BuildKit builder with multi-platform support for both architectures (for
example `docker buildx create --use --driver docker-container`). Cross-platform
RUN instructions require binfmt/QEMU when not building on native workers.

```sh
make test                       # Go vet and full race-enabled suite
make test-harnesses              # references, rendered templates, hooks, build plan
make -C harnesses plan           # resolved build graph without building
make -C harnesses images         # native images loaded into local Docker
make -C harnesses images IMAGES="codex codex-root"
make -C harnesses smoke          # after building all native images
make -C harnesses images MODE=check PLATFORMS=linux/amd64,linux/arm64
```

`build-images.sh` is the shared local/CI entrypoint and can also be run from
any working directory. `REGISTRY` defaults to `ghcr.io/eminwux/kuketeams`, `TAG`
to `latest`. `MODE=load` requires one platform; `MODE=check` uses cache-only
outputs and never publishes. `MODE=push` explicitly publishes all selected
images, including their bases when the complete default group is selected.
Local Docker loads and checks need no registry credentials. A subset build
automatically builds its transitive bases through Bake `target:` contexts.
These contexts share freshly built bases inside BuildKit, without registry
round trips or publishing intermediates for PR validation. See the
[Docker Bake context documentation](https://docs.docker.com/build/bake/contexts/).

`.tmpl.yaml` files contain Go templates, so generic YAML/Prettier hooks exclude
only those files. Go tests parse every sibling partial and render project and
singleton fixtures for user/root envelopes before checking the resulting YAML.

The native smoke target checks the user/root bases and all seven harness
variants: effective UID/account, HOME and passwd home, working directory,
writable agent state, a read-only SSH fixture (including root `-F` handling),
and CLI version/help startup without credentials or network access. The common
`base` image remains a neutral Debian development layer.

## Release tags and publication

The final publication workflow triggers only on pushed `v[0-9]*` tags. The
release script additionally requires `vMAJOR.MINOR.PATCH` with an optional
prerelease suffix. Before an authorized release, pin the leaf catalog, commit
it, and tag that commit:

```sh
./harnesses/release-images.sh v0.1.0
./harnesses/release-images.sh v0.1.0 --check
# Commit the catalog, then create/push the release tag through the release process.
```

The catalog's initial `v0.1.0` references reserve the first release; they are not
an assertion that images have already been published. No release tag is created
by this packaging change. Tag runs check catalog/tag consistency before login,
then use the same graph with both platforms and registry outputs. Every base
and leaf carries the release tag, and every leaf consumes that release's base
result, preventing an unrelated `latest` base from entering a release. Only the
tag publication job receives `packages: write` and the GHCR login token.

After a tag run succeeds, verify all ten references, including bases:

```sh
for image in base base-user base-root claude claude-root claude-android codex codex-root opencode opencode-root; do
  docker buildx imagetools inspect "ghcr.io/eminwux/kuketeams/$image:v0.1.0"
done
```

Check that each manifest includes `linux/amd64` and `linux/arm64` (attestation
manifests may appear too). New GHCR packages can be private: the owner must set
package visibility to public in GitHub package settings for anonymous use.
Then repeat manifest inspection and `docker pull --platform linux/amd64` and
`docker pull --platform linux/arm64` for each reference with an empty temporary
`DOCKER_CONFIG` on a machine with Docker/Buildx installed. This confirms public
pull access independently of a logged-in publisher. Compare leaf references
with the tagged catalog using `release-images.sh <tag> --check` at that tag.
