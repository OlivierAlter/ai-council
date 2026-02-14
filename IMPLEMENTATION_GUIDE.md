# WebSocket SDK Implementation Guide

**Step-by-step guide to implementing the AI Council WebSocket SDK**

This guide provides concrete implementation steps, code examples, and testing procedures for building the WebSocket SDK server that enables Debate Mode and real-time orchestration.

---

## Quick Start

### Prerequisites

```bash
# 1. Install Bun (if not already installed)
curl -fsSL https://bun.sh/install | bash

# 2. Verify Claude Code is installed
which claude

# 3. Navigate to sdk-server directory
cd /workspace/sdk-server

# 4. Install dependencies
bun install
```

### Verify Protocol Access

```bash
# Test that Claude Code supports --sdk-url
claude --help | grep sdk-url

# Expected: Should show the --sdk-url flag (even if undocumented)
```

---

## Implementation Steps

### Step 1: Minimal WebSocket Echo Server (30 min)

**Goal**: Verify Claude Code can connect and send `system/init`

Create `sdk-server/minimal.ts`:

```typescript
// Minimal WebSocket server to verify protocol
const server = Bun.serve({
  port: 8765,
  fetch(req, server) {
    const url = new URL(req.url);

    if (url.pathname === '/ws') {
      const upgraded = server.upgrade(req);
      if (upgraded) return undefined;
    }

    return new Response('WebSocket endpoint: /ws', { status: 400 });
  },

  websocket: {
    open(ws) {
      console.log('[SERVER] Client connected');
    },

    message(ws, message) {
      const lines = message.toString().split('\n').filter(l => l.trim());

      for (const line of lines) {
        try {
          const msg = JSON.parse(line);
          console.log('[RECEIVED]', JSON.stringify(msg, null, 2));

          // Respond to system/init with a user message
          if (msg.type === 'system/init') {
            const response = {
              type: 'user',
              session_id: 'test-session',
              parent_tool_use_id: null,
              message: {
                role: 'user',
                content: 'Say hello and list your available tools',
              },
            };

            ws.send(JSON.stringify(response) + '\n');
            console.log('[SENT]', JSON.stringify(response, null, 2));
          }
        } catch (e) {
          console.error('[ERROR] Failed to parse message:', e);
        }
      }
    },

    close(ws) {
      console.log('[SERVER] Client disconnected');
    },
  },
});

console.log(`WebSocket server listening on ws://localhost:${server.port}/ws`);
```

**Test**:

```bash
# Terminal 1: Start server
bun run minimal.ts

# Terminal 2: Connect Claude Code
claude --sdk-url ws://localhost:8765/ws \
  --print \
  --output-format stream-json \
  --input-format stream-json \
  --verbose
```

**Expected output**:
- Server receives `system/init` with tools list
- Claude receives user message
- Claude responds with `stream_event` tokens
- Claude sends `assistant` message with final response

---

### Step 2: Type-Safe Protocol Implementation (1-2 hours)

Create complete protocol types and handlers.

**File**: `sdk-server/types.ts` (see WEBSOCKET_SDK_DESIGN.md for full types)

**File**: `sdk-server/protocol.ts`

```typescript
import type { CLIMessage, ServerMessage } from './types';

export class NDJSONParser {
  static parse(raw: string): CLIMessage[] {
    const lines = raw.split('\n').filter(l => l.trim());
    return lines.map(line => JSON.parse(line) as CLIMessage);
  }

  static serialize(msg: ServerMessage): string {
    return JSON.stringify(msg) + '\n';
  }

  static serializeBatch(messages: ServerMessage[]): string {
    return messages.map(m => JSON.stringify(m)).join('\n') + '\n';
  }
}

