# AI Council WebSocket SDK Gateway (Bun + Hono)

This module implements a production-ready starting point for controlling Claude Code through the undocumented `--sdk-url` WebSocket transport (NDJSON protocol) and exposing it to AI Council orchestration flows.

Reference protocol: [WEBSOCKET_PROTOCOL_REVERSED.md](https://github.com/The-Vibe-Company/companion/blob/main/WEBSOCKET_PROTOCOL_REVERSED.md)

## What This Gateway Does

- Accepts Claude Code CLI WebSocket agent connections (`/ws/agent/:agentId`).
- Parses NDJSON frames with multiple JSON lines per WebSocket message.
- Routes `control_request` messages, including:
  - `can_use_tool` -> policy decision (`allow`/`deny`) with required `updatedInput`.
  - `hook_callback` -> callback ack.
- Correlates control request/response with request IDs and timeouts.
- Supports multi-turn messaging (`POST /api/sessions/:id/turn`).
- Supports debate loops with iterative peer review (`POST /api/sessions/:id/debate/start`).
- Broadcasts real-time session events to TUI clients (`/ws/tui/:sessionId`).
- Launches Claude agents programmatically with `--sdk-url` (`POST /api/sessions/:id/agents/:agentId/launch`).

## Architecture

```
Claude CLI (--sdk-url) ---> /ws/agent/:agentId      (NDJSON over WS)
                                   |
                                   v
                        protocol/router + permission policy
                                   |
          +------------------------+--------------------------+
          |                        |                          |
          v                        v                          v
    session store            debate coordinator         control API
  (session + agent state)   (round orchestration)     (/api/sessions/*)
          |
          v
   /ws/tui/:sessionId ---> live event fanout for TUI/dashboard
```

## File Layout

- `src/index.ts`: Hono server + REST + WebSocket routes.
- `src/config.ts`: env-driven config.
- `src/protocol/types.ts`: protocol envelopes and guards.
- `src/protocol/codec.ts`: NDJSON framing and encoding.
- `src/core/gateway-state.ts`: session/agent state + control correlation.
- `src/core/message-router.ts`: SDK message router (`system/init`, `result`, controls).
- `src/core/permission-policy.ts`: default tool permission policy.
- `src/core/debate-mode.ts`: iterative agent review rounds.
- `src/core/claude-launcher.ts`: `claude --sdk-url ...` process runner.

## Run

Prereqs:

- Bun runtime installed.
- `claude` CLI installed and authenticated.

```bash
cd sdk-gateway
bun install
bun run dev
```

Default listener: `ws://localhost:8765`

## Key Endpoints

- `POST /api/sessions` -> create session.
- `GET /api/sessions/:sessionId` -> session snapshot.
- `POST /api/sessions/:sessionId/agents/:agentId/launch` -> spawn Claude agent process.
- `POST /api/sessions/:sessionId/initialize` -> send `initialize` control request.
- `POST /api/sessions/:sessionId/turn` -> send user turn to one/many agents.
- `POST /api/sessions/:sessionId/control/interrupt` -> interrupt current turn.
- `POST /api/sessions/:sessionId/debate/start` -> start iterative debate mode.
- `GET /ws/tui/:sessionId` -> stream events for TUI.

## Launch Flow Example

```bash
# 1) Create session
curl -s -X POST http://localhost:8765/api/sessions -H 'content-type: application/json' -d '{}'

# 2) Launch two Claude agents
curl -s -X POST http://localhost:8765/api/sessions/<SESSION>/agents/claude-a/launch -H 'content-type: application/json' -d '{}'
curl -s -X POST http://localhost:8765/api/sessions/<SESSION>/agents/claude-b/launch -H 'content-type: application/json' -d '{}'

# 3) Initialize runtime controls
curl -s -X POST http://localhost:8765/api/sessions/<SESSION>/initialize -H 'content-type: application/json' -d '{"appendSystemPrompt":"You are agent A in AI Council."}'

# 4) Run normal turn
curl -s -X POST http://localhost:8765/api/sessions/<SESSION>/turn -H 'content-type: application/json' -d '{"content":"Review src/auth.ts for race conditions"}'

# 5) Or start debate mode
curl -s -X POST http://localhost:8765/api/sessions/<SESSION>/debate/start -H 'content-type: application/json' -d '{"prompt":"Design retry strategy for distributed locks","rounds":3}'
```

## TUI Integration Pattern

Connect TUI to `ws://localhost:8765/ws/tui/<SESSION>` and render events by `type`:

- `sdk.message`: raw inbound protocol traffic.
- `sdk.assistant_text`: display incremental assistant text.
- `sdk.result`: per-turn completion.
- `sdk.permission_decision`: permission policy outcomes.
- `debate.round_dispatched`, `debate.round_started`, `debate.completed`: debate lifecycle.
- `agent.process.output`: process stderr/stdout diagnostics.

## Security Notes

- Set `SDK_GATEWAY_BEARER_TOKEN` to require `Authorization: Bearer <token>` for agent WS ingress.
- Default permission policy is intentionally conservative for unknown tools.
- Add a project-root path allowlist before enabling `Write/Edit` in production.

## Env Variables

- `PORT` (default `8765`)
- `HOST` (default `0.0.0.0`)
- `PUBLIC_WS_BASE_URL` (default `ws://localhost:<PORT>`)
- `SDK_GATEWAY_BEARER_TOKEN` (optional)
- `CLAUDE_BINARY` (default `claude`)
- `CLAUDE_DEFAULT_MODEL` (optional)
- `CLAUDE_APPEND_SYSTEM_PROMPT` (optional)
- `MAX_CONTROL_WAIT_MS` (default `30000`)
- `MAX_EVENT_BACKLOG` (default `3000`)
- `DEFAULT_DEBATE_ROUNDS` (default `3`)
