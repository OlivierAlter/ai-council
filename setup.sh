#!/usr/bin/env bash
# setup.sh - First-time setup for AI Council

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/config/council.conf"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[info]${NC}  $*"; }
ok()    { echo -e "${GREEN}[ok]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC}  $*"; }
fail()  { echo -e "${RED}[fail]${NC}  $*"; }

echo ""
echo "==================================="
echo "  AI Council - Setup"
echo "==================================="
echo ""

# Step 1: Verify macOS version and Apple Silicon
info "Checking system requirements..."

OS_NAME="$(uname -s)"
if [ "$OS_NAME" != "Darwin" ]; then
    fail "This tool requires macOS. Detected: $OS_NAME"
    exit 1
fi
ok "macOS detected"

ARCH="$(uname -m)"
if [ "$ARCH" != "arm64" ]; then
    fail "Apple Silicon (arm64) required. Detected: $ARCH"
    exit 1
fi
ok "Apple Silicon detected"

OS_VERSION="$(sw_vers -productVersion)"
MAJOR_VERSION="${OS_VERSION%%.*}"
if [ "$MAJOR_VERSION" -lt 26 ]; then
    warn "macOS 26 (Tahoe) recommended. Detected: macOS $OS_VERSION"
    warn "Apple containers require macOS 26+. Proceeding anyway..."
else
    ok "macOS $OS_VERSION (Tahoe)"
fi

# Step 2: Verify container CLI
info "Checking for container CLI..."
if command -v container &>/dev/null; then
    ok "container CLI found: $(which container)"
else
    fail "container CLI not found"
    echo ""
    echo "  Install Apple's container CLI:"
    echo "  https://github.com/apple/container/releases"
    echo ""
    echo "  Or build from source:"
    echo "    git clone https://github.com/apple/container.git"
    echo "    cd container && swift build -c release"
    echo "    cp .build/release/container /usr/local/bin/"
    echo ""
    exit 1
fi

# Step 3: Verify Claude CLI on host (needed for synthesizer)
info "Checking for Claude CLI on host..."
if command -v claude &>/dev/null; then
    ok "claude CLI found: $(which claude)"
else
    fail "claude CLI not found on host (required for synthesis)"
    echo ""
    echo "  Install Claude Code:"
    echo "    npm install -g @anthropic-ai/claude-code"
    echo ""
    exit 1
fi

# Step 4: Verify authentication
info "Auth mode: ${AUTH_MODE}"

if [ "${AUTH_MODE}" = "oauth" ]; then
    # OAuth mode - check that host credential directories exist
    echo ""
    info "Checking host CLI credentials..."

    oauth_ready=0

    # Claude
    if [ -d "${CLAUDE_AUTH_DIR}" ]; then
        ok "Claude credentials found: ${CLAUDE_AUTH_DIR}"
        oauth_ready=$((oauth_ready + 1))
    else
        warn "Claude credentials not found at ${CLAUDE_AUTH_DIR}"
        echo "    Run 'claude' on the host and complete the login flow first."
    fi

    # Codex
    if [ -f "${CODEX_AUTH_DIR}/auth.json" ]; then
        ok "Codex credentials found: ${CODEX_AUTH_DIR}/auth.json"
        oauth_ready=$((oauth_ready + 1))
    elif [ -d "${CODEX_AUTH_DIR}" ]; then
        warn "Codex dir exists but no auth.json found"
        echo "    Run 'codex' on the host and complete the login flow first."
    else
        warn "Codex credentials not found at ${CODEX_AUTH_DIR}"
        echo "    Run 'codex' on the host and complete the login flow first."
    fi

    # Gemini
    if [ -d "${GEMINI_AUTH_DIR}" ]; then
        ok "Gemini credentials found: ${GEMINI_AUTH_DIR}"
        oauth_ready=$((oauth_ready + 1))
    else
        warn "Gemini credentials not found at ${GEMINI_AUTH_DIR}"
        echo "    Run 'gemini' on the host and complete the login flow first."
    fi

    # Vibe
    if [ -f "${VIBE_AUTH_DIR}/.env" ]; then
        ok "Vibe credentials found: ${VIBE_AUTH_DIR}/.env"
        oauth_ready=$((oauth_ready + 1))
    elif [ -d "${VIBE_AUTH_DIR}" ]; then
        warn "Vibe dir exists but no .env found"
        echo "    Run 'vibe --setup' on the host and add your MISTRAL_API_KEY."
    else
        warn "Vibe credentials not found at ${VIBE_AUTH_DIR}"
        echo "    Run 'vibe --setup' on the host or create ~/.vibe/.env with MISTRAL_API_KEY=..."
    fi

    echo ""
    if [ "$oauth_ready" -eq 0 ]; then
        fail "No CLI credentials found. Log in to at least one CLI on the host first."
        echo ""
        echo "  Each CLI needs to be logged in on the host so we can mount"
        echo "  the credentials into the containers:"
        echo "    claude    -> logs in via OAuth, stores in ~/.claude/"
        echo "    codex     -> logs in via ChatGPT OAuth, stores in ~/.codex/"
        echo "    gemini    -> logs in via Google OAuth, stores in ~/.gemini/"
        echo "    vibe      -> run 'vibe --setup', stores key in ~/.vibe/.env"
        echo ""
        exit 1
    fi
    info "$oauth_ready of 4 agent credentials found"

