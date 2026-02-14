#!/usr/bin/env bash
# council.sh - AI Council Multi-Agent Orchestrator
# Runs Claude Code, OpenAI Codex, Google Gemini CLI, and Mistral Vibe CLI
# in parallel inside Apple containers, then synthesizes results.
#
# Compatible with bash 3.2+ (macOS default)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Defaults ---
PROMPT=""
AGENTS_OVERRIDE=""
TIMEOUT_OVERRIDE=""
REBUILD=false
NO_SYNTH=false
VERBOSE=0
TUI_MODE=false
WORKSPACE="$(pwd)"
COLLAB_MODE=false
COLLAB_PATTERN=""
COLLAB_ROUNDS=""
COLLAB_GRAPH=""
COLLAB_DRY_RUN=false

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# --- TUI Event Emitter ---
emit_event() {
    local type="$1"
    local data="$2"
    if [ "$TUI_MODE" = true ]; then
        # Use date -u for UTC ISO8601
        local ts
        ts="$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)"
        printf '{"type":"%s","ts":"%s","data":%s}\n' "$type" "$ts" "$data"
    fi
}

# --- Usage ---
usage() {
    cat <<EOF
${BOLD}AI Council${NC} - Multi-Agent Orchestrator

${BOLD}Usage:${NC}
  council "prompt"                     Run all agents on prompt
  council "prompt" --agents claude,codex  Run specific agents only
  council "prompt" --no-synth          Skip synthesis, show raw outputs
  council "prompt" --timeout 600       Override timeout (seconds)
  council "prompt" --rebuild           Force rebuild container image
  council "prompt" --verbose           Show detailed progress
  council "prompt" --workspace /path   Mount specific directory

${BOLD}Collaborative Mode:${NC}
  council "prompt" --collab            Decompose task across agents
  council "prompt" --collab --pattern pipeline  Force coordination pattern
  council "prompt" --collab --pattern iterative --rounds 3  Multi-round review
  council "prompt" --collab --dry-run  Generate task graph, don't execute
  council "prompt" --collab --graph /path/to/taskgraph.json  Use pre-built graph

${BOLD}Options:${NC}
  --agents AGENTS    Comma-separated agent list (default: all enabled)
  --timeout SECS     Agent timeout in seconds (default: from config)
  --rebuild          Force rebuild the container image
  --no-synth         Skip synthesis step, display raw agent outputs
  --verbose          Enable verbose output
  --tui-mode         Output JSONL events for TUI consumption
  --workspace PATH   Directory to mount as /workspace (default: cwd)
  --collab           Enable collaborative mode (task decomposition)
  --pattern PATTERN  Coordination pattern: pipeline, parallel, iterative
  --rounds N         Max iterative rounds (default: 2, requires --collab)
  --dry-run          Show task graph without executing (requires --collab)
  --graph PATH       Use pre-built task graph JSON (requires --collab)
  -h, --help         Show this help

${BOLD}Examples:${NC}
  council "Fix the auth bug in login.py"
  council "Write a Python palindrome checker" --agents claude,gemini
  council "Explain how HTTP works" --no-synth --verbose
  council "Build a REST API with auth" --collab
  council "Design an algorithm" --collab --pattern iterative --rounds 3
EOF
    exit 0
}

# --- Parse args ---
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)    usage ;;
        --agents)     AGENTS_OVERRIDE="$2"; shift 2 ;;
        --timeout)    TIMEOUT_OVERRIDE="$2"; shift 2 ;;
        --rebuild)    REBUILD=true; shift ;;
        --no-synth)   NO_SYNTH=true; shift ;;
        --verbose)    VERBOSE=1; shift ;;
        --tui-mode)   TUI_MODE=true; shift ;;
        --workspace)  WORKSPACE="$2"; shift 2 ;;
        --collab)     COLLAB_MODE=true; shift ;;
        --pattern)    COLLAB_PATTERN="$2"; shift 2 ;;
        --rounds)     COLLAB_ROUNDS="$2"; shift 2 ;;
        --graph)      COLLAB_GRAPH="$2"; shift 2 ;;
        --dry-run)    COLLAB_DRY_RUN=true; shift ;;
        -*)           echo "Unknown option: $1"; usage ;;
        *)
            if [ -z "$PROMPT" ]; then
                PROMPT="$1"
            else
                echo "Unexpected argument: $1"; usage
            fi
            shift
            ;;
    esac