export class ProtocolValidator {
  static isValid(msg: unknown): msg is CLIMessage {
    if (typeof msg !== 'object' || msg === null) return false;
    const obj = msg as Record<string, unknown>;

    // All messages must have a type field
    if (typeof obj.type !== 'string') return false;

    // Validate based on type
    switch (obj.type) {
      case 'system/init':
        return Array.isArray(obj.tools);

      case 'assistant':
      case 'stream_event':
      case 'result':
      case 'tool_progress':
      case 'control_request':
        return typeof obj.session_id === 'string';

      default:
        return false;
    }
  }
}
```

---

### Step 3: Session Manager (2-3 hours)

Create `sdk-server/session-manager.ts`:

```typescript
import type {
  Session,
  AgentConnection,
  AgentName,
  UserMessage,
  CLIMessage,
  ServerMessage,
  WebSocketData,
} from './types';
import { NDJSONParser } from './protocol';

export class SessionManager {
  private sessions = new Map<string, Session>();
  private keepAliveInterval: Timer | null = null;

  constructor() {
    // Start keepalive broadcaster (every 10s)
    this.keepAliveInterval = setInterval(() => {
      this.broadcastKeepAlive();
    }, 10000);
  }

  createSession(prompt: string, agents: AgentName[]): Session {
    const id = this.generateSessionId();

    const session: Session = {
      id,
      createdAt: new Date(),
      prompt,
      agents: new Map(),
      messageHistory: [],
      status: 'initializing',
      metadata: {},
    };

    this.sessions.set(id, session);
    console.log(`[SESSION] Created ${id} with agents:`, agents);

    return session;
  }

  registerAgent(
    sessionId: string,
    agent: AgentName,
    ws: ServerWebSocket<WebSocketData>,
  ): void {
    const session = this.sessions.get(sessionId);
    if (!session) {
      console.error(`[SESSION] Not found: ${sessionId}`);
      return;
    }

    const connection: AgentConnection = {
      agent,
      ws,
      status: 'connected',
      lastMessageAt: new Date(),
      tools: [],
    };

    session.agents.set(agent, connection);
    console.log(`[SESSION] ${sessionId} — registered agent: ${agent}`);
  }

  unregisterAgent(sessionId: string, agent: AgentName): void {
    const session = this.sessions.get(sessionId);
    if (!session) return;

    session.agents.delete(agent);
    console.log(`[SESSION] ${sessionId} — unregistered agent: ${agent}`);

    // Cleanup session if no agents left
    if (session.agents.size === 0) {
      this.sessions.delete(sessionId);
      console.log(`[SESSION] Cleaned up: ${sessionId}`);
    }
  }

  async sendToAgent(
    sessionId: string,
    agent: AgentName,
    message: UserMessage,
  ): Promise<void> {
    const session = this.sessions.get(sessionId);
    const connection = session?.agents.get(agent);

    if (!connection) {
      throw new Error(`Agent ${agent} not connected to session ${sessionId}`);
    }

    const serialized = NDJSONParser.serialize(message);
    connection.ws.send(serialized);
    connection.status = 'processing';
    connection.lastMessageAt = new Date();

    session.messageHistory.push(message);

    console.log(`[SESSION] ${sessionId} → ${agent}:`, message.type);
  }

  async broadcastToAllAgents(
    sessionId: string,
    message: UserMessage,
  ): Promise<void> {
    const session = this.sessions.get(sessionId);
    if (!session) {
      throw new Error(`Session ${sessionId} not found`);
    }

    const serialized = NDJSONParser.serialize(message);

    for (const [agent, connection] of session.agents.entries()) {
      connection.ws.send(serialized);
      connection.status = 'processing';
      connection.lastMessageAt = new Date();

      console.log(`[SESSION] ${sessionId} → ${agent}:`, message.type);
    }

    session.messageHistory.push(message);
  }

  recordMessage(sessionId: string, message: CLIMessage | ServerMessage): void {
    const session = this.sessions.get(sessionId);
    if (session) {
      session.messageHistory.push(message);
    }
  }

  getSession(sessionId: string): Session | undefined {
    return this.sessions.get(sessionId);
  }

