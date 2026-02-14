# AI Council WebSocket SDK — Executive Summary

## What This Unlocks

The WebSocket SDK enables **full bidirectional control** over Claude Code agents via the undocumented `--sdk-url` flag, unlocking capabilities impossible with the current PTY-based orchestration:

| Capability | Impact |
|------------|--------|
| **Debate Mode** | Agents review each other's work iteratively (sequential, round-robin, tournament patterns) |
| **Real-time Streaming** | Token-by-token output for responsive TUI |
| **Programmatic Permissions** | Fine-grained tool approval (e.g., allow Read but deny Write) |
| **Multi-turn Conversations** | Session-persistent agents, no process restarts |
| **Dynamic Routing** | Send different prompts to different agents mid-session |

---

## Architecture Overview

```
WebSocket SDK Server (Bun/Hono)
├── Session Manager       — Track active sessions, agent connections
├── Permission Controller — Evaluate tool use requests, apply policies
├── Message Router        — NDJSON protocol handler (bidirectional)
└── Debate Engine         — Orchestrate peer review patterns

                    ↕ WebSocket (NDJSON)

Claude Code Agents (4x parallel)
--sdk-url ws://localhost:8765/claude
--verbose (streaming mode)
```

**Key insight**: Keep existing Bash orchestrator for container/auth management. SDK server is an **additive layer** for new patterns, not a rewrite.

---

## Debate Mode Patterns

### 1. Sequential Review Pipeline

```
Claude → proposes solution
Codex → reviews Claude's work, suggests improvements
Gemini → reviews Codex's feedback, refines further
Vibe → synthesizes all perspectives into final answer
```

**Use case**: Code review workflows

### 2. Round-Robin Peer Review

```
Round 0: All agents solve in parallel
Round 1: Each agent reviews next agent's work
Round 2: Agents refine based on feedback
Final: One agent merges all refined solutions
```

**Use case**: Algorithm optimization, consensus building

**Convergence detection**: Hash all outputs per round, stop if identical to previous round

### 3. Tournament Bracket

```
Round 1: Claude vs Codex → winner
         Gemini vs Vibe → winner
Round 2: Winners compete → final winner
```

**Use case**: Best solution selection

**Judging**: LLM-based (send both to Claude Opus), metric-based (test pass rate), or user choice

---

## Protocol Flow

### Connection Handshake

1. Server starts 4 WebSocket listeners (one per agent)
2. CLI connects: `claude --sdk-url ws://localhost:8765/ws?session=<id>`
3. CLI sends `system/init` with tool list
4. Server sends `user` message with prompt
5. CLI responds with `stream_event` tokens (real-time)
6. CLI sends `assistant` message with final response

### Tool Permission

1. CLI encounters tool use (e.g., `Edit` file)
2. CLI sends `control_request.can_use_tool` and **blocks**
3. Server evaluates against policy (allow_all, denylist, custom)
4. Server sends `control_response` (allow/deny)
5. CLI proceeds or aborts based on response

**Policies**:
- `allow_all`: Auto-approve everything (default)
- `denylist`: Block specific tools (e.g., `['Write', 'Bash']`)
- `custom`: Arbitrary logic (e.g., sanitize file paths, block `/etc` writes)
- `debate_mode`: Log all tools but allow (for audit trail)

---

## Implementation Roadmap

| Phase | Time | Deliverable |
|-------|------|-------------|
| **1. Core SDK** | 1-2 weeks | Single agent connects, completes task |
| **2. Multi-Agent** | 1 week | Parallel execution with 4 agents |
| **3. Permissions** | 1 week | Tool approval policies working |
| **4. Debate Mode** | 2 weeks | Sequential + round-robin patterns |
| **5. Production** | 2 weeks | Auth, metrics, deployment |

**Total**: 6-8 weeks to production-ready system

---

## Quick Start (5 minutes)

