#!/usr/bin/env bash
# container-ops.sh - Build/run/cleanup helpers for Apple containers

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Claude Code OAuth constants (extracted from CLI binary)
CLAUDE_OAUTH_TOKEN_URL="https://platform.claude.com/v1/oauth/token"
CLAUDE_OAUTH_CLIENT_ID="9d1c250a-e61b-44d9-88ed-5944d1962f5e"
# Refresh buffer: refresh if token expires within this many seconds
CLAUDE_OAUTH_REFRESH_BUFFER=300  # 5 minutes

# Ensure Claude's OAuth access token is valid, refreshing if needed.
# Reads from macOS Keychain, checks expiresAt, and if expired/expiring soon,
# uses the refresh token to get a new access token and updates the Keychain.
# Returns: the valid access token on stdout, or empty string on failure.
ensure_claude_oauth_token() {
    local claude_creds
    claude_creds=$(security find-generic-password -s "Claude Code-credentials" -w 2>/dev/null || true)
    if [ -z "$claude_creds" ]; then
        echo ""
        return 0
    fi

    local access_token expires_at refresh_token
    access_token=$(echo "$claude_creds" | jq -r '.claudeAiOauth.accessToken // empty' 2>/dev/null || true)
    expires_at=$(echo "$claude_creds" | jq -r '.claudeAiOauth.expiresAt // empty' 2>/dev/null || true)
    refresh_token=$(echo "$claude_creds" | jq -r '.claudeAiOauth.refreshToken // empty' 2>/dev/null || true)

    if [ -z "$access_token" ]; then
        echo ""
        return 0
    fi

    # Check if token is still valid (with buffer)
    local now_ms
    now_ms=$(python3 -c "import time; print(int(time.time() * 1000))" 2>/dev/null || date +%s000)
    local buffer_ms=$((CLAUDE_OAUTH_REFRESH_BUFFER * 1000))

    if [ -n "$expires_at" ] && [ "$((now_ms + buffer_ms))" -lt "$expires_at" ] 2>/dev/null; then
        # Token is still valid
        [ "${VERBOSE:-0}" = "1" ] && echo "[claude] OAuth token valid (expires in $(( (expires_at - now_ms) / 1000 / 60 ))m)" >&2
        echo "$access_token"
        return 0
    fi

    # Token expired or expiring soon — try to refresh
    if [ -z "$refresh_token" ]; then
        echo "[claude] OAuth token expired and no refresh token available" >&2
        echo ""
        return 0
    fi

    echo "[claude] OAuth token expired/expiring, refreshing..." >&2
    local refresh_response
    refresh_response=$(curl -s -X POST "$CLAUDE_OAUTH_TOKEN_URL" \
        -H "Content-Type: application/x-www-form-urlencoded" \
        -d "grant_type=refresh_token" \
        -d "refresh_token=${refresh_token}" \
        -d "client_id=${CLAUDE_OAUTH_CLIENT_ID}" 2>/dev/null || true)

    local new_access_token new_refresh_token expires_in
    new_access_token=$(echo "$refresh_response" | jq -r '.access_token // empty' 2>/dev/null || true)
    new_refresh_token=$(echo "$refresh_response" | jq -r '.refresh_token // empty' 2>/dev/null || true)
    expires_in=$(echo "$refresh_response" | jq -r '.expires_in // empty' 2>/dev/null || true)

    if [ -z "$new_access_token" ]; then
        local error_msg
        error_msg=$(echo "$refresh_response" | jq -r '.error_description // .error // empty' 2>/dev/null || true)
        echo "[claude] OAuth refresh failed: ${error_msg:-unknown error}. Run 'claude /login' to re-authenticate." >&2
        # Return the old (expired) token as a last resort — it will fail but with a clear error
        echo "$access_token"
        return 0
    fi

    # Calculate new expiresAt
    local new_expires_at
    if [ -n "$expires_in" ]; then
        new_expires_at=$(python3 -c "import time; print(int(time.time() * 1000) + ${expires_in} * 1000)" 2>/dev/null || true)
    else
        new_expires_at="$expires_at"
    fi

    # Update Keychain with new tokens
    local updated_creds
    updated_creds=$(echo "$claude_creds" | jq \
        --arg at "$new_access_token" \
        --arg rt "${new_refresh_token:-$refresh_token}" \
        --argjson ea "${new_expires_at:-0}" \
        '.claudeAiOauth.accessToken = $at | .claudeAiOauth.refreshToken = $rt | .claudeAiOauth.expiresAt = $ea' \
        2>/dev/null || true)

    if [ -n "$updated_creds" ]; then
        # Delete old entry and add updated one
        security delete-generic-password -s "Claude Code-credentials" 2>/dev/null || true
        security add-generic-password -s "Claude Code-credentials" -a "Claude Code" -w "$updated_creds" 2>/dev/null || true
        echo "[claude] OAuth token refreshed successfully (valid for $((expires_in / 60))m)" >&2
    else
        echo "[claude] Warning: refreshed token but failed to update Keychain" >&2
    fi

    echo "$new_access_token"
}