  getAllSessions(): Session[] {
    return Array.from(this.sessions.values());
  }

  cleanup(sessionId: string): void {
    const session = this.sessions.get(sessionId);
    if (!session) return;

    // Close all agent connections
    for (const connection of session.agents.values()) {
      connection.ws.close(1000, 'Session ended');
    }

    this.sessions.delete(sessionId);
    console.log(`[SESSION] Cleaned up: ${sessionId}`);
  }

  private broadcastKeepAlive(): void {
    const now = new Date().toISOString();

    for (const session of this.sessions.values()) {
      for (const connection of session.agents.values()) {
        const keepAlive = {
          type: 'keep_alive' as const,
          session_id: session.id,
          timestamp: now,
        };

        connection.ws.send(NDJSONParser.serialize(keepAlive));
      }
    }
  }

  private generateSessionId(): string {
    const timestamp = Date.now().toString(36);
    const random = Math.random().toString(36).substring(2, 9);
    return `session-${timestamp}-${random}`;
  }

  destroy(): void {
    if (this.keepAliveInterval) {
      clearInterval(this.keepAliveInterval);
    }

    // Close all sessions
    for (const sessionId of this.sessions.keys()) {
      this.cleanup(sessionId);
    }
  }
}
```

---

### Step 4: Permission Controller (1-2 hours)

Create `sdk-server/permission-controller.ts`:

```typescript
import type {
  PermissionPolicy,
  Decision,
  ControlRequest,
  PermissionRule,
} from './types';

export class PermissionController {
  private policies = new Map<string, PermissionPolicy>();

  setPolicy(sessionId: string, policy: PermissionPolicy): void {
    this.policies.set(sessionId, policy);
    console.log(`[PERMISSION] Set policy for ${sessionId}:`, policy.type);
  }

  getPolicy(sessionId: string): PermissionPolicy {
    return this.policies.get(sessionId) || { type: 'allow_all' };
  }

  async evaluate(sessionId: string, request: ControlRequest): Promise<Decision> {
    const policy = this.getPolicy(sessionId);

    console.log(
      `[PERMISSION] Evaluating ${request.tool} against ${policy.type}`,
    );

    switch (policy.type) {
      case 'allow_all':
        return { allow: true };

      case 'deny_all':
        return {
          allow: false,
          reason: 'All tools blocked by policy',
        };

      case 'allowlist':
        return {
          allow: policy.tools.includes(request.tool || ''),
          reason: policy.tools.includes(request.tool || '')
            ? undefined
            : `Tool ${request.tool} not in allowlist`,
        };

      case 'denylist':
        return {
          allow: !policy.tools.includes(request.tool || ''),
          reason: policy.tools.includes(request.tool || '')
            ? `Tool ${request.tool} blocked by denylist`
            : undefined,
        };

      case 'custom':
        return await policy.evaluator(request);

      case 'debate_mode':
        // In debate mode, allow all tools but log for review
        console.log(`[DEBATE] Tool used: ${request.tool}`, request.input);
        return { allow: true };

      default:
        throw new Error(`Unknown policy type: ${(policy as any).type}`);
    }
  }

  createSanitizationPolicy(): PermissionPolicy {
    return {
      type: 'custom',
      evaluator: async (req) => {
        // Example: Sanitize file paths in Edit/Write tools
        if (req.tool === 'Edit' || req.tool === 'Write') {
          const input = req.input as { file_path?: string };

          if (input?.file_path) {
            // Block attempts to write outside workspace
            if (input.file_path.includes('..') || input.file_path.startsWith('/etc')) {
              return {
                allow: false,
                reason: 'File path blocked for security',
              };
            }
          }
        }

        return { allow: true };
      },
    };
  }
}
```

---

### Step 5: Message Router (2-3 hours)

Create `sdk-server/message-router.ts`:

```typescript
import { EventEmitter } from 'events';
import type {
  CLIMessage,
  AgentName,
  WebSocketData,
  SystemInitMessage,
  AssistantMessage,
  StreamEvent,
  ControlRequest,
  ResultMessage,
} from './types';
import { SessionManager } from './session-manager';
import { PermissionController } from './permission-controller';
import { NDJSONParser } from './protocol';