else
    # API key mode - set up council.env
    info "Setting up API keys..."
    ENV_FILE="${SCRIPT_DIR}/config/council.env"
    ENV_EXAMPLE="${SCRIPT_DIR}/config/council.env.example"

    if [ -f "$ENV_FILE" ]; then
        ok "council.env already exists"
    else
        if [ -f "$ENV_EXAMPLE" ]; then
            cp "$ENV_EXAMPLE" "$ENV_FILE"
            warn "Created council.env from template"
            echo ""
            echo "  Please edit ${ENV_FILE} and add your API keys:"
            echo "    ANTHROPIC_API_KEY=sk-ant-..."
            echo "    OPENAI_API_KEY=sk-..."
            echo "    GEMINI_API_KEY=..."
            echo ""
            read -rp "  Press Enter after you've added your keys (or Ctrl+C to exit)..."
        else
            fail "council.env.example not found"
            exit 1
        fi
    fi

    # Validate keys are not placeholders
    validate_key() {
        local key_name="$1"
        local key_value
        key_value=$(grep "^${key_name}=" "$ENV_FILE" | cut -d= -f2-)
        if [ -z "$key_value" ] || echo "$key_value" | grep -qi "your.*key\|your.*here"; then
            warn "$key_name appears to be a placeholder - agent may fail"
            return 1
        fi
        ok "$key_name configured"
        return 0
    }

    local_valid=0
    validate_key "ANTHROPIC_API_KEY" && local_valid=$((local_valid + 1))
    validate_key "OPENAI_API_KEY" && local_valid=$((local_valid + 1))
    validate_key "GEMINI_API_KEY" && local_valid=$((local_valid + 1))
    validate_key "MISTRAL_API_KEY" && local_valid=$((local_valid + 1))

    if [ "$local_valid" -eq 0 ]; then
        fail "No valid API keys found. At least one is required."
        exit 1
    fi
fi

# Step 5: Build container image
echo ""
info "Building container image..."
source "${SCRIPT_DIR}/lib/container-ops.sh"
build_image "true"
ok "Image ${IMAGE_NAME}:${IMAGE_TAG} built"

# Step 6: Smoke test
info "Running smoke tests..."

smoke_test() {
    local agent="$1"
    local tmp_out tmp_log
    tmp_out="$(mktemp)"
    tmp_log="$(mktemp)"

    # Use a short timeout for smoke test
    local saved_timeout="$AGENT_TIMEOUT"
    AGENT_TIMEOUT=30

    if run_agent "$agent" "$(pwd)" "Respond with exactly: hello" "$tmp_out" "$tmp_log" 2>/dev/null; then
        ok "Smoke test passed: $agent"
    else
        warn "Smoke test failed: $agent (check stderr in logs)"
    fi

    AGENT_TIMEOUT="$saved_timeout"
    rm -f "$tmp_out" "$tmp_log"
}

check_agent_ready() {
    local agent="$1"
    if [ "${AUTH_MODE}" = "oauth" ]; then
        case "$agent" in
            claude) [ -d "${CLAUDE_AUTH_DIR}" ] && return 0 ;;
            codex)  [ -f "${CODEX_AUTH_DIR}/auth.json" ] && return 0 ;;
            gemini) [ -d "${GEMINI_AUTH_DIR}" ] && return 0 ;;
            vibe)   [ -f "${VIBE_AUTH_DIR}/.env" ] && return 0 ;;
        esac
        return 1
    else
        local env_file="${SCRIPT_DIR}/config/council.env"
        local key_name=""
        case "$agent" in
            claude)  key_name="ANTHROPIC_API_KEY" ;;
            codex)   key_name="OPENAI_API_KEY" ;;
            gemini)  key_name="GEMINI_API_KEY" ;;
            vibe)    key_name="MISTRAL_API_KEY" ;;
        esac
        local key_val
        key_val=$(grep "^${key_name}=" "$env_file" 2>/dev/null | cut -d= -f2- || true)
        if [ -n "$key_val" ] && ! echo "$key_val" | grep -qi "your.*key\|your.*here"; then
            return 0
        fi
        return 1
    fi
}

for agent in $ENABLED_AGENTS; do
    if check_agent_ready "$agent"; then
        smoke_test "$agent"
    else
        warn "Skipping smoke test for $agent (no credentials)"
    fi
done

# Step 7: Symlink
echo ""
read -rp "Symlink 'council' to /usr/local/bin/council? (requires sudo) [y/N] " symlink_choice
if [[ "$symlink_choice" =~ ^[Yy]$ ]]; then
    sudo ln -sf "${SCRIPT_DIR}/council.sh" /usr/local/bin/council
    ok "Symlinked: council -> ${SCRIPT_DIR}/council.sh"
else
    info "Skipped symlink. Run directly with: ${SCRIPT_DIR}/council.sh"
    info "Or add to PATH: export PATH=\"${SCRIPT_DIR}:\$PATH\""
fi

echo ""
echo "==================================="
echo "  Setup complete!"
echo "==================================="
echo ""
echo "  Auth mode: ${AUTH_MODE}"
if [ "${AUTH_MODE}" = "oauth" ]; then
    echo "  Credentials mounted from host ~/.claude, ~/.codex, ~/.gemini, ~/.vibe"
fi
echo ""
echo "  Usage:"
echo "    council \"Your prompt here\""
echo "    council \"Fix the auth bug\" --agents claude,codex"
echo "    council \"Explain HTTP\" --no-synth --verbose"
echo ""
