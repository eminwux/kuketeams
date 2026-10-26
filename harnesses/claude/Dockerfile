ARG REGISTRY=ghcr.io/eminwux/kuketeams
ARG TAG=latest
FROM ${REGISTRY}/base-user:${TAG}

ARG CLAUDE_CODE_VERSION=latest

# Native install — Claude Code self-migrates the npm-global install to a
# standalone binary at ~/.local/bin/claude and records installMethod=native;
# baking the native binary into the image keeps that consistent across cells
# (~/.local is image state, not a bind-mount, so it persists), and avoids a
# ~230MB per-cell re-download on first run.
ENV PATH=/home/user/.local/bin:$PATH

RUN curl -fsSL https://claude.ai/install.sh | bash -s -- "${CLAUDE_CODE_VERSION}" \
 && echo 'export PATH=$HOME/.local/bin:$PATH' >> /home/user/.bashrc \
 && rm -f $HOME/.claude.json
# Drop the default ~/.claude.json the native install bakes in: every profile's
# onInit seeds login state via `[ -f ~/.claude.json ] || cp .claude.json.seed`,
# and a baked-in file would short-circuit that guard and skip the seed copy.