export class MessageRouter extends EventEmitter {
  constructor(
    private sessionManager: SessionManager,
    private permissionController: PermissionController,
  ) {
    super();
  }

  async handleMessage(ws: ServerWebSocket<WebSocketData>, rawMessage: string): Promise<void> {
    try {
      const messages = NDJSONParser.parse(rawMessage);

      for (const msg of messages) {
        await this.routeMessage(ws, msg);
      }
    } catch (error) {
      console.error('[ROUTER] Failed to handle message:', error);
    }
  }

  private async routeMessage(
    ws: ServerWebSocket<WebSocketData>,
    msg: CLIMessage,
  ): Promise<void> {
    const { sessionId, agent } = ws.data;

    // Record message in session history
    this.sessionManager.recordMessage(sessionId, msg);

    switch (msg.type) {
      case 'system/init':
        await this.handleInit(sessionId, agent, msg);
        break;

      case 'assistant':
        await this.handleAssistant(sessionId, agent, msg);
        break;

      case 'stream_event':
        await this.handleStreamEvent(sessionId, agent, msg);
        break;

      case 'control_request':
        await this.handleControlRequest(ws, sessionId, agent, msg);
        break;

      case 'result':
        await this.handleResult(sessionId, agent, msg);
        break;

      case 'tool_progress':
        // Just emit event, no response needed
        this.emit('tool:progress', {
          sessionId,
          agent,
          toolUseId: msg.tool_use_id,
          progress: msg.progress,
          message: msg.message,
        });
        break;

      default:
        console.warn(`[ROUTER] Unknown message type: ${(msg as any).type}`);
    }
  }

  private async handleInit(
    sessionId: string,
    agent: AgentName,
    msg: SystemInitMessage,
  ): Promise<void> {
    const session = this.sessionManager.getSession(sessionId);
    const connection = session?.agents.get(agent);

    if (!connection) {
      console.error(`[ROUTER] Agent ${agent} not found in session ${sessionId}`);
      return;
    }

    // Store tools
    connection.tools = msg.tools.map(t => t.name);
    connection.status = 'ready';

    console.log(`[ROUTER] ${agent} ready with ${connection.tools.length} tools`);

    this.emit('agent:ready', {
      sessionId,
      agent,
      tools: connection.tools,
    });

    // If all agents ready, send initial prompt
    if (this.allAgentsReady(session)) {
      console.log(`[ROUTER] All agents ready, sending initial prompt`);

      await this.sessionManager.broadcastToAllAgents(sessionId, {
        type: 'user',
        session_id: sessionId,
        parent_tool_use_id: null,
        message: {
          role: 'user',
          content: session.prompt,
        },
      });

      session.status = 'running';
    }
  }

  private async handleAssistant(
    sessionId: string,
    agent: AgentName,
    msg: AssistantMessage,
  ): Promise<void> {
    const session = this.sessionManager.getSession(sessionId);
    const connection = session?.agents.get(agent);

    if (connection) {
      connection.status = 'idle';
    }

    console.log(`[ROUTER] ${agent} completed response (${msg.usage?.output_tokens || 0} tokens)`);

    this.emit('agent:completed', {
      sessionId,
      agent,
      message: msg.message,
      usage: msg.usage,
    });
  }

  private async handleStreamEvent(
    sessionId: string,
    agent: AgentName,
    msg: StreamEvent,
  ): Promise<void> {
    this.emit('stream:token', {
      sessionId,
      agent,
      delta: msg.delta,
    });
  }

