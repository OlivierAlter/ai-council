FROM docker.io/library/node:22-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl ca-certificates jq python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g \
    @anthropic-ai/claude-code@latest \
    @openai/codex@latest \
    @google/gemini-cli@latest

# Install uv (Python package manager) then install Mistral Vibe CLI
# Vibe requires Python 3.12+ but node:22-slim has 3.11; uv manages its own Python
RUN curl -LsSf https://astral.sh/uv/install.sh | sh
ENV PATH="/root/.local/bin:/root/.cargo/bin:${PATH}"
RUN uv tool install mistral-vibe

# Ensure npm global binaries and uv tools are in PATH for all shells
ENV PATH="/root/.local/bin:/root/.cargo/bin:/usr/local/lib/node_modules/.bin:/usr/local/bin:/usr/bin:/bin:${PATH}"

# Verify the CLIs are findable
RUN claude --version && codex --version && gemini --version && vibe --version

# Create non-root user for Claude (refuses --dangerously-skip-permissions as root)
RUN useradd -m -s /bin/sh council

RUN mkdir -p /workspace
WORKDIR /workspace