```bash
# 1. Install dependencies
cd /workspace/sdk-server
bun install

# 2. Start server
bun run index.ts

# 3. Create session
curl -X POST http://localhost:8000/sessions \
  -H "Content-Type: application/json" \
  -d '{"prompt":"What is 2+2?","agents":["claude"]}'

# 4. Launch Claude (replace <SESSION_ID>)
claude --sdk-url ws://localhost:8765/ws?session=<SESSION_ID> \
  --print \
  --output-format stream-json \
  --input-format stream-json \
  --verbose
```

---

## Key Files

| File | Purpose |
|------|---------|
| `WEBSOCKET_SDK_DESIGN.md` | Complete architecture, all components, production considerations |
| `IMPLEMENTATION_GUIDE.md` | Step-by-step build instructions with code examples |
| `sdk-server/types.ts` | TypeScript protocol types (NDJSON messages) |
| `sdk-server/index.ts` | Main server entry point |
| `sdk-server/session-manager.ts` | Session lifecycle, agent registration |
| `sdk-server/permission-controller.ts` | Tool approval policies |
| `sdk-server/message-router.ts` | NDJSON message routing |
| `sdk-server/debate-engine.ts` | Debate orchestration (to be implemented) |

---

## Integration with Existing Council

### Option 1: New Flag (Recommended)

```bash
# Add --debate flag to council.sh
./council.sh "Fix auth bug" --debate --pattern round_robin --rounds 3
```

Orchestration:
1. `council.sh` detects `--debate`
2. Starts SDK server in background
3. Launches agents with `--sdk-url` instead of PTY
4. SDK server runs debate pattern
5. Final result written to `output/<session>/debate-synthesis.json`

### Option 2: Separate Binary

```bash
# New command: council-debate
./council-debate "Fix auth bug" --pattern round_robin --rounds 3
```

Cleaner separation, but duplicates some orchestration logic.

---

## Why This Approach

### Why WebSocket over PTY?

| PTY (current) | WebSocket SDK |
|---------------|---------------|
| One-way control (can't interrupt) | Bidirectional (can send new prompts mid-stream) |
| Buffer until completion | Token-by-token streaming |
| All-or-nothing permissions | Per-tool approval |
| Process per turn | Session-persistent |

### Why Bun + Hono?

- **Native WebSocket**: No `ws` library needed
- **Fast**: <100ms cold boot vs Node's 500ms+
- **TypeScript-first**: No build step
- **Process spawning**: `Bun.spawn()` better than `child_process`

### Why Keep Bash Orchestrator?

- **Proven auth**: Keychain extraction, OAuth mounting
- **Container lifecycle**: Apple container quirks solved
- **Risk reduction**: SDK is additive, not a rewrite

---

## Debate Mode Use Cases

### 1. Code Review Pipeline

```bash
./council.sh "Review and improve this auth module" --debate --pattern sequential
```

**Flow**:
1. Claude proposes refactor
2. Codex reviews code quality, security
3. Gemini checks performance implications
4. Vibe synthesizes final recommendations

### 2. Algorithm Optimization

```bash
./council.sh "Optimize this sorting function" --debate --pattern round_robin --rounds 3
```

**Flow**:
- All agents propose optimizations
- Agents peer-review each other's approaches
- Iterate until convergence
- Best solution selected

### 3. Consensus Building

```bash
./council.sh "Should we use Redis or Memcached?" --debate --pattern tournament
```

**Flow**:
- Pairs of agents argue for each option
- Winners advance
- Final agent makes decision based on all arguments

---

## Production Considerations

### Authentication

```typescript
// Generate session tokens with JWT
const token = jwt.sign({ sessionId, agents }, process.env.JWT_SECRET);

// CLI connects with token
claude --sdk-url "ws://localhost:8765/ws?token=${token}"
```

### Rate Limiting

```typescript
// Limit requests per session
const rateLimiter = new RateLimiter(100, 60000); // 100 req/min

if (!rateLimiter.isAllowed(sessionId)) {
  return new Response('Rate limit exceeded', { status: 429 });
}
```

### Metrics

```typescript
// Prometheus metrics
import { Counter, Histogram } from 'prom-client';

const messagesRouted = new Counter({
  name: 'council_messages_total',
  labelNames: ['type', 'agent'],
});

// Expose at /metrics
app.get('/metrics', async (c) => c.text(await register.metrics()));
```

### Deployment

```bash
# Docker
docker build -t council-sdk .
docker run -p 8000:8000 -p 8765-8768:8765-8768 council-sdk

# systemd
sudo systemctl enable council-sdk
sudo systemctl start council-sdk

# Nginx reverse proxy (WebSocket support)
location /ws/ {
  proxy_pass http://localhost:8765;
  proxy_http_version 1.1;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection "upgrade";
}
```

---

## Testing Strategy

### Unit Tests (Bun Test)

```typescript
import { describe, test, expect } from 'bun:test';

describe('SessionManager', () => {
  test('creates session with unique ID', () => {
    const manager = new SessionManager();
    const s1 = manager.createSession('test', ['claude']);
    const s2 = manager.createSession('test', ['claude']);
    expect(s1.id).not.toBe(s2.id);
  });
});
```

### Integration Tests

```typescript
// Full lifecycle test
test('complete session flow', async () => {
  const res = await fetch('http://localhost:8000/sessions', {
    method: 'POST',
    body: JSON.stringify({ prompt: 'test', agents: ['claude'] }),
  });

  const { sessionId } = await res.json();
  const ws = new WebSocket(`ws://localhost:8765/ws?session=${sessionId}`);

  // Verify system/init → user message → assistant response
});
```

### E2E Tests

```bash
# Bash script
./tests/e2e/debate-mode.sh

