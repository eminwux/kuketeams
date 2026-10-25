# kuketeams

A planned standalone CLI and library for composing agent teams into
[kukeon](https://github.com/eminwux/kukeon) blueprints and configs.

This repository will own the basic operator role, public harnesses, rendering
templates, and harness image definitions. Specialized roles and skills can come
from external repositories such as [agents](https://github.com/eminwux/agents).

## Status

The standalone tool is not implemented yet. The design and six planned delivery
steps are tracked in [the implementation epic](https://github.com/eminwux/kuketeams/issues/1).

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

## Repository development team

The checked-in [kuketeam.yaml](kuketeam.yaml) uses the existing `ProjectTeam`
format consumed by `kuke team init`. It follows kukeon's development roster,
with this repository's team name, and selects the dev, pm, and pr-reviewer roles
from the agents repository with Claude, OpenCode, and Codex harnesses.

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