  private async handleControlRequest(
    ws: ServerWebSocket<WebSocketData>,
    sessionId: string,
    agent: AgentName,
    msg: ControlRequest,
  ): Promise<void> {
    if (msg.subtype === 'can_use_tool') {
      const decision = await this.permissionController.evaluate(sessionId, msg);

      // Send response back to CLI
      const response = {
        type: 'control_response' as const,
        subtype: 'can_use_tool' as const,
        session_id: sessionId,
        uuid: msg.uuid,
        behavior: decision.allow ? 'allow' as const : 'deny' as const,
        updatedInput: decision.modifiedArgs || null,
        updatedPermissions: decision.learnedRules || null,
      };

      ws.send(NDJSONParser.serialize(response));

      console.log(
        `[ROUTER] Tool ${msg.tool} ${decision.allow ? 'ALLOWED' : 'DENIED'} for ${agent}`,
      );

      this.emit('tool:requested', {
        sessionId,
        agent,
        tool: msg.tool!,
        input: msg.input || {},
        decision,
      });
    }
  }

  private async handleResult(
    sessionId: string,
    agent: AgentName,
    msg: ResultMessage,
  ): Promise<void> {
    console.log(`[ROUTER] ${agent} result: ${msg.status}`);

    this.emit('agent:result', {
      sessionId,
      agent,
      status: msg.status,
      error: msg.error,
    });
  }

  private allAgentsReady(session: Session): boolean {
    return Array.from(session.agents.values()).every(c => c.status === 'ready');
  }
}
```

---

### Step 6: Main Server (1 hour)

Create `sdk-server/index.ts`:

```typescript
import { Hono } from 'hono';
import { SessionManager } from './session-manager';
import { PermissionController } from './permission-controller';
import { MessageRouter } from './message-router';
import type { AgentName, WebSocketData } from './types';

const app = new Hono();

// Initialize components
const sessionManager = new SessionManager();
const permissionController = new PermissionController();
const messageRouter = new MessageRouter(sessionManager, permissionController);

// Agent ports
const AGENT_PORTS: Record<AgentName, number> = {
  claude: 8765,
  codex: 8766,
  gemini: 8767,
  vibe: 8768,
};

// Create WebSocket servers for each agent
for (const [agent, port] of Object.entries(AGENT_PORTS) as [AgentName, number][]) {
  Bun.serve({
    port,
    fetch(req, server) {
      const url = new URL(req.url);

      if (url.pathname === '/ws') {
        const sessionId = url.searchParams.get('session') || 'default';

        const upgraded = server.upgrade(req, {
          data: { agent, sessionId } as WebSocketData,
        });

        if (upgraded) return undefined;
      }

      return new Response(`Expected WebSocket connection at /ws?session=<id>`, {
        status: 400,
      });
    },

    websocket: {
      open(ws) {
        console.log(`[${ws.data.agent}] Connected to session ${ws.data.sessionId}`);
        sessionManager.registerAgent(ws.data.sessionId, ws.data.agent, ws);
      },

      message(ws, message) {
        messageRouter.handleMessage(ws, message.toString());
      },

      close(ws) {
        console.log(`[${ws.data.agent}] Disconnected`);
        sessionManager.unregisterAgent(ws.data.sessionId, ws.data.agent);
      },
    },
  });

  console.log(`✓ ${agent} WebSocket listening on ws://localhost:${port}/ws`);
}

// HTTP API for session control
app.post('/sessions', async (c) => {
  const body = await c.req.json();
  const { prompt, agents = ['claude'] } = body as { prompt: string; agents: AgentName[] };

  const session = sessionManager.createSession(prompt, agents);

  return c.json({
    sessionId: session.id,
    prompt,
    agents,
    wsUrls: agents.map(agent => ({
      agent,
      url: `ws://localhost:${AGENT_PORTS[agent]}/ws?session=${session.id}`,
    })),
  });
});

app.get('/sessions/:id', (c) => {
  const sessionId = c.req.param('id');
  const session = sessionManager.getSession(sessionId);

  if (!session) {
    return c.json({ error: 'Session not found' }, 404);
  }

  return c.json({
    id: session.id,
    prompt: session.prompt,
    status: session.status,
    agents: Array.from(session.agents.keys()),
    messageCount: session.messageHistory.length,
  });
});