# Verifies:
# 1. Server starts
# 2. Debate completes
# 3. Output file exists
# 4. All agents participated
```

---

## Next Steps

1. **Validate protocol**: Run `claude --sdk-url` manually, capture messages
2. **Build minimal server**: Implement Phase 1 (basic session + routing)
3. **Test single agent**: Full conversation lifecycle
4. **Add multi-agent**: Parallel execution
5. **Implement Debate**: Start with sequential pattern
6. **Integrate TUI**: Stream events to Bubbletea
7. **Production**: Auth, metrics, deployment

---

## Questions?

**Q**: Will this break existing council.sh?
**A**: No. WebSocket SDK is opt-in via `--debate` flag. Standard mode unchanged.

**Q**: Can I use this without containers?
**A**: Yes. CLI runs on host or in container. Server doesn't care.

**Q**: What if Claude Code doesn't support `--sdk-url`?
**A**: Fall back to PTY mode gracefully. Flag is undocumented but stable.

**Q**: How do I judge "winner" in tournament mode?
**A**: Three options: (1) LLM judge (send both to Claude Opus), (2) Metrics (test coverage, exec time), (3) User choice

**Q**: Can agents collaborate on file edits?
**A**: Yes! Each agent has its own session, but they share the workspace. Agent A writes file, Agent B reads and reviews.

---

## Resources

- **Protocol Spec**: [Companion WebSocket Protocol](https://github.com/The-Vibe-Company/companion/blob/main/WEBSOCKET_PROTOCOL_REVERSED.md)
- **Bun Docs**: https://bun.sh/docs/api/websockets
- **Hono Docs**: https://hono.dev/
- **Design Doc**: `/workspace/WEBSOCKET_SDK_DESIGN.md`
- **Implementation Guide**: `/workspace/IMPLEMENTATION_GUIDE.md`

---

**Status**: Design and implementation specs complete
**Estimated Effort**: 6-8 weeks to production
**Risk Level**: Low (additive, graceful fallback)
**Value**: Unlocks entirely new orchestration patterns