done

if [ -z "$PROMPT" ]; then
    echo -e "${RED}Error:${NC} No prompt provided"
    echo ""
    usage
fi

# --- Load config ---
source "${SCRIPT_DIR}/config/council.conf"

# Validate auth config
if [ "${AUTH_MODE}" = "apikey" ]; then
    ENV_FILE="${SCRIPT_DIR}/config/council.env"
    if [ ! -f "$ENV_FILE" ]; then
        echo -e "${RED}Error:${NC} council.env not found. Run setup.sh first or switch to AUTH_MODE=oauth."
        exit 1
    fi
fi

# Apply overrides
if [ -n "$TIMEOUT_OVERRIDE" ]; then
    AGENT_TIMEOUT="$TIMEOUT_OVERRIDE"
fi

# Determine active agents
if [ -n "$AGENTS_OVERRIDE" ]; then
    ACTIVE_AGENTS="${AGENTS_OVERRIDE//,/ }"
else
    ACTIVE_AGENTS="$ENABLED_AGENTS"
fi

# Validate workspace
WORKSPACE="$(cd "$WORKSPACE" && pwd)"
if [ ! -d "$WORKSPACE" ]; then
    echo -e "${RED}Error:${NC} Workspace directory does not exist: $WORKSPACE"
    exit 1
fi

# --- Source libraries ---
source "${SCRIPT_DIR}/lib/container-ops.sh"
source "${SCRIPT_DIR}/lib/output-collector.sh"
source "${SCRIPT_DIR}/lib/synthesizer.sh"

# --- Create session ---
SESSION_ID="$(date +%Y%m%d-%H%M%S)"
SESSION_DIR="${SCRIPT_DIR}/output/${SESSION_ID}"
LOG_DIR="${SCRIPT_DIR}/logs/${SESSION_ID}"
mkdir -p "$SESSION_DIR" "$LOG_DIR"

session_data=$(jq -cn --arg id "$SESSION_ID" --arg p "$PROMPT" --arg a "$ACTIVE_AGENTS" --arg w "$WORKSPACE" '{"id":$id, "prompt":$p, "agents":$a, "workspace":$w}')
emit_event "session.started" "$session_data"

# --- Trap for cleanup ---
cleanup() {
    echo ""
    echo -e "${DIM}[cleanup] Stopping containers...${NC}"
    cleanup_containers
}
trap cleanup EXIT INT TERM

# --- Header ---
echo ""
echo -e "${BOLD}AI Council${NC} - Session ${DIM}${SESSION_ID}${NC}"
echo -e "${DIM}────────────────────────────────────────${NC}"
echo -e "  Prompt:    ${PROMPT}"
echo -e "  Agents:    ${ACTIVE_AGENTS}"
echo -e "  Auth:      ${AUTH_MODE}"
echo -e "  Timeout:   ${AGENT_TIMEOUT}s"
echo -e "  Workspace: ${WORKSPACE}"
echo -e "${DIM}────────────────────────────────────────${NC}"
echo ""

# --- Build image ---
build_image "$REBUILD"

# --- Collaborative mode ---
if [ "$COLLAB_MODE" = true ]; then
    source "${SCRIPT_DIR}/lib/collab.sh"

    if run_collab "$PROMPT"; then
        # Collab succeeded — skip standard execution path
        exit 0
    else
        echo ""
        echo -e "${YELLOW}Falling back to standard council mode...${NC}"
        echo ""
        # Continue to standard parallel execution below
    fi
fi

# --- Launch agents in parallel ---
echo -e "${BOLD}Launching agents...${NC}"

# Track PIDs using parallel indexed arrays (bash 3.2 compatible)
AGENT_NAMES=()
AGENT_PIDS=()

for agent in $ACTIVE_AGENTS; do
    output_file="${SESSION_DIR}/${agent}-raw.txt"
    log_file="${LOG_DIR}/${agent}.log"

    agent_data=$(jq -cn --arg a "$agent" '{"agent":$a}')
    emit_event "agent.started" "$agent_data"
    run_agent "$agent" "$WORKSPACE" "$PROMPT" "$output_file" "$log_file" "$TUI_MODE" &
    AGENT_NAMES+=("$agent")
    AGENT_PIDS+=($!)