app.get('/sessions', (c) => {
  const sessions = sessionManager.getAllSessions();

  return c.json({
    sessions: sessions.map(s => ({
      id: s.id,
      status: s.status,
      agents: Array.from(s.agents.keys()),
    })),
  });
});

// Start HTTP API server
export default {
  port: 8000,
  fetch: app.fetch,
};

console.log('✓ HTTP API listening on http://localhost:8000');
console.log('');
console.log('Ready! Try:');
console.log('  curl -X POST http://localhost:8000/sessions -H "Content-Type: application/json" -d \'{"prompt":"Hello"}\'');
```

---

## Testing

### Test 1: Single Agent Connection

```bash
# Terminal 1: Start server
cd /workspace/sdk-server
bun run index.ts

# Terminal 2: Create session
curl -X POST http://localhost:8000/sessions \
  -H "Content-Type: application/json" \
  -d '{"prompt":"What is 2+2?","agents":["claude"]}'

# Copy the session ID from response

# Terminal 3: Launch Claude
claude --sdk-url ws://localhost:8765/ws?session=<SESSION_ID> \
  --print \
  --output-format stream-json \
  --input-format stream-json \
  --verbose
```

**Expected**: Claude connects, receives prompt, responds with answer.

### Test 2: Multi-Agent Parallel

```bash
# Create session with all 4 agents
curl -X POST http://localhost:8000/sessions \
  -H "Content-Type: application/json" \
  -d '{"prompt":"Write a factorial function","agents":["claude","codex","gemini","vibe"]}'

# Launch all 4 CLIs in parallel (4 terminals)
claude --sdk-url ws://localhost:8765/ws?session=<ID> --print --output-format stream-json --input-format stream-json --verbose
codex --sdk-url ws://localhost:8766/ws?session=<ID> --print --output-format stream-json --input-format stream-json --verbose
gemini --sdk-url ws://localhost:8767/ws?session=<ID> --print --output-format stream-json --input-format stream-json --verbose
vibe --sdk-url ws://localhost:8768/ws?session=<ID> --print --output-format stream-json --input-format stream-json --verbose
```

**Expected**: All agents receive prompt simultaneously, work in parallel.

### Test 3: Tool Permission Control

Add permission policy before creating session:

```typescript
// In index.ts, before creating session
permissionController.setPolicy(session.id, {
  type: 'denylist',
  tools: ['Write', 'Bash'],
});
```

**Expected**: Agents can read files but cannot write or execute bash commands.

---

## Next: Debate Mode Implementation

See `WEBSOCKET_SDK_DESIGN.md` Section 4 for Debate Engine implementation.

**Quick snippet** for round-robin debate:

```typescript
import { DebateEngine } from './debate-engine';

const debateEngine = new DebateEngine(sessionManager, messageRouter);

// Start round-robin debate
await debateEngine.startDebate(
  sessionId,
  'Optimize this sorting algorithm',
  'round_robin',
  3, // max rounds
);
```

---

## Deployment

### Docker

```bash
# Build
docker build -t council-sdk -f sdk-server/Dockerfile .

# Run
docker run -p 8000:8000 -p 8765-8768:8765-8768 council-sdk
```

### systemd

```bash
# Install service
sudo cp deploy/council-sdk.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable council-sdk
sudo systemctl start council-sdk
```

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| `Connection refused` | Verify server is running, check port numbers |
| `Invalid WebSocket upgrade` | Ensure `--sdk-url` includes `/ws` path |
| `system/init not received` | Check Claude Code version, flag may not be available |
| `Agent never becomes ready` | Check server logs for errors in `handleInit` |
| `Tool permission denied` | Verify permission policy allows the tool |

---

**Status**: Implementation guide complete
**Next steps**: Build minimal server → test with single agent → add multi-agent → implement debate mode
