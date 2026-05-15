# =============================================================================
# ISMS Dev Runtime
# Go (ISMS) + Claude Code | debian:trixie-slim
# Editor lives on the host — this is pure runtime.
# =============================================================================
FROM debian:trixie-slim

# ── Build args ───────────────────────────────────────────────────────────────
ARG CRONTAB_URL_DEPS
ARG GO_DL=go1.25.3.linux-amd64.tar.gz
ARG USER_NAME=tower
ARG USER_UID=1000
ARG USER_GID=1000

# ── Environment ──────────────────────────────────────────────────────────────
ENV DEBIAN_FRONTEND=noninteractive
ENV LOCAL_INSTALL=/usr/local
ENV GOROOT=${LOCAL_INSTALL}/go
ENV GOPATH=/home/${USER_NAME}/.gopath
ENV PATH=${GOROOT}/bin:${GOPATH}/bin:${PATH}
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
RUN npx playwright install --with-deps chromium

# ── Go ───────────────────────────────────────────────────────────────────────
RUN mkdir -p ${LOCAL_INSTALL} && cd ${LOCAL_INSTALL} \
    && wget -q ${CRONTAB_URL_DEPS}/${GO_DL} \
    && tar -zxf ${GO_DL} \
    && rm -f ${GO_DL}

# ── Create non-root user (tower:tower 1000:1000) ────────────────────────
RUN groupadd -g ${USER_GID} ${USER_NAME} \
    && useradd -m -u ${USER_UID} -g ${USER_GID} -s /bin/bash ${USER_NAME}

ENV PATH=/home/tower/.local/bin:${PATH}

# ── Project directory ────────────────────────────────────────────────────────
RUN mkdir -p ${WORKSPACE_HOME} \
    && chown -R ${USER_UID}:${USER_GID} /home/${USER_NAME}

# ── Entrypoint ───────────────────────────────────────────────────────────────
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# ── Claude Code ──────────────────────────────────────────────────────────────
USER tower 
RUN curl -fsSL https://claude.ai/install.sh | bash
USER root

WORKDIR ${WORKSPACE_HOME}
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/bin/bash"]

