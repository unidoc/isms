# =============================================================================
# ISMS Dev Runtime
# Go (ISMS) + Claude Code | debian:trixie-slim
# Editor lives on the host — this is pure runtime.
# =============================================================================
FROM debian:trixie-slim

# ── Build args ───────────────────────────────────────────────────────────────
ARG GO_VERSION=1.25.3
ARG USER_NAME=isms
ARG USER_UID=1000
ARG USER_GID=1000

# ── Environment ──────────────────────────────────────────────────────────────
ENV DEBIAN_FRONTEND=noninteractive
ENV LOCAL_INSTALL=/usr/local
ENV GOROOT=${LOCAL_INSTALL}/go
ENV GOPATH=/home/${USER_NAME}/.gopath
ENV PATH=${GOROOT}/bin:${GOPATH}/bin:/home/${USER_NAME}/.local/bin:${PATH}
ENV COLORTERM=truecolor
ENV TERM=xterm-256color
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

ENV CODEX_UNSAFE_ALLOW_NO_SANDBOX=1

ENV WORKSPACE_HOME=/home/${USER_NAME}/workspace

# ── System packages ─────────────────────────────────────────────────────────
RUN apt-get update && apt-get install -y --no-install-recommends \
    file \
    python3 \
    python3-pip \
    python3-venv \
    python3-dev \
    git \
    curl \
    ripgrep \
    fd-find \
    fzf \
    just \
    less \
    build-essential \
    libpq-dev \
    make \
    wget \
    procps \
    nano \
    ca-certificates \
    bash \
    gosu \
    openssh-client \
    jq \
    && rm -rf /var/lib/apt/lists/*

# ── Node.js (LTS) ───────────────────────────────────────────────────────────
RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN npm i -g @openai/codex
RUN npm i -g @google/gemini-cli
RUN npm i -g @playwright/test
# Fixed, world-readable browser path. This RUN executes as root, so the
# default ($HOME/.cache/ms-playwright) would land in /root — invisible to
# the isms user and to the Python playwright in the test venv. The ENV makes
# every playwright (npm and pip, any user) resolve the same location.
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright
RUN npx playwright install --with-deps chromium \
    && chmod -R a+rX ${PLAYWRIGHT_BROWSERS_PATH}

# ── Go ───────────────────────────────────────────────────────────────────────
RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz \
    | tar -C ${LOCAL_INSTALL} -xz

# Debian's /etc/profile hard-resets PATH in login shells (bash -l), dropping
# the ENV PATH above. profile.d is sourced after that reset, so this wins —
# keeps `go` resolvable in compose commands, Justfile recipes, and attach.
RUN printf 'export PATH=%s/bin:%s/bin:$HOME/.local/bin:$PATH\n' "${GOROOT}" "${GOPATH}" \
    > /etc/profile.d/golang.sh

# goreleaser — local snapshot builds (`goreleaser release --snapshot`) for
# testing the release pipeline without tagging.
RUN GOBIN=${LOCAL_INSTALL}/bin GOPATH=/tmp/gobuild ${GOROOT}/bin/go install github.com/goreleaser/goreleaser/v2@latest \
    && rm -rf /tmp/gobuild

# ── Create non-root user (isms:isms 1000:1000) ───────────────────────────────
RUN groupadd -g ${USER_GID} ${USER_NAME} \
    && useradd -m -u ${USER_UID} -g ${USER_GID} -s /bin/bash ${USER_NAME}

# ── Project directory ────────────────────────────────────────────────────────
RUN mkdir -p ${WORKSPACE_HOME} \
    && chown -R ${USER_UID}:${USER_GID} /home/${USER_NAME}

# ── Entrypoint ───────────────────────────────────────────────────────────────
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# ── Claude Code ──────────────────────────────────────────────────────────────
USER ${USER_NAME}
RUN curl -fsSL https://claude.ai/install.sh | bash
USER root

WORKDIR ${WORKSPACE_HOME}
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/bin/bash"]