done

# --- Wait with progress ---
echo ""

i=0
while [ $i -lt ${#AGENT_PIDS[@]} ]; do
    agent="${AGENT_NAMES[$i]}"
    pid="${AGENT_PIDS[$i]}"
    if wait "$pid" 2>/dev/null; then
        echo -e "  ${GREEN}✓${NC} ${agent} completed"
    else
        echo -e "  ${RED}✗${NC} ${agent} failed"
    fi
    i=$((i + 1))
done

echo ""

# --- Collect and validate outputs ---
echo -e "${BOLD}Processing outputs...${NC}"
success_count=0

for agent in $ACTIVE_AGENTS; do
    raw_file="${SESSION_DIR}/${agent}-raw.txt"
    json_file="${SESSION_DIR}/${agent}.json"

    extract_json "$agent" "$raw_file" > "$json_file" 2>/dev/null || true

    status=$(validate_result "$agent" "$json_file")
    echo -e "  ${agent}: ${status}"

    usage="null"
    if [ "$status" = "success" ]; then
        success_count=$((success_count + 1))
        # Extract usage if available
        usage=$(jq -c '.usage // .item.usage // null' "$json_file" 2>/dev/null || echo "null")
    fi

    agent_comp_data=$(jq -cn --arg a "$agent" --arg s "$status" --argjson u "$usage" '{"agent":$a, "status":$s, "usage":$u}')
    emit_event "agent.completed" "$agent_comp_data"
done

echo ""

# --- Build envelope ---
envelope_file="${SESSION_DIR}/envelope.json"
build_envelope "$SESSION_DIR" "$PROMPT" $ACTIVE_AGENTS > "$envelope_file"

# --- Show raw outputs if --no-synth ---
if [ "$NO_SYNTH" = true ]; then
    echo -e "${BOLD}Agent Outputs (no synthesis):${NC}"
    echo ""
    for agent in $ACTIVE_AGENTS; do
        json_file="${SESSION_DIR}/${agent}.json"
        echo -e "${BLUE}--- ${agent} ---${NC}"
        if [ -f "$json_file" ]; then
            jq '.' "$json_file" 2>/dev/null || cat "$json_file"
        else
            echo "(no output)"
        fi
        echo ""
    done
    echo -e "${DIM}Session: ${SESSION_DIR}${NC}"
    exit 0
fi

# --- Synthesize ---
if [ "$success_count" -eq 0 ]; then
    echo -e "${RED}All agents failed. No synthesis possible.${NC}"
    echo -e "${DIM}Check logs: ${LOG_DIR}${NC}"
    exit 1
fi

echo -e "${BOLD}Synthesizing results...${NC}"
synth_file="${SESSION_DIR}/synthesis.json"

emit_event "synthesis.started" "{}"
if run_synthesis "$envelope_file" "$synth_file"; then
    synth_result=$(cat "$synth_file" 2>/dev/null | jq -c . || echo "null")
    synth_comp_data=$(jq -cn --argjson r "$synth_result" '{"result":$r}')
    emit_event "synthesis.completed" "$synth_comp_data"
    echo ""
    echo -e "${BOLD}${GREEN}=== Council Result ===${NC}"
    echo ""

    # Pretty-print the synthesis
    if jq '.' "$synth_file" 2>/dev/null; then
        : # jq already printed
    else
        cat "$synth_file"
    fi
else
    emit_event "synthesis.completed" '{"result":null}'
    echo -e "${YELLOW}Synthesis failed. Showing raw envelope:${NC}"
    jq '.' "$envelope_file"
fi

# --- Summary ---
echo ""
echo -e "${DIM}────────────────────────────────────────${NC}"
echo -e "  Session:  ${SESSION_DIR}"
echo -e "  Logs:     ${LOG_DIR}"

for agent in $ACTIVE_AGENTS; do
    status=$(validate_result "$agent" "${SESSION_DIR}/${agent}.json" 2>/dev/null || echo "unknown")
    case "$status" in
        success) icon="${GREEN}✓${NC}" ;;
        timeout) icon="${YELLOW}⏱${NC}" ;;
        *)       icon="${RED}✗${NC}" ;;
    esac
    echo -e "  ${icon} ${agent}: ${status}"
done

echo -e "${DIM}────────────────────────────────────────${NC}"
echo ""