build_image() {
    local rebuild="${1:-false}"
    local image="${IMAGE_NAME}:${IMAGE_TAG}"

    # Check if image already exists
    if [ "$rebuild" = "false" ]; then
        if container image list 2>/dev/null | grep -q "$IMAGE_NAME.*$IMAGE_TAG"; then
            [ "${VERBOSE:-0}" = "1" ] && echo "[build] Image $image already exists, skipping"
            return 0
        fi
    fi

    echo "[build] Building $image..."
    container build --tag "$image" "$SCRIPT_DIR"
}

# Get auth mount/env args for a given agent
# OAuth mode: mount host credential directories into the container
# API key mode: pass env vars from council.env
# Gemini always needs GEMINI_API_KEY (no OAuth support in headless CLI)
get_auth_args() {
    local agent="$1"
    local env_file="${SCRIPT_DIR}/config/council.env"

    if [ "${AUTH_MODE}" = "oauth" ]; then
        case "$agent" in
            claude)
                # Mount ~/.claude/ for config/hooks
                if [ -d "${CLAUDE_AUTH_DIR}" ]; then
                    echo "--mount source=${CLAUDE_AUTH_DIR},target=/home/council/.claude"
                fi
                # Claude Code stores OAuth in macOS Keychain. Extract the access token
                # at runtime, refresh if expired, and pass as CLAUDE_CODE_OAUTH_TOKEN.
                local oauth_token
                oauth_token=$(ensure_claude_oauth_token)
                if [ -n "$oauth_token" ]; then
                    echo "-e CLAUDE_CODE_OAUTH_TOKEN=${oauth_token}"
                else
                    # Fallback to council.env
                    if [ -f "$env_file" ]; then
                        local claude_key
                        claude_key=$(grep '^ANTHROPIC_API_KEY=' "$env_file" 2>/dev/null | cut -d= -f2- || true)
                        if [ -n "$claude_key" ] && ! echo "$claude_key" | grep -qi "your.*key\|your.*here"; then
                            echo "-e ANTHROPIC_API_KEY=${claude_key}"
                        else
                            echo "[claude] Warning: no OAuth token in Keychain and no ANTHROPIC_API_KEY in council.env" >&2
                        fi
                    else
                        echo "[claude] Warning: no OAuth token in Keychain and no council.env found" >&2
                    fi
                fi
                ;;
            codex)
                if [ -d "${CODEX_AUTH_DIR}" ]; then
                    echo "--mount source=${CODEX_AUTH_DIR},target=/root/.codex"
                else
                    echo "[${agent}] Warning: ${CODEX_AUTH_DIR} not found, auth may fail" >&2
                fi
                ;;
            gemini)
                if [ -d "${GEMINI_AUTH_DIR}" ]; then
                    echo "--mount source=${GEMINI_AUTH_DIR},target=/root/.gemini"
                else
                    echo "[${agent}] Warning: ${GEMINI_AUTH_DIR} not found, auth may fail" >&2
                fi
                ;;
            vibe)
                # Vibe stores API key in ~/.vibe/.env and config in ~/.vibe/config.toml
                # Apple containers only support directory mounts, so we create a merged
                # temp directory with the host .env + our container-specific config.toml
                local vibe_merged
                vibe_merged=$(mktemp -d "${TMPDIR:-/tmp}/council-vibe.XXXXXX")
                if [ -f "${VIBE_AUTH_DIR}/.env" ]; then
                    cp "${VIBE_AUTH_DIR}/.env" "$vibe_merged/.env"
                fi
                local vibe_container_conf="${SCRIPT_DIR}/config/vibe-container.toml"
                if [ -f "$vibe_container_conf" ]; then
                    cp "$vibe_container_conf" "$vibe_merged/config.toml"
                fi
                echo "--mount source=${vibe_merged},target=/root/.vibe"
                if [ ! -d "${VIBE_AUTH_DIR}" ]; then
                    echo "[${agent}] Warning: ${VIBE_AUTH_DIR} not found, auth may fail" >&2
                fi
                # Also extract MISTRAL_API_KEY from ~/.vibe/.env and pass as env var
                if [ -f "${VIBE_AUTH_DIR}/.env" ]; then
                    local mistral_key
                    mistral_key=$(grep '^MISTRAL_API_KEY=' "${VIBE_AUTH_DIR}/.env" 2>/dev/null | cut -d= -f2- || true)
                    if [ -n "$mistral_key" ]; then
                        echo "-e MISTRAL_API_KEY=${mistral_key}"
                    fi
                fi
                ;;
        esac
    else
        # API key mode - read from council.env
        if [ ! -f "$env_file" ]; then
            echo "[${agent}] Error: council.env not found (required for apikey mode)" >&2
            return 1
        fi
        case "$agent" in
            claude)
                echo "-e ANTHROPIC_API_KEY=$(grep '^ANTHROPIC_API_KEY=' "$env_file" | cut -d= -f2-)"
                ;;
            codex)
                echo "-e OPENAI_API_KEY=$(grep '^OPENAI_API_KEY=' "$env_file" | cut -d= -f2-)"
                ;;
            gemini)
                echo "-e GEMINI_API_KEY=$(grep '^GEMINI_API_KEY=' "$env_file" | cut -d= -f2-)"
                ;;
            vibe)
                echo "-e MISTRAL_API_KEY=$(grep '^MISTRAL_API_KEY=' "$env_file" | cut -d= -f2-)"
                ;;
        esac
    fi
}

