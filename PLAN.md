# AI Council - Apple Container Multi-Agent Orchestrator

Run Claude Code, OpenAI Codex, Google Gemini CLI, and Mistral Vibe CLI in parallel inside Apple containers on macOS 26 Tahoe, then synthesize their results into a single best answer.

## Prerequisites

- macOS 26 Tahoe on Apple Silicon
- [Apple `container` CLI](https://github.com/apple/container/releases)
- [Claude Code](https://www.npmjs.com/package/@anthropic-ai/claude-code) installed on the host (used for synthesis)
- All four CLIs installed and authenticated on the host via OAuth / API key

## Quick Start

```bash
# 1. Clone/copy this folder to the target machine

# 2. Install all 4 CLIs on the host
npm install -g @anthropic-ai/claude-code @openai/codex @google/gemini-cli
pip install mistral-vibe   # or: uv tool install mistral-vibe

# 3. Log in to each CLI on the host (do this BEFORE setup)
claude        # follow OAuth login flow, stores creds in ~/.claude/
codex         # follow ChatGPT OAuth login flow, stores creds in ~/.codex/
gemini        # follow Google OAuth login flow, stores creds in ~/.gemini/
vibe --setup  # stores MISTRAL_API_KEY in ~/.vibe/.env

# 4. Run setup (builds container image, verifies auth, smoke tests)
./setup.sh

# 5. Use it
./council.sh "Write a Python function to check palindromes"
```

## Architecture

```
$ council "Fix the auth bug in login.py"
        |
   ┌────┴──────────────────────────────────────────┐
   │            council.sh orchestrator             │
   │                                                │
   │  ┌──────────┬──────────┬──────────┬─────────┐  │
   │  │ parallel │ parallel │ parallel │ parallel │  │
   │  ▼          ▼          ▼          ▼          │  │
   │  container  container  container  container  │  │
   │  claude     codex      gemini     vibe       │  │
   │  │          │          │          │          │  │
   │  ▼          ▼          ▼          ▼          │  │
   │  claude.json codex.json gemini.json vibe.json│  │
   │         │       │       │       │            │  │
   │         └───────┴───────┼───────┘            │  │
   │                         ▼                    │  │
   │                  SYNTHESIZER                 │  │
   │               (Claude Code on host)          │  │
   │                         │                    │  │
   │                         ▼                    │  │
   │                  Final Result                │  │
   └──────────────────────────────────────────────┘
```

One container image, four instances. Each runs a different CLI tool. Workspace mounted read-write (container is the sandbox). Synthesizer runs on the host.

## Authentication

**Default: OAuth (recommended)**

All three agents reuse your existing host CLI logins. No API keys needed.

| Agent | Host auth storage | How it reaches the container |
|-------|-------------------|------------------------------|
| **Claude** | macOS Keychain (`Claude Code-credentials`) | Extracted at runtime via `security find-generic-password`, passed as `CLAUDE_CODE_OAUTH_TOKEN` env var |
| **Codex** | `~/.codex/auth.json` (file-based) | Directory mounted into container at `/root/.codex` |
| **Gemini** | `~/.gemini/settings.json` (file-based) | Directory mounted into container at `/root/.gemini` |
| **Vibe** | `~/.vibe/.env` (file-based, `MISTRAL_API_KEY`) | Directory mounted into container at `/root/.vibe`, key also passed as `MISTRAL_API_KEY` env var |

Claude's OAuth token lives in the macOS Keychain (not files), so it cannot be mounted. The orchestrator extracts it with `security` CLI and passes it as `CLAUDE_CODE_OAUTH_TOKEN` (not `ANTHROPIC_API_KEY` — OAuth tokens are a different format). Falls back to `ANTHROPIC_API_KEY` in `council.env` if Keychain extraction fails.

Claude's `~/.claude/` directory is still mounted for config and hooks (at `/home/council/.claude` since Claude runs as non-root).

**Alternative: API keys**

Set `AUTH_MODE="apikey"` in `config/council.conf`, then:
```bash
cp config/council.env.example config/council.env
# Edit council.env with your keys (ANTHROPIC_API_KEY, OPENAI_API_KEY, GEMINI_API_KEY)
```

## Usage

### Standard Mode (Consensus)

All agents run the same prompt in parallel, then results are synthesized.

```bash
# Full council (all 4 agents + synthesis)
./council.sh "Write a Python function to check palindromes"

# Specific agents only
./council.sh "Fix the auth bug" --agents claude,codex

# Skip synthesis, show raw outputs
./council.sh "Explain how HTTP works" --no-synth

# Custom timeout (seconds)
./council.sh "Refactor this module" --timeout 600

# Mount a specific workspace directory
./council.sh "Review the code" --workspace /path/to/project

# Force rebuild the container image
./council.sh "test" --rebuild

# Verbose output
./council.sh "test" --verbose
```

### Collaborative Mode (Task Decomposition)

A planner decomposes the task, assigns subtasks to agents by strength, and executes with dependency resolution. Three coordination patterns (auto-selected by planner unless forced):

- **Pipeline**: Sequential stages (research → design → implement → test)
- **Parallel**: Independent subtasks run simultaneously, merged at the end
- **Iterative**: Multiple rounds where agents review and improve each other's work

```bash
# Basic: planner decides everything
./council.sh "Build a REST API with auth" --collab

# Force a coordination pattern
./council.sh "Build a REST API" --collab --pattern pipeline

# Iterative with 3 review rounds
./council.sh "Design an algorithm" --collab --pattern iterative --rounds 3

# Dry run: generate task graph, don't execute
./council.sh "Build a REST API" --collab --dry-run

# Pre-built graph (skip planning phase)
./council.sh "test" --collab --graph /path/to/taskgraph.json

# All existing flags still work with collab
./council.sh "Build a REST API" --collab --timeout 600 --workspace /project --verbose
```

## Directory Structure

```
ai-council/
├── council.sh                   # Main orchestrator (bash 3.2+ compatible)
├── council                      # Symlink to council.sh
├── setup.sh                     # First-time setup (requires sudo for /usr/local/bin symlink)
├── Containerfile                # Single image: node 22 + python3 + all 4 CLIs
├── PLAN.md                      # This file
├── .gitignore                   # Ignores council.env, output/, logs/
├── config/
│   ├── council.conf             # Auth mode, timeouts, enabled agents, collab defaults
│   ├── council.env.example      # API key template (only for apikey mode)
│   ├── council.env              # Actual keys (gitignored, only for apikey mode)
│   └── vibe-container.toml      # Vibe config with all tools set to "always" (mounted into container)
├── lib/
│   ├── container-ops.sh         # Build/run/cleanup helpers + auth mounting
│   ├── output-collector.sh      # JSON extraction + validation + extract_result_text
│   ├── synthesizer.sh           # Synthesis invocation (standard + collab modes)
│   ├── collab.sh                # Collaborative mode orchestrator
│   └── graph-exec.sh            # DAG execution engine with JSONL state
├── prompts/
│   ├── synthesizer-system.md    # Standard synthesis system prompt
│   ├── planner-system.md        # Collab planner system prompt (task decomposition)
│   └── collab-synthesizer-system.md  # Collab synthesis system prompt (assembly)
├── output/                      # Per-session results (gitignored)
│   └── <session>/collab/        # Collab mode: task graph, state, per-task outputs
└── logs/                        # Per-session agent stderr logs (gitignored)
```

## How It Works

### Standard Mode

1. **Parse args** - prompt, agent selection, timeout, flags
2. **Build image** - single `council:latest` image with all 4 CLIs (skips if exists)
3. **Launch agents** - N containers in parallel via background processes
4. **Mount auth** - host credential dirs mounted into each container (OAuth) or API keys passed via `-e` (apikey mode)
5. **Mount workspace** - project directory mounted read-write at `/workspace` (container is the sandbox)
6. **Timeout watchdog** - macOS-compatible background process kills containers after deadline (no GNU `timeout`)
7. **Collect outputs** - extract JSON from each agent's stdout (Claude/Gemini/Vibe: single JSON object; Codex: JSONL `agent_message` event)
8. **Preserve partial results** - if agent produced output but exited non-zero (e.g. hook failure), output is kept
9. **Build envelope** - combine all results + statuses into one JSON
10. **Synthesize** - Claude on the host reviews all outputs via stdin pipe, picks/merges the best answer
11. **Display result** - structured JSON with confidence, evaluations, consensus areas

### Collaborative Mode

```
$ council "Build a REST API with auth" --collab
        │
   ┌────┴───────────────────────────────────────┐
   │  PHASE 1: PLAN                              │
   │  Claude on host → task graph JSON            │
   │  (validates: schema + cycle detection)       │
   │  (fallback: standard council if plan fails)  │
   │                                              │
   │  PHASE 2: EXECUTE GRAPH                      │
   │  Wave 1: [task-1, task-2]  ← no deps         │
   │       │        │                             │
   │       ▼        ▼                             │
   │   container  container   (parallel)          │
   │   claude     gemini                          │
   │       │        │                             │
   │  Wave 2: [task-3]  ← depends on 1+2          │
   │       │                                      │
   │       ▼                                      │
   │   container  (gets prior output via file)    │
   │   codex                                      │
   │       │                                      │
   │  Wave 3: [task-4]  ← depends on 3            │
   │       │                                      │
   │       ▼                                      │
   │   container                                  │
   │   vibe                                       │
   │                                              │
   │  PHASE 3: FINAL SYNTHESIS                    │
   │  Claude on host merges all task outputs       │
   └──────────────────────────────────────────────┘
```

**Phase 1: Plan** (`lib/collab.sh` → `run_planner`)
1. Invoke Claude on host with `prompts/planner-system.md` as system prompt
2. Claude produces task graph JSON with task IDs, agent assignments, dependencies, prompts
3. If `--graph` provided, skip planning and load pre-built graph
4. Validate: JSON schema check (required fields, valid agents, unique IDs) + cycle detection (Kahn's algorithm)
5. If validation fails, fall back to standard council mode

**Phase 2: Execute** (`lib/graph-exec.sh` + `lib/collab.sh`)
1. Find ready tasks (pending + all dependencies completed) via `find_ready_tasks()`
2. For each ready task, build augmented prompt: dependency context + task prompt → `collab/<task-id>-prompt.txt`
3. Execute wave: launch ready tasks in parallel via `run_agent()` with file-based prompt
4. Wait for wave, update JSONL state file (`collab/state.jsonl`)
5. Handle failures: retry same agent → reassign to alternative → mark failed
6. Repeat until graph is complete or deadlocked
7. For iterative pattern: generate review rounds where agents get all previous outputs

**Context passing**: Agent outputs → `extract_result_text()` → sanitize → truncate at `COLLAB_MAX_CONTEXT_CHARS` → write augmented prompt to file → mount into container via `COUNCIL_CONTEXT_FILE` env var. This avoids env var size limits (128-256KB).

**State tracking**: Authoritative state in `collab/state.jsonl` (JSONL, one line per state transition). Append-only via `append_state()`, read via `read_task_status()`. Prevents corruption from concurrent parallel writes.

**Phase 3: Synthesize** (`lib/synthesizer.sh` → `run_collab_synthesis`)
1. Build collab envelope JSON (mode, pattern, per-task status + output)
2. Invoke Claude on host with `prompts/collab-synthesizer-system.md`
3. Claude assembles complementary pieces into coherent result (combine, not compare)

### Task Graph JSON Schema

```json
{
  "version": 1,
  "pattern": "pipeline|parallel|iterative",
  "description": "Brief description of the decomposition strategy",
  "tasks": [
    {
      "id": "task-1",
      "description": "Human-readable description",
      "agent": "claude|codex|gemini|vibe",
      "prompt": "Full prompt to send to this agent",
      "depends_on": [],
      "round": 0
    }
  ],
  "final_synthesis_prompt": "Instructions for combining all outputs"
}
```

Rules: `id` must be unique (`task-N`), `depends_on` references valid task IDs, no cycles, `round` is for iterative pattern only (round 0 tasks first, later rounds auto-generated).

## Platform Compatibility Notes

These were discovered during development and are built into the scripts:

- **macOS bash 3.2**: No associative arrays (`declare -A`). All scripts use indexed arrays only.
- **No GNU `timeout`**: macOS doesn't ship it. Uses background process + watchdog `sleep/kill` pattern instead.
- **Apple `container` CLI syntax**: Does NOT use `--` as argument separator (passes it literally). Arguments go directly after the image name.
- **node:22-slim ENTRYPOINT**: Defaults to `["node"]`. Must use `--entrypoint /bin/sh` to run shell commands.
- **Claude root restriction**: `--dangerously-skip-permissions` refuses to run as root. Container runs Claude as a non-root `council` user (created in Containerfile).
- **Claude OAuth via Keychain**: Claude Code stores OAuth tokens in macOS Keychain, not files. Browser-based PKCE login (`claude /login`) fails inside containers. The orchestrator extracts the token at runtime via `security find-generic-password -s "Claude Code-credentials"` and passes it as `CLAUDE_CODE_OAUTH_TOKEN` (not `ANTHROPIC_API_KEY` — OAuth tokens use a different env var). Access tokens expire after ~8 hours. The `ensure_claude_oauth_token()` function in `container-ops.sh` checks `expiresAt` and auto-refreshes using the refresh token via `POST https://platform.claude.com/v1/oauth/token` (client_id: `9d1c250a-e61b-44d9-88ed-5944d1962f5e`, grant_type: `refresh_token`). After refresh, the updated tokens are written back to Keychain.
- **`CLAUDE_CODE_OAUTH_TOKEN` vs `ANTHROPIC_API_KEY`**: OAuth access tokens (`sk-ant-oat01-...`) must be passed via `CLAUDE_CODE_OAUTH_TOKEN`. Passing them as `ANTHROPIC_API_KEY` results in "Invalid API key" errors because the API rejects OAuth tokens sent as API keys.
- **Claude hooks in container**: Host `~/.claude/hooks/` gets mounted too. Containerfile includes `python3` to avoid hook failures from missing interpreters.
- **Non-zero exit with valid output**: Claude may produce valid JSON but exit 1 due to hook failures. The orchestrator preserves output if the file is non-empty rather than overwriting with error JSON.
- **Codex JSONL event stream**: Codex outputs newline-delimited JSON events (`thread.started`, `turn.started`, `item.completed`, `turn.completed`). The actual response text is in `item.completed` events with `item.type == "agent_message"`, NOT in the final `turn.completed` (which only has usage stats). The output collector extracts the last `agent_message` event and merges it with `turn.completed` usage data.
- **Vibe Python 3.12+ requirement**: Mistral Vibe requires Python 3.12+ but `node:22-slim` ships 3.11. Solved by installing `uv` (Astral's Python package manager) which manages its own Python. `uv tool install mistral-vibe` downloads Python 3.12 automatically.
- **Vibe `--auto-approve` not a CLI flag**: Despite documentation suggesting `--auto-approve`, it's actually a `config.toml` setting. The CLI flag `-p` (prompt mode) already implies non-interactive auto-approved execution.
- **Vibe JSON output format**: `--output json` produces a JSON array of all conversation messages (system, user, assistant). The actual response is in the last message with `"role": "assistant"`. The system prompt is included in the array, making output verbose but parseable.
- **Vibe container config override**: Vibe's host `~/.vibe/config.toml` may have tools set to `"ask"` permission (interactive prompts). The orchestrator mounts `config/vibe-container.toml` over `/root/.vibe/config.toml` inside the container, setting all tools (bash, write_file, search_replace, grep, read_file, todo) to `permission = "always"`. This is safe because the container is the sandbox boundary.
- **Prompt passing via env var**: Agent prompts are passed via `COUNCIL_PROMPT` environment variable and referenced as `"$COUNCIL_PROMPT"` in the `-c` command string. This avoids shell quoting issues with apostrophes, backticks, `$`, etc. that broke the previous approach of embedding the prompt in single quotes inside the `-c "..."` string.
- **Apple containers: directory-only mounts**: `--mount source=...,target=...` only supports directories, not individual files. Attempting to mount a single file (e.g. `vibe-container.toml`) results in `"path is not a directory"`. The Vibe auth workaround creates a merged temp directory containing both `~/.vibe/.env` and the container `config.toml`.
- **Codex Landlock sandbox**: Codex uses Linux Landlock LSM internally to sandbox filesystem access. `--full-auto` only sets `--sandbox workspace-write` which still enforces Landlock. To fully disable the sandbox (like the other agents), use `--yolo` (alias for `--sandbox danger-full-access --ask-for-approval never`). Without `--yolo`, Codex fails with `Sandbox(LandlockRestrict)` errors on writes.
- **Synthesis prompt piping**: The synthesizer writes the prompt to a temp file and pipes it via stdin to `claude -p` rather than passing it as a shell argument. This avoids quoting issues when agent outputs contain single quotes, backticks, `$`, or other special shell characters (common in code).
- **`CLAUDECODE` env var**: Claude Code sets `CLAUDECODE=1` to prevent nested sessions. The synthesizer unsets this (`CLAUDECODE= claude -p ...`) so it can invoke Claude on the host even when `council.sh` is run from within a Claude Code session.
- **Collab context file mounting**: In collab mode, augmented prompts are written to `collab/<task-id>-prompt.txt` and mounted into containers at `/council-prompt/`. The `COUNCIL_CONTEXT_FILE` env var tells the agent command to read from this file via `$(cat "$COUNCIL_CONTEXT_FILE")` instead of `$COUNCIL_PROMPT`. This avoids env var size limits (128-256KB) that would break with large multi-round context.
- **Collab JSONL state**: Task state is tracked in `collab/state.jsonl` (append-only, one JSON line per state transition). This is more robust than mutating bash arrays during parallel execution with retries/reassignment.
- **Collab cycle detection**: Uses Kahn's algorithm with temp files for in-degree tracking (bash 3.2 compatible — no associative arrays). If topological sort can't visit all nodes, a cycle exists.
- **Collab output sanitization**: Agent outputs passed as context to downstream tasks are stripped of prompt injection patterns (`<system>`, `Ignore previous instructions`, etc.) via `sanitize_output()` in `lib/collab.sh`.
- **Collab iterative early stop**: Review rounds compute a `shasum` hash of all round outputs. If hash matches previous round, outputs have converged and iteration stops early.

## Design Decisions

- **Read-write workspace**: Agents have full read-write access to the mounted workspace. The container itself is the sandbox — agents can create, modify, and delete files freely inside the container without affecting the host beyond the mounted directory.
- **Full agent permissions**: All agents run with maximum autonomy inside their containers. Claude uses `--dangerously-skip-permissions`, Codex uses `--yolo` (disables Landlock sandbox + approvals), Gemini uses `--yolo`, and Vibe has all tools set to `"always"` via a container-specific `config.toml` (`config/vibe-container.toml`).
- **Partial failure tolerance**: If 1+ agent succeeds, synthesis proceeds. Failed agents are noted in the envelope.
- **Raw JSON to synthesizer**: Rather than normalizing 4 different JSON formats, the synthesizer LLM interprets them directly.
- **OAuth by default**: No API keys to manage. Just log in to each CLI once on the host.
- **Single Containerfile**: One image to build, one to update. Four instances running different commands.
- **Non-root for Claude only**: Codex, Gemini, and Vibe run as root (no restriction). Claude gets `--user council`.

## Reproducing on a New Machine

```bash
# 1. Ensure macOS 26 Tahoe + Apple Silicon
sw_vers -productVersion   # should be 26.x
uname -m                  # should be arm64

# 2. Install container CLI
brew install container
# or: download from https://github.com/apple/container/releases

# 3. Install all CLIs on the host
npm install -g @anthropic-ai/claude-code @openai/codex @google/gemini-cli
pip install mistral-vibe   # or: uv tool install mistral-vibe

# 4. Log in to each CLI
claude    # OAuth -> macOS Keychain (extracted at runtime by orchestrator)
codex     # OAuth -> ~/.codex/auth.json (mounted into container)
gemini    # OAuth -> ~/.gemini/settings.json (mounted into container)
vibe --setup  # stores MISTRAL_API_KEY in ~/.vibe/.env (mounted into container)

# 5. Copy this folder to the machine and run setup
scp -r ai-council/ user@newmachine:~/ai-council/
ssh user@newmachine
cd ~/ai-council
./setup.sh    # builds image, verifies creds, smoke tests, optionally symlinks (needs sudo)

# 6. Verify
./council.sh "What is 2+2?" --agents claude
./council.sh "Explain HTTP" --no-synth
./council.sh "Write a palindrome checker"
```

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `exit code 127` | CLI binary not in PATH inside container | Rebuild with `--rebuild` (Containerfile sets `ENV PATH`) |
| `exit code 127` + `timeout: command not found` | macOS has no GNU `timeout` | Already fixed: uses background watchdog pattern |
| `Cannot find module '/workspace/sh'` | Container ENTRYPOINT is `node`, not `sh` | Already fixed: uses `--entrypoint /bin/sh` |
| `/bin/sh: cannot open -c` | Apple container passes `--` literally | Already fixed: removed `--` separator |
| `--dangerously-skip-permissions cannot be used with root` | Claude refuses root | Already fixed: runs as `--user council` |
| `SessionEnd hook failed: python3 not found` | Host hooks mounted into container, missing interpreter | Already fixed: `python3` in Containerfile |
| `Not logged in · Please run /login` (Claude) | Claude OAuth is in Keychain, not files | Already fixed: extracts token from Keychain via `security` CLI at runtime |
| `OAuth token has expired` (Claude 401) | OAuth access token expired before/during container run | Already fixed: `ensure_claude_oauth_token()` checks `expiresAt` and auto-refreshes via `platform.claude.com/v1/oauth/token` before passing to container. Falls back to API key if refresh fails. If refresh token is also invalid, run `claude /login` on the host. |
| `Invalid API key` (Claude) | OAuth token passed as `ANTHROPIC_API_KEY` instead of `CLAUDE_CODE_OAUTH_TOKEN` | Already fixed: uses `CLAUDE_CODE_OAUTH_TOKEN` for OAuth tokens |
| `Invalid OAuth Request / code_challenge_method` | Tried `claude /login` inside container | Can't do browser OAuth in container. Use Keychain extraction (default) or API key fallback |
| `Please set an Auth method in settings.json` (Gemini) | Gemini CLI not logged in | Run `gemini` on host to complete OAuth, or set `GEMINI_API_KEY` in `council.env` |
| `declare: -A: invalid option` | macOS bash 3.2 doesn't support associative arrays | Already fixed: uses indexed arrays |
| `ln: /usr/local/bin/council: Permission denied` | Symlink to `/usr/local/bin` needs sudo | Already fixed: setup.sh uses `sudo ln` |
| Codex output shows only `turn.completed` (no answer) | Output collector grabbed last JSONL line instead of `agent_message` event | Already fixed: extracts `item.completed` events with `item.type == "agent_message"` |
| `vibe: error: unrecognized arguments: --auto-approve` | `--auto-approve` is a config.toml setting, not a CLI flag | Already fixed: uses `-p` (prompt mode) which implies auto-approve |
| Vibe `pip install` fails with `Requires-Python >=3.12` | `node:22-slim` ships Python 3.11 | Already fixed: uses `uv tool install` which manages its own Python 3.12 |
| `Unterminated quoted string` (all agents) | Prompt contains apostrophes or special chars that break shell quoting | Already fixed: prompt passed via `COUNCIL_PROMPT` env var instead of embedded in `-c` string |
| `path '/path/to/file' is not a directory` (Vibe) | Apple containers only support directory mounts, not single files | Already fixed: creates merged temp directory with `.env` + `config.toml` |
| `Sandbox(LandlockRestrict)` (Codex) | Codex's internal Landlock sandbox blocks filesystem writes with `--full-auto` | Already fixed: uses `--yolo` which disables Landlock sandbox entirely |
| `Synthesis failed` (empty `synthesis.json`) | Prompt passed as shell arg breaks on quotes/backticks in code | Already fixed: writes prompt to temp file and pipes via stdin |
| `Synthesis failed` when run from Claude Code session | `CLAUDECODE=1` env var blocks nested `claude` invocation | Already fixed: synthesizer unsets `CLAUDECODE` before calling `claude -p` |
| `Planner failed. Falling back to standard council mode.` | Claude planner produced invalid JSON or no `.tasks` field | Check verbose output; planner prompt may need refinement for the specific task. Use `--dry-run` to debug |
| `dependency cycle detected` | Task graph has circular dependencies | Fix the graph JSON — use `--graph` with a corrected file |
| `Deadlock detected` | A task's dependency failed, blocking downstream tasks | Retry handling tried alternative agents but all failed. Check agent logs in `collab/` |
| `Validation errors: Task X has invalid agent` | Planner assigned a task to an agent not in the available list | The planner prompt constrains valid agents; if using `--agents` to limit agents, ensure the planner prompt reflects this |
| Collab mode falls back to standard | Planner or graph validation failed | This is by design — graceful degradation. Check `--verbose` output for the specific error |

## Verification Checklist

### Standard Mode
- [ ] `./setup.sh` builds image and passes smoke tests
- [ ] `./council.sh "What is 2+2?" --agents claude` returns synthesized result
- [ ] `./council.sh "Explain HTTP" --no-synth` shows all 4 raw agent outputs
- [ ] `./council.sh "Write a palindrome checker"` runs full pipeline with synthesis
- [ ] `./council.sh "test" --agents claude,codex --timeout 5` handles timeout gracefully
- [ ] After each run, `container list` shows no leftover containers

### Collaborative Mode
- [ ] `./council.sh "Build a calculator CLI" --collab --dry-run` → valid task graph JSON printed
- [ ] `./council.sh "Build a calculator CLI" --collab --pattern pipeline --verbose` → sequential execution with context passing
- [ ] `./council.sh "Build frontend and backend" --collab --pattern parallel` → parallel tasks, merged synthesis
- [ ] `./council.sh "Design a sorting algorithm" --collab --pattern iterative --rounds 2` → multi-round review
- [ ] `./council.sh "test" --collab --graph /path/to/bad-graph.json` → validation catches errors, falls back gracefully
- [ ] Syntax check: `bash -n lib/collab.sh lib/graph-exec.sh` passes
- [ ] Graph engine: cycle detection rejects cyclic graphs, schema validation catches invalid agents/duplicate IDs/unknown deps

## Production Considerations

These recommendations were surfaced by the AI Council itself when asked to describe its own architecture.

### Resources
- **RAM**: ~16GB recommended (4GB per agent container running concurrently)
- **CPU**: 8 cores recommended (Apple Silicon, one container per core pair)
- **Disk**: Container image is ~2GB; each session's output is <1MB

### Cost Management
- **Smart routing**: For simple queries (e.g. "What is 2+2?"), run fewer agents (`--agents claude`) instead of the full council
- **Token budgets**: Codex supports `--max-tokens`, Vibe supports `--max-price` — could be exposed as council-level flags
- **Result caching**: Identical prompts within a session window could skip re-running agents

### Retry Logic
- If an agent fails but others succeed, synthesis proceeds (already implemented)
- Future: retry failed agents once with exponential backoff before giving up
- If synthesis confidence is below a threshold, present all individual responses instead of the merged result

### Observability
- Per-agent logs already captured in `logs/<session>/`
- Token usage and cost tracked in each agent's JSON output
- Future: aggregate cost/latency metrics across sessions for trend analysis

### Security
- Workspace mounted read-write but contained within Apple containers (agents can modify files but are sandboxed)
- Each container gets only its own credentials (no cross-agent key leakage)
- OAuth tokens extracted at runtime, not stored on disk
- `council.env` is gitignored to prevent key commits
- Vibe container config (`vibe-container.toml`) sets all tools to "always" — safe because the container is the sandbox boundary

## Replicating the Collab Mode Implementation

If you need to rebuild the collab mode from scratch (e.g. on a fresh clone without these files), here are the exact steps:

### Step 1: Config defaults
Add to `config/council.conf`:
```bash
COLLAB_MAX_ROUNDS=2
COLLAB_MAX_CONTEXT_CHARS=50000
```

### Step 2: Arg parser (`council.sh`)
Add variables: `COLLAB_MODE=false`, `COLLAB_PATTERN=""`, `COLLAB_ROUNDS=""`, `COLLAB_GRAPH=""`, `COLLAB_DRY_RUN=false`

Add case branches: `--collab`, `--pattern`, `--rounds`, `--graph`, `--dry-run`

Add after `build_image`: if `COLLAB_MODE=true`, source `lib/collab.sh`, call `run_collab "$PROMPT"`. On success, `exit 0`. On failure, fall through to standard mode.

### Step 3: Planner prompt (`prompts/planner-system.md`)
System prompt instructing Claude to decompose tasks. Must include: agent capability matrix (claude=reasoning, codex=code, gemini=research, vibe=prototyping), pattern descriptions (pipeline/parallel/iterative), output JSON schema, guidelines (2-6 tasks, assign by strength, minimize deps).

### Step 4: Collab synthesis prompt (`prompts/collab-synthesizer-system.md`)
System prompt for assembling complementary pieces. Key difference from standard: combine, don't compare. Output format: `summary`, `result`, `plan_executed`, `tasks_completed[]`, `gaps[]`, `confidence`, `warnings[]`.

### Step 5: `extract_result_text()` (`lib/output-collector.sh`)
Function that extracts human-readable text from agent-specific JSON. Claude/Codex: `.result`, Gemini: `.response`, Vibe: last assistant message from array. Fallback: `jq 'tostring'`.

### Step 6: File-based prompts (`lib/container-ops.sh`)
Add 7th optional `prompt_file` parameter to `run_agent()`. When set: mount prompt directory at `/council-prompt`, set `COUNCIL_CONTEXT_FILE` env var, agent commands use `$(cat "$COUNCIL_CONTEXT_FILE")` instead of `$COUNCIL_PROMPT`. When not set: existing inline prompt via `COUNCIL_PROMPT` env var.

### Step 7: Graph execution engine (`lib/graph-exec.sh`)
Core functions: `parse_task_graph` (schema validation + cycle detection → populate arrays), `detect_cycles` (Kahn's algorithm with temp files), `append_state`/`read_task_status` (JSONL state file), `find_ready_tasks` (pending + all deps completed), `execute_wave` (parallel launch + wait), `is_graph_complete`, `has_deadlock`.

### Step 8: Collab orchestrator (`lib/collab.sh`)
Sources `lib/graph-exec.sh`. Core functions: `run_collab` (plan → validate → execute waves → synthesize), `run_planner` (invoke Claude with planner prompt), `build_task_prompt` (augmented prompt with dep context), `generate_review_round` (iterative), `handle_task_failure` (retry → reassign → skip), `build_collab_envelope`, `sanitize_output`.

### Step 9: Collab synthesis (`lib/synthesizer.sh`)
Add `run_collab_synthesis()` — same pattern as `run_synthesis()` but uses collab system prompt, counts `.tasks[]` instead of `.agents{}`, passes `final_synthesis_prompt` from task graph.

### Step 10: Skills (`.claude/commands/`)
Update `council.md` to detect `--collab` flag. Create `council-collab.md` as dedicated shortcut.

## Future: WebSocket SDK Protocol

Claude Code has an undocumented `--sdk-url` flag for full bidirectional WebSocket control (NDJSON protocol). This unlocks real-time streaming, programmatic tool permission handling, multi-turn conversations, and session resume.

Reference: [Companion WebSocket Protocol (reverse-engineered)](https://github.com/The-Vibe-Company/companion/blob/main/WEBSOCKET_PROTOCOL_REVERSED.md)

### Why WebSocket SDK

The current container-based orchestration is **fire-and-forget**: launch agent, wait for JSON output, collect. The WebSocket SDK enables:

- **Real-time streaming**: Token-by-token output to TUI without waiting for completion
- **Programmatic tool permissions**: Server approves/denies tool use requests (Bash, Write, etc.) based on policy
- **Multi-turn conversations**: Send follow-up prompts without restarting the agent
- **Session resume**: Reconnect to an interrupted session and continue
- **Debate Mode**: Agents review each other's work iteratively with the server coordinating turns

### Architecture

```
$ council "Build a REST API" --debate --rounds 3
        │
   ┌────┴─────────────────────────────────────────────┐
   │          sdk-gateway (Bun/Hono)                   │
   │                                                   │
   │  Routes:                                          │
   │    /ws/agent/:agentId   ← Claude CLI connects     │
   │    /ws/tui/:sessionId   ← TUI consumers           │
   │    /api/sessions        ← REST control plane      │
   │                                                   │
   │  ┌──────────┐  ┌──────────┐  ┌───────────────┐   │
   │  │  NDJSON   │  │ Session  │  │  Permission   │   │
   │  │  Codec    │  │  State   │  │  Policy       │   │
   │  └──────────┘  └──────────┘  └───────────────┘   │
   │  ┌──────────┐  ┌──────────┐                      │
   │  │  Debate  │  │  Claude  │                      │
   │  │  Mode    │  │  Launcher│                      │
   │  └──────────┘  └──────────┘                      │
   │                                                   │
   │  Agent connections:                               │
   │  claude --sdk-url ws://localhost:8765/ws/agent/1  │
   │  claude --sdk-url ws://localhost:8765/ws/agent/2  │
   └───────────────────────────────────────────────────┘
```

### Protocol Summary

The CLI is the **WebSocket client**, connecting to your server. Protocol is NDJSON (newline-delimited JSON).

**Connection flow:**
1. Server starts listening
2. Spawn `claude --sdk-url ws://localhost:8765/ws/agent/1 --print --output-format stream-json --input-format stream-json --verbose -p "placeholder"`
3. CLI connects, sends `system/init` (capabilities, tools, model, session_id)
4. Server sends `user` message (the actual prompt)
5. CLI processes and streams back: `stream_event` (tokens), `assistant` (full response), `result` (completion)
6. If tool needs permission: CLI sends `control_request` (can_use_tool) → Server responds with `control_response` (allow/deny)
7. For multi-turn: Server sends another `user` message after `result`

**Key message types:**

| Direction | Type | Purpose |
|-----------|------|---------|
| CLI → Server | `system/init` | Agent capabilities, model, session_id |
| Server → CLI | `user` | Send prompt or follow-up |
| CLI → Server | `assistant` | Full LLM response |
| CLI → Server | `stream_event` | Token-by-token streaming (with `--verbose`) |
| CLI → Server | `result` | Query complete (success/error, usage, cost) |
| CLI → Server | `control_request` | Tool permission request (can_use_tool) |
| Server → CLI | `control_response` | Approve/deny tool use |
| Both | `keep_alive` | Heartbeat (every 10s) |

### Project Structure

```
sdk-gateway/
├── src/
│   ├── index.ts                # Bun/Hono entry point (port 8765)
│   ├── protocol/
│   │   ├── types.ts            # All NDJSON message type interfaces
│   │   └── codec.ts            # NDJSON parse/serialize + validation
│   ├── core/
│   │   ├── gateway-state.ts    # Session state, agent registry, pending request_id map
│   │   ├── message-router.ts   # Route messages between agents, TUI, control plane
│   │   ├── permission-policy.ts # Policy-based tool permission engine + audit log
│   │   ├── debate-mode.ts      # Multi-round debate orchestration + convergence detection
│   │   └── claude-launcher.ts  # Spawn `claude --sdk-url` child processes
│   └── routes/
│       ├── agent-ws.ts         # /ws/agent/:agentId — agent NDJSON connections
│       ├── tui-ws.ts           # /ws/tui/:sessionId — TUI streaming fanout
│       └── control-api.ts      # REST: create session, start debate, list agents
├── test/
│   ├── protocol.test.ts        # NDJSON codec + message type tests
│   └── integration.test.ts     # End-to-end with mock agent
├── package.json
└── tsconfig.json
```

### Implementation Steps

1. **Set up Bun + Hono** project with TypeScript strict mode
2. **Implement NDJSON codec** — parse `\n`-delimited JSON, serialize outbound, validate message types
3. **Build gateway state** — track sessions, agent connections, `request_id` → pending response map with timeouts
4. **Implement agent WS handler** — accept `system/init`, forward `user` messages, relay `assistant`/`result` back
5. **Implement permission router** — intercept `can_use_tool` requests, apply per-tool/per-agent policy, respond with allow/deny + `updatedInput`
6. **Build debate mode** — manage rounds (analyze → review → refine → consensus), inject prior outputs as review prompts, detect convergence via output hash comparison
7. **Add TUI WS fanout** — broadcast `stream_event`/`assistant`/`result` to connected TUI clients
8. **Add Claude launcher** — spawn and manage `claude --sdk-url` child processes with proper cleanup
9. **Wire into council** — new `--debate` flag that uses the gateway instead of containers for Claude (other agents still use containers until they support WebSocket)

### Debate Mode Design

```
Round 1: Each agent works independently
  Agent A → analysis     ← server sends prompt
  Agent B → analysis     ← server sends same prompt

Round 2: Cross-review
  Agent A → review(B's output)  ← server injects B's result into new prompt
  Agent B → review(A's output)  ← server injects A's result into new prompt

Round 3: Refinement
  Agent A → refine(all feedback) ← server combines all round-2 outputs
  Agent B → refine(all feedback)

Consensus Check: Compare outputs — if hash matches or similarity > threshold, stop
```

### Current Limitations

- **Claude Code only** — `--sdk-url` is specific to Claude Code CLI; Codex/Gemini/Vibe don't have equivalent WebSocket modes
- **Undocumented** — the flag is hidden (`--sdk-url .hideHelp()` in Commander) and could change/break in future versions
- **OAuth tokens expire** — the gateway needs to handle token refresh (now implemented in `container-ops.sh` via `ensure_claude_oauth_token()`)
- **No official SDK docs** — protocol reverse-engineered from CLI v2.1.37; verify against installed version

### Council Review (Feb 14, 2026)

Three agents (Codex, Gemini, Vibe) reviewed this design. Consensus:
- All agree on NDJSON-over-WebSocket architecture with central server
- All agree on debate mode pattern (round-based with cross-review)
- Codex provided strongest typed implementation (TypeScript codec + Hono routes)
- Vibe provided best permission engine design (policy-based with audit log)
- Gemini provided clearest protocol documentation (dual-protocol: agent NDJSON + client JSON)

---

## Go Rewrite — Progressive Migration

The bash orchestrator has hit its complexity ceiling. Word-splitting bugs, temp-file-based algorithms, jq pipelines for what should be map lookups, python3 shims for timestamps. The TUI is already in Go (Bubble Tea). The path forward is a single Go binary that is both orchestrator and TUI.

### Strategy: Progressive, Not Big-Bang

```
Phase 1 (current):   bash orchestrator + Go TUI (TUI shells out to council.sh)
Phase 2 (next):      Go binary IS the orchestrator — shells out to `container` CLI
Phase 3 (later):     WebSocket SDK gateway native in Go — debate mode, streaming TUI
```

Keep bash for what it's good at (thin wrapper scripts, credential extraction one-liners). Move everything else to Go.

### What Moves to Go vs. What Stays

| Move to Go | Keep as-is |
|---|---|
| Arg parsing + config loading | `Containerfile` (stays as container build spec) |
| Agent scheduling (parallel, wave-based) | `config/council.conf` (flat key-value, read by Go) |
| Container invocation (`exec.Command`) | `config/vibe-container.toml` (mounted into container) |
| Output collection + JSON extraction | `prompts/*.md` (read as embedded files) |
| Synthesis orchestration | `setup.sh` (one-time setup, can stay bash) |
| Collab mode (planner, graph engine, DAG) | |
| OAuth token management + refresh | |
| Session state + logging | |
| WebSocket SDK gateway | |
| TUI (already Go) | |

### Package Structure

```
ai-council/
├── cmd/
│   └── council/
│       └── main.go              # Single binary: CLI + TUI + orchestrator
│
├── internal/
│   ├── config/
│   │   └── config.go            # Load council.conf + CLI flags + env vars
│   │
│   ├── auth/
│   │   ├── keychain.go          # macOS Keychain read/write (security CLI)
│   │   ├── oauth.go             # OAuth token refresh (platform.claude.com)
│   │   └── provider.go          # Per-agent auth: Claude=Keychain, Codex=file, etc.
│   │
│   ├── agent/
│   │   ├── agent.go             # Agent interface + registry
│   │   ├── container.go         # Container-based execution (shells out to `container run`)
│   │   ├── websocket.go         # WebSocket SDK execution (--sdk-url, Phase 3)
│   │   └── output.go            # Per-agent JSON output parsing (Claude, Codex, Gemini, Vibe)
│   │
│   ├── orchestrator/
│   │   ├── standard.go          # Standard mode: parallel agents → collect → synthesize
│   │   ├── collab.go            # Collab mode: plan → DAG execute → synthesize
│   │   ├── debate.go            # Debate mode: round-based cross-review (Phase 3)
│   │   └── synthesizer.go       # Invoke Claude on host for synthesis
│   │
│   ├── graph/
│   │   ├── graph.go             # Task graph: parse, validate, topo-sort
│   │   ├── executor.go          # Wave-based DAG execution with state tracking
│   │   └── planner.go           # Invoke Claude to decompose task → graph JSON
│   │
│   ├── sdk/                     # Phase 3: WebSocket SDK gateway
│   │   ├── server.go            # WebSocket server (gorilla/websocket or nhooyr)
│   │   ├── ndjson.go            # NDJSON codec: parse/serialize + message types
│   │   ├── session.go           # Session state, agent registry, pending requests
│   │   ├── permission.go        # Policy-based tool permission engine
│   │   └── launcher.go          # Spawn `claude --sdk-url` child processes
│   │
│   ├── tui/                     # Already exists — expand
│   │   ├── model.go             # Bubble Tea model (exists)
│   │   ├── update.go            # Message handling (exists)
│   │   ├── view.go              # Rendering (exists)
│   │   ├── styles.go            # Lipgloss styles (exists)
│   │   └── panels/              # NEW: per-panel components
│   │       ├── agent.go         # Agent output panel with streaming
│   │       ├── overview.go      # Dashboard: all agents at a glance
│   │       ├── synthesis.go     # Synthesis result panel
│   │       ├── debate.go        # Debate mode: round tracker, agent turns
│   │       └── graph.go         # Collab mode: task graph visualization
│   │
│   ├── types/
│   │   └── state.go             # Shared types (exists — extend)
│   │
│   └── msgs/
│       └── messages.go          # Bubble Tea messages (exists — extend)
│
├── prompts/                     # Embedded via go:embed
│   ├── synthesizer-system.md
│   ├── planner-system.md
│   └── collab-synthesizer-system.md
│
├── config/
│   ├── council.conf
│   ├── council.env.example
│   └── vibe-container.toml
│
├── Containerfile
├── setup.sh                     # Stays bash: image build, CLI install, symlink
├── go.mod
└── go.sum
```

### Key Design Decisions

**Single binary, two modes:**
```bash
council "Fix the auth bug"                    # CLI mode: run and print result
council "Fix the auth bug" --tui              # TUI mode: interactive Bubble Tea UI
council "Fix the auth bug" --collab           # Collab mode (CLI or TUI)
council "Fix the auth bug" --debate --rounds 3  # Debate mode (Phase 3)
```

**`agent.Agent` interface:**
```go
type Agent interface {
    ID() AgentID
    Run(ctx context.Context, prompt string, workspace string) (*Result, error)
    Stream() <-chan StreamEvent   // real-time output (for TUI)
}

// Two implementations:
type ContainerAgent struct { ... }   // Phase 2: shells out to `container run`
type WebSocketAgent struct { ... }   // Phase 3: connects via --sdk-url
```

**Container execution via `exec.Command`:**
```go
func (a *ContainerAgent) Run(ctx context.Context, prompt, workspace string) (*Result, error) {
    args := []string{"run", "--rm", "--entrypoint", "/bin/sh"}
    args = append(args, a.authArgs()...)
    args = append(args, "--mount", fmt.Sprintf("source=%s,target=/workspace", workspace))
    args = append(args, a.image, "-c", a.buildCommand(prompt))

    cmd := exec.CommandContext(ctx, "container", args...)
    // ... capture stdout/stderr, parse JSON output
}
```

This is exactly what `container-ops.sh` does, but with proper error handling, no word-splitting risk, and typed output parsing.

**Concurrency model:**
```go
func (o *StandardOrchestrator) Run(ctx context.Context, prompt string) (*Envelope, error) {
    var wg sync.WaitGroup
    results := make(map[AgentID]*Result)
    var mu sync.Mutex

    for _, agent := range o.agents {
        wg.Add(1)
        go func(a Agent) {
            defer wg.Done()
            ctx, cancel := context.WithTimeout(ctx, o.timeout)
            defer cancel()

            result, err := a.Run(ctx, prompt, o.workspace)

            mu.Lock()
            defer mu.Unlock()
            if err != nil {
                results[a.ID()] = &Result{Error: err.Error()}
            } else {
                results[a.ID()] = result
            }
        }(agent)
    }

    wg.Wait()
    return o.buildEnvelope(results), nil
}
```

Compare to the bash version: background processes with `&`, PID tracking, watchdog `sleep/kill`, `wait $pid`, exit code checking. The Go version is clearer, safer, and handles cancellation properly via context.

**Graph engine — proper data structures:**
```go
type TaskGraph struct {
    Tasks map[string]*Task          // was: parallel indexed arrays in bash
    Deps  map[string][]string       // was: TASK_DEPS array with comma-separated strings
    State map[string]TaskStatus     // was: JSONL append-only file parsed with jq
}

func (g *TaskGraph) FindReady() []*Task {
    var ready []*Task
    for id, task := range g.Tasks {
        if g.State[id] != Pending { continue }
        allDepsComplete := true
        for _, dep := range g.Deps[id] {
            if g.State[dep] != Completed { allDepsComplete = false; break }
        }
        if allDepsComplete { ready = append(ready, task) }
    }
    return ready
}

func (g *TaskGraph) DetectCycles() error {
    // Kahn's algorithm — 10 lines with a map, not 50 lines with temp files
    inDegree := make(map[string]int)
    // ...
}
```

**OAuth token management — no python3 dependency:**
```go
func (p *KeychainProvider) EnsureToken() (string, error) {
    creds, err := p.readKeychain()
    if err != nil { return "", err }

    if time.Now().Add(5 * time.Minute).Before(creds.ExpiresAt) {
        return creds.AccessToken, nil  // still valid
    }

    // Refresh
    newToken, err := p.refreshOAuth(creds.RefreshToken)
    if err != nil { return "", fmt.Errorf("OAuth refresh failed: %w (run 'claude /login')", err) }

    p.writeKeychain(newToken)
    return newToken.AccessToken, nil
}
```

**Embedded prompts via `go:embed`:**
```go
//go:embed prompts/synthesizer-system.md
var synthesizerPrompt string

//go:embed prompts/planner-system.md
var plannerPrompt string
```

No more `cat "${SCRIPT_DIR}/prompts/..."` — prompts are compiled into the binary.

### Migration Steps

#### Phase 2a: Standard Mode in Go
1. `internal/config` — load `council.conf`, merge with CLI flags
2. `internal/auth` — Keychain read/write, OAuth refresh, per-agent provider
3. `internal/agent/container.go` — shell out to `container run`, capture output
4. `internal/agent/output.go` — parse per-agent JSON (Claude `.result`, Codex JSONL, Gemini `.response`, Vibe array)
5. `internal/orchestrator/standard.go` — parallel launch, collect, build envelope
6. `internal/orchestrator/synthesizer.go` — invoke Claude on host via `exec.Command`
7. `cmd/council/main.go` — arg parsing, dispatch to orchestrator or TUI
8. **Verification**: `council "What is 2+2?"` produces same output as `council.sh "What is 2+2?"`

#### Phase 2b: Collab Mode in Go
9. `internal/graph/graph.go` — parse task graph JSON, validate, cycle detection
10. `internal/graph/planner.go` — invoke Claude with planner prompt, extract graph
11. `internal/graph/executor.go` — wave-based DAG execution with state tracking
12. `internal/orchestrator/collab.go` — plan → execute → synthesize pipeline
13. **Verification**: `council "Build a calculator" --collab --dry-run` produces valid graph

#### Phase 2c: TUI Integration
14. Remove `orchestrator.Executor` (which shells out to `council.sh`)
15. TUI's `submitPrompt()` calls `orchestrator.Standard.Run()` directly
16. Stream events flow through channels to Bubble Tea model
17. Add `tui/panels/` for agent, overview, synthesis, graph panels
18. **Verification**: `council "test" --tui` shows live agent output without `council.sh`

#### Phase 3: WebSocket SDK + Debate Mode
19. `internal/sdk/` — WebSocket server, NDJSON codec, session state, permissions
20. `internal/agent/websocket.go` — `WebSocketAgent` implementation
21. `internal/orchestrator/debate.go` — round-based cross-review orchestration
22. `tui/panels/debate.go` — debate mode visualization
23. **Verification**: `council "Design X" --debate --rounds 3` with Claude via WebSocket

### Dependencies to Add

```go
// go.mod additions
require (
    github.com/gorilla/websocket v1.5.3     // Phase 3: WebSocket SDK
    github.com/spf13/cobra v1.8.1           // Better CLI than flag (subcommands, help)
    github.com/spf13/viper v1.19.0          // Config file loading (council.conf)
)
```

The existing Bubble Tea / Lipgloss / Bubbles deps stay.

### What Gets Deleted (Eventually)

Once Go standard mode is verified:
- `council.sh` — replaced by `cmd/council/main.go`
- `lib/container-ops.sh` — replaced by `internal/agent/container.go` + `internal/auth/`
- `lib/output-collector.sh` — replaced by `internal/agent/output.go`
- `lib/synthesizer.sh` — replaced by `internal/orchestrator/synthesizer.go`

Once Go collab mode is verified:
- `lib/collab.sh` — replaced by `internal/orchestrator/collab.go`
- `lib/graph-exec.sh` — replaced by `internal/graph/`

Keep forever:
- `setup.sh` (one-time setup)
- `Containerfile`
- `config/` files
- `prompts/` (embedded into binary via `go:embed`)

---

## Remote Deployment (Dedicated Mac + Tailscale)

Run the AI Council on a dedicated Apple Silicon Mac and access it from anywhere via Tailscale.

### Target Architecture

```
                       Tailscale VPN
    ┌──────────────┐                  ┌──────────────────────────┐
    │  Laptop      │                  │  Mac Mini (dedicated)     │
    │  (anywhere)  │◄────────────────►│                          │
    │              │                  │  council --serve :8080    │
    │  Options:    │   MagicDNS       │  ┌──────────────────┐    │
    │  • SSH CLI   │──────────────────│  │ Apple containers  │    │
    │  • HTTP API  │  macmini.tail... │  │ claude  codex     │    │
    │  • WS TUI    │                  │  │ gemini  vibe      │    │
    └──────────────┘                  │  └──────────────────┘    │
                                      │                          │
                                      │  Always-on:              │
                                      │  • Auto-login (GUI)      │
                                      │  • Keychain unlocked     │
                                      │  • Sleep disabled        │
                                      │  • containermanagerd     │
                                      └──────────────────────────┘
```

### Prerequisites

- Apple Silicon Mac (Mini, Studio, or MacBook) running macOS 26 Tahoe
- Tailscale installed on both the dedicated Mac and client machine(s)
- All 4 CLIs installed and authenticated on the dedicated Mac

### Setup Checklist

#### 1. macOS System Settings

```bash
# Disable sleep entirely
sudo pmset -a sleep 0 displaysleep 0 disksleep 0

# Enable Wake on Network Access (System Settings > Energy Saver)
sudo pmset -a womp 1

# Verify
pmset -g
```

Enable **automatic login** in System Settings > Users & Groups > Login Options. This keeps a GUI session active, which is required for:
- macOS Keychain to remain unlocked (Claude OAuth)
- Apple container runtime (`containermanagerd`) to function
- Screen Sharing access for maintenance

Enable **Remote Login** (SSH) in System Settings > General > Sharing.

#### 2. Tailscale

```bash
# Install
brew install tailscale

# Enable and authenticate
sudo tailscale up --ssh  # --ssh enables Tailscale SSH (no key management)

# Verify from client
tailscale status              # see the Mac in your tailnet
tailscale ssh macmini         # SSH without keys
ping macmini.tailnet          # MagicDNS resolution
```

Consider Tailscale ACLs if your tailnet is shared — restrict who can access the Mac.

#### 3. Initial CLI Authentication

Do this once via **Screen Sharing** (VNC) over Tailscale, since all CLIs need a browser:

```bash
# On the dedicated Mac (via Screen Sharing)
claude         # OAuth → macOS Keychain
codex          # OAuth → ~/.codex/auth.json
gemini         # OAuth → ~/.gemini/settings.json
vibe --setup   # API key → ~/.vibe/.env
```

After initial auth, all subsequent access is via SSH/API — no browser needed.

#### 4. Container Image + Council Setup

```bash
# SSH into the Mac
tailscale ssh macmini

# Clone/copy the council
git clone <repo> ~/ai-council && cd ~/ai-council

# Run setup (builds image, verifies auth, symlinks)
./setup.sh

# Smoke test
council "What is 2+2?" --agents claude
```

### Access Patterns

#### Pattern A: SSH + CLI (simplest, works now)

```bash
# From laptop
tailscale ssh macmini "council 'Fix the auth bug in login.py'"

# Interactive TUI over SSH (needs -t for TTY)
tailscale ssh -t macmini "council 'Fix the auth bug' --tui"

# Or start an SSH session and run multiple queries
tailscale ssh macmini
council "Explain HTTP" --no-synth
council "Build a REST API" --collab --dry-run
```

Works today with no code changes. The TUI renders correctly over SSH since Bubble Tea uses standard ANSI escape codes.

#### Pattern B: HTTP API (requires Go rewrite Phase 2)

Add `--serve` mode to the Go binary:

```bash
# On the dedicated Mac (daemonized)
council --serve --port 8080 --bind 0.0.0.0
```

```bash
# From laptop (via Tailscale)
curl http://macmini.tailnet:8080/api/council \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Fix the auth bug", "agents": ["claude", "codex"]}'

# Stream results
curl -N http://macmini.tailnet:8080/api/council/stream \
  -d '{"prompt": "Build a REST API", "mode": "collab"}'
```

**API design sketch:**

```
POST   /api/council              # Submit prompt, get result
POST   /api/council/stream       # Submit prompt, SSE stream
GET    /api/sessions             # List past sessions
GET    /api/sessions/:id         # Get session result
GET    /api/sessions/:id/agents  # Per-agent raw output
DELETE /api/sessions/:id         # Delete session data
GET    /api/health               # Status + agent auth check
```

Tailscale handles authentication — every request comes from a known Tailscale identity. No extra auth layer needed. Optionally check `Tailscale-User-Login` header for per-user access control.

The Go HTTP handler is straightforward:

```go
// cmd/council/main.go
case "serve":
    mux := http.NewServeMux()
    mux.HandleFunc("/api/council", handleCouncil)
    mux.HandleFunc("/api/council/stream", handleCouncilStream)
    mux.HandleFunc("/api/health", handleHealth)
    log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), mux))
```

#### Pattern C: Remote TUI via WebSocket (Phase 3)

The TUI runs on your laptop. The WebSocket SDK gateway runs on the dedicated Mac. Agent streaming flows in real time over Tailscale.

```bash
# On the dedicated Mac
council --serve-ws --port 8765

# On laptop
council --remote ws://macmini.tailnet:8765 --tui
```

The laptop TUI connects to `/ws/tui/:sessionId` on the gateway, receives `stream_event`/`assistant`/`result` messages, and renders them locally. Prompts are sent back over the same WebSocket. This gives you the full TUI experience with zero latency on the rendering side.

### Known Challenges

#### macOS Keychain Over SSH

The login Keychain may be locked in pure SSH sessions (no GUI). The auto-login requirement above mitigates this by ensuring a GUI session always exists with an unlocked Keychain.

If the Keychain is locked (e.g., after a reboot before auto-login completes):

```bash
# Unlock manually over SSH
security unlock-keychain -p "your-login-password" ~/Library/Keychains/login.keychain-db
```

For a fully headless setup without auto-login, consider switching Claude to API key mode:
```bash
# config/council.conf
AUTH_MODE="apikey"

# config/council.env
ANTHROPIC_API_KEY=sk-ant-api03-...
```

This avoids Keychain entirely but means API billing instead of subscription usage.

#### OAuth Credential Rot

| Token | Lifetime | Auto-refresh? | Manual fix |
|-------|----------|---------------|------------|
| Claude access token | ~8 hours | Yes (`ensure_claude_oauth_token()`) | Automatic |
| Claude refresh token | Weeks–months | Rotated on each refresh | `claude /login` via Screen Share |
| Codex OAuth | Varies | File-based, managed by Codex CLI | `codex` via Screen Share |
| Gemini OAuth | Varies | File-based, managed by Gemini CLI | `gemini` via Screen Share |
| Vibe API key | No expiry | N/A | Update `~/.vibe/.env` |

**Mitigation:** Keep `council.env` populated with API keys as a fallback. The orchestrator already falls back to `ANTHROPIC_API_KEY` if Keychain extraction fails. For Codex/Gemini, add similar fallbacks (not yet implemented).

**Monitoring:** The `--serve` HTTP API should include a `/api/health` endpoint that checks each agent's auth status before you discover it's broken mid-council-run:

```json
{
  "status": "degraded",
  "agents": {
    "claude": {"auth": "oauth", "token_expires_in": "7h42m", "status": "ok"},
    "codex": {"auth": "oauth", "credential_file": "~/.codex/auth.json", "status": "ok"},
    "gemini": {"auth": "oauth", "credential_file": "~/.gemini/settings.json", "status": "expired"},
    "vibe": {"auth": "apikey", "status": "ok"}
  }
}
```

#### Apple Container Runtime

The `containermanagerd` daemon must be running. Verify over SSH:

```bash
# Check if container runtime is available
container list 2>&1 | head -1

# If it fails, may need to start it
# (exact mechanism depends on macOS 26 — may require GUI session)
```

If `container` CLI doesn't work over pure SSH, the auto-login approach solves this by maintaining a GUI session.

#### Power and Connectivity

| Risk | Mitigation |
|------|-----------|
| Mac goes to sleep | `pmset -a sleep 0` + Wake on Network |
| Power outage | UPS + auto-power-on after power failure (System Settings > Energy Saver) |
| macOS update reboot | Disable automatic updates, or accept brief downtime |
| Tailscale disconnect | Tailscale auto-reconnects; `tailscaled` runs as a system daemon |
| Network outage | Council queues are local — results persist in `output/` until retrieved |

Enable auto-power-on:
```bash
# Restart after power failure
sudo pmset -a autorestart 1
```

### Daemonizing the Council Server

For Pattern B/C, run the council as a launchd service:

```xml
<!-- ~/Library/LaunchAgents/com.council.server.plist -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.council.server</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/council</string>
        <string>--serve</string>
        <string>--port</string>
        <string>8080</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/council-server.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/council-server.error.log</string>
</dict>
</plist>
```

```bash
# Install and start
launchctl load ~/Library/LaunchAgents/com.council.server.plist

# Check status
launchctl list | grep council
```

This ensures the council server starts on boot and restarts if it crashes.
