# AI Council WebSocket SDK Protocol Design

## Scope

This design defines how AI Council should orchestrate Claude Code over `--sdk-url` with full bidirectional NDJSON over WebSocket, enabling:

- real-time streaming and TUI rendering,
- programmatic permission handling (`can_use_tool`),
- multi-turn conversations and controlled interrupts,
- session resume/fork,
- iterative Debate Mode where agents critique and improve each other.

Protocol basis (reverse-engineered, version-dependent):
- [WEBSOCKET_PROTOCOL_REVERSED.md](https://github.com/The-Vibe-Company/companion/blob/main/WEBSOCKET_PROTOCOL_REVERSED.md)

## Recommended Architecture

1. `sdk-gateway` service (Bun/Hono).
2. `claude` processes launched per agent with `--sdk-url ws://gateway/ws/agent/:agentId?sessionId=:id`.
3. Session manager keeps agent state, pending control requests, and event log.
4. Policy engine handles `can_use_tool` deterministically.
5. Debate coordinator drives review rounds.
6. TUI subscribes to `/ws/tui/:sessionId` for live event fanout.

## Protocol Contract (Gateway Perspective)

Inbound from Claude agent:
- `system/init`
- `assistant`
- `stream_event` (if `--verbose`)
- `result`
- `control_request` (`can_use_tool`, `hook_callback`, etc.)
- `control_response` (for gateway-initiated requests)
- `keep_alive`

Outbound to Claude agent:
- `user`
- `control_request` (`initialize`, `interrupt`, `set_model`, `set_permission_mode`, ...)
- `control_response` (reply to agent `control_request`)
- `keep_alive`

### NDJSON Rules

- Each logical message is one JSON object + trailing `\n`.
- A single WebSocket frame may contain multiple NDJSON lines.
- Parser must support partial lines across frames.

### Control Correlation

- Every outbound `control_request` has generated `request_id`.
- Maintain `pendingRequests[request_id]` with timeout and cleanup on disconnect.
- On `control_response`, resolve/reject corresponding pending promise.

## Debate Mode Design

Round 1:
- Dispatch same root prompt to all participating agents.

Round N (N > 1):
- Wait for `result` from all agents in round N-1.
- Build per-agent critique prompt with peer outputs.
- Dispatch revised prompt to each agent.

Termination:
- `round == maxRounds`, or
- optional convergence rule (hash/similarity threshold), or
- manual interrupt.

Output:
- Persist all round outputs in session state.
- Emit `debate.completed` event with per-round outputs for downstream synthesis.

## Ways Of Working (Operational Model)

1. `Policy-first mode` (safe default)
- Deny unknown tools.
- Allow read-only tools and vetted Bash patterns.
- Store all permission decisions in event log for audit.

2. `Human-in-the-loop mode`
- For non-trivial tools, emit pending permission event to TUI.
- TUI sends approval/deny action back to gateway.
- Gateway responds via `control_response`.

3. `Autopilot mode`
- Auto-approve policy-safe operations.
- Auto-deny dangerous commands.
- Use `updatedPermissions` sparingly and session-scoped first.

4. `Debate mode`
- Minimum 2 agents, recommended 2-4.
- 2-3 rounds typical.
- Optional final synthesis step from best round outputs.

## Concrete Implementation Steps

1. Stand up `sdk-gateway`.
- Use `sdk-gateway/src/index.ts`.
- Validate `/healthz` and `/api/sessions`.

2. Connect Claude over WebSocket.
- Launch via `/api/sessions/:id/agents/:agentId/launch`.
- Verify `agent.ws.connected` then `sdk.system_init` events.

3. Send `initialize` control request.
- Include council system prompt append, hooks, MCP server list.
- Confirm `control_response` success per agent.

4. Enable turn loop.
- Call `/api/sessions/:id/turn`.
- Stream `sdk.assistant_text` and finalize on `sdk.result`.

5. Implement permission policy hardening.
- Start with deny-by-default unknown tool behavior.
- Add workspace path validation for write/edit tools.
- Add command parser for Bash risk scoring.

6. Wire TUI.
- Subscribe to `/ws/tui/:sessionId`.
- Map events to agent panes, token stream, status bar, and prompt input.

7. Enable Debate Mode.
- Call `/api/sessions/:id/debate/start`.
- Persist per-round outputs and trigger synthesis.

8. Add resume support.
- Launch agent with `resumeSessionId` to pass `--resume`.
- Use stored `session_id` values and replay context as needed.

## Code Structure Guidance

Minimal modules:

- `protocol/`
  - `types.ts` (envelopes and guards)
  - `codec.ts` (NDJSON framing)
- `core/`
  - `gateway-state.ts` (session + control correlation)
  - `message-router.ts` (wire dispatch)
  - `permission-policy.ts` (policy decisions)
  - `debate-mode.ts` (iterative review orchestration)
  - `claude-launcher.ts` (process lifecycle)
- `index.ts`
  - WebSocket routes for agents and TUI
  - REST control plane

## AI Council Integration Suggestion

Phased rollout from current architecture:

1. Add `--transport sdk-ws` mode in `council.sh` or Go orchestrator.
2. Keep existing container execution as fallback path.
3. Move collaborative/iterative strategy logic to gateway API calls.
4. Keep synthesizer stage unchanged initially; feed it debate outputs.

## Known Risks

- Protocol is undocumented and may drift across Claude CLI versions.
- `initialize` timing can be sensitive; send before first user turn.
- If `can_use_tool` is not answered, agent execution can stall indefinitely.
- Reconnect/resume logic must be explicit for long-running sessions.

## Versioning Recommendation

- Pin and log tested Claude CLI versions in deployment metadata.
- Add protocol compatibility checks on `system/init.claude_code_version`.
- Gate rollout with canary sessions before broad adoption.