run_agent() {
    local agent="$1"
    local workspace="$2"
    local prompt="$3"
    local output_file="$4"
    local log_file="$5"
    local tui_mode="${6:-false}"
    local prompt_file="${7:-}"

    local image="${IMAGE_NAME}:${IMAGE_TAG}"

    # Build CLI command based on agent.
    # Two modes:
    #   1. Inline prompt via COUNCIL_PROMPT env var (standard mode)
    #   2. File-based prompt via COUNCIL_CONTEXT_FILE (collab mode — avoids env var size limits)
    local cmd
    local use_prompt_file=false
    local prompt_mount_args=""

    if [ -n "$prompt_file" ] && [ -f "$prompt_file" ]; then
        # File-based prompt: mount the prompt file directory into the container
        use_prompt_file=true
        local prompt_dir
        prompt_dir="$(cd "$(dirname "$prompt_file")" && pwd)"
        local prompt_basename
        prompt_basename="$(basename "$prompt_file")"
        prompt_mount_args="--mount source=${prompt_dir},target=/council-prompt -e COUNCIL_CONTEXT_FILE=/council-prompt/${prompt_basename}"

        case "$agent" in
            claude)
                cmd='claude -p "$(cat "$COUNCIL_CONTEXT_FILE")" --output-format json --dangerously-skip-permissions'
                ;;
            codex)
                cmd='codex exec --json --yolo --skip-git-repo-check "$(cat "$COUNCIL_CONTEXT_FILE")"'
                ;;
            gemini)
                cmd='gemini -p "$(cat "$COUNCIL_CONTEXT_FILE")" --output-format json --yolo'
                ;;
            vibe)
                cmd='vibe -p "$(cat "$COUNCIL_CONTEXT_FILE")" --output json'
                ;;
            *)
                echo "[error] Unknown agent: $agent" >&2
                return 1
                ;;
        esac
    else
        # Inline prompt via env var (standard mode)
        # NOTE: COUNCIL_PROMPT is passed with quotes directly in the container run call
        # below, NOT via a variable, because the prompt contains spaces that would break
        # word splitting if stored in an unquoted variable.
        case "$agent" in
            claude)
                cmd='claude -p "$COUNCIL_PROMPT" --output-format json --dangerously-skip-permissions'
                ;;
            codex)
                cmd='codex exec --json --yolo --skip-git-repo-check "$COUNCIL_PROMPT"'
                ;;
            gemini)
                cmd='gemini -p "$COUNCIL_PROMPT" --output-format json --yolo'
                ;;
            vibe)
                cmd='vibe -p "$COUNCIL_PROMPT" --output json'
                ;;
            *)
                echo "[error] Unknown agent: $agent" >&2
                return 1
                ;;
        esac
    fi

    # Get auth-specific container args
    local auth_args
    auth_args="$(get_auth_args "$agent")"

    # Claude refuses --dangerously-skip-permissions as root, so run as non-root user
    local user_args=""
    if [ "$agent" = "claude" ]; then
        user_args="--user council"
    fi

    echo "[${agent}] Starting container (auth: ${AUTH_MODE})..."

    # Run container with timeout (macOS-compatible: background + kill after deadline)
    # --entrypoint /bin/sh overrides node:22-slim's default ENTRYPOINT ["node"]
    #
    # Two container run paths:
    #   - Standard mode: pass prompt via -e "COUNCIL_PROMPT=..." (quoted to preserve spaces)
    #   - File mode: mount prompt file + set COUNCIL_CONTEXT_FILE (no spaces in paths)
    # shellcheck disable=SC2086
    if [ "$use_prompt_file" = true ]; then
        if [ "$tui_mode" = "true" ]; then
            container run \
                --rm \
                --entrypoint /bin/sh \
                $user_args \
                $auth_args \
                $prompt_mount_args \
                --mount "source=${workspace},target=/workspace" \
                "$image" \
                -c "$cmd" \
                2>&1 | while IFS= read -r line; do
                    echo "$line" >> "$output_file"
                    echo "$line" >> "$log_file"
                    local event_data
                    event_data=$(jq -cn --arg a "$agent" --arg l "$line" '{"agent":$a, "line":$l}')
                    emit_event "agent.output" "$event_data"
                done &
            local container_pid=$!
        else
            container run \
                --rm \
                --entrypoint /bin/sh \
                $user_args \
                $auth_args \
                $prompt_mount_args \
                --mount "source=${workspace},target=/workspace" \
                "$image" \
                -c "$cmd" \
                > "$output_file" 2> "$log_file" &
            local container_pid=$!
        fi
    else
        if [ "$tui_mode" = "true" ]; then
            container run \
                --rm \
                --entrypoint /bin/sh \
                $user_args \
                $auth_args \
                -e "COUNCIL_PROMPT=${prompt}" \
                --mount "source=${workspace},target=/workspace" \
                "$image" \
                -c "$cmd" \
                2>&1 | while IFS= read -r line; do
                    echo "$line" >> "$output_file"
                    echo "$line" >> "$log_file"
                    local event_data
                    event_data=$(jq -cn --arg a "$agent" --arg l "$line" '{"agent":$a, "line":$l}')
                    emit_event "agent.output" "$event_data"
                done &
            local container_pid=$!
        else
            container run \
                --rm \
                --entrypoint /bin/sh \
                $user_args \
                $auth_args \
                -e "COUNCIL_PROMPT=${prompt}" \
                --mount "source=${workspace},target=/workspace" \
                "$image" \
                -c "$cmd" \
                > "$output_file" 2> "$log_file" &
            local container_pid=$!
        fi
    fi

    # Watchdog: kill after AGENT_TIMEOUT seconds
    (
        sleep "${AGENT_TIMEOUT}" 2>/dev/null
        kill "$container_pid" 2>/dev/null
    ) &
    local watchdog_pid=$!

    if wait "$container_pid" 2>/dev/null; then
        kill "$watchdog_pid" 2>/dev/null || true
        wait "$watchdog_pid" 2>/dev/null || true

        echo "[${agent}] Completed successfully"
        return 0
    else
        local exit_code=$?
        kill "$watchdog_pid" 2>/dev/null || true
        wait "$watchdog_pid" 2>/dev/null || true

        if [ $exit_code -eq 143 ] || [ $exit_code -eq 137 ]; then
            echo "[${agent}] Timed out after ${AGENT_TIMEOUT}s"
            echo '{"error": "timeout", "agent": "'"$agent"'"}' > "$output_file"
        else
            echo "[${agent}] Failed with exit code $exit_code"
            # Only write error JSON if the agent produced no output
            # (e.g. Claude may succeed but exit non-zero due to hook failures)
            if [ ! -s "$output_file" ]; then
                echo '{"error": "exit_code_'"$exit_code"'", "agent": "'"$agent"'"}' > "$output_file"
            fi
        fi
        return $exit_code
    fi
}

cleanup_containers() {
    echo "[cleanup] Removing any leftover council containers..."
    local running
    running=$(container list 2>/dev/null | grep "$IMAGE_NAME" | awk '{print $1}' || true)
    if [ -n "$running" ]; then
        echo "$running" | while read -r cid; do
            container stop "$cid" 2>/dev/null || true
            container rm "$cid" 2>/dev/null || true
        done
    fi
}
