# AI Council WebSocket SDK — Architecture & Implementation Plan

> **Full bidirectional control over Claude Code via undocumented `--sdk-url` flag**
>
> Reference: [Companion WebSocket Protocol](https://github.com/The-Vibe-Company/companion/blob/main/WEBSOCKET_PROTOCOL_REVERSED.md)

This document describes the architecture for a **WebSocket SDK server** that orchestrates multiple Claude Code agents via the NDJSON protocol, enabling **Debate Mode** (iterative peer review), real-time streaming, programmatic tool permissions, and multi-turn conversations.

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Protocol Flow](#protocol-flow)
4. [Core Components](#core-components)
5. [Debate Mode Design](#debate-mode-design)
6. [Implementation Roadmap](#implementation-roadmap)
7. [Code Structure](#code-structure)
8. [Example Implementations](#example-implementations)
9. [Testing Strategy](#testing-strategy)
10. [Production Considerations](#production-considerations)

---

## Overview

### What This Unlocks

| Feature | Current (PTY) | WebSocket SDK |
|---------|--------------|---------------|
| **Streaming** | Buffer until completion | Token-by-token real-time |
| **Tool Control** | All-or-nothing permissions | Programmatic approval per tool |
| **Multi-turn** | Restart process each time | Session-persistent conversations |
| **Resume** | Limited | Full state recovery with `--resume` |
| **Orchestration** | Shell scripts + container isolation | Bidirectional control, dynamic routing |
| **Debate Mode** | Not possible | Agents review each other iteratively |

### Architecture Philosophy

**Bash orchestrator + WebSocket layer** (NOT a full rewrite)

- Keep existing container isolation, auth, and timeout logic in `council.sh`
- Add a **WebSocket server** (`sdk-server`) that launches Claude Code instances with `--sdk-url ws://localhost:<port>/<agent>`
- Server acts as **message router** and **permission manager**
- Enables new patterns (Debate Mode, streaming TUI) without breaking the battle-tested container orchestration

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                         User / TUI Client                         │
└────────────────────────────┬─────────────────────────────────────┘
                             │ HTTP/WS (control API)
┌────────────────────────────▼─────────────────────────────────────┐
│                    SDK WebSocket Server (Bun/Hono)                │
│  ┌──────────────┬──────────────┬──────────────┬─────────────┐    │
│  │  Session     │  Permission  │  Message     │  Debate     │    │
│  │  Manager     │  Controller  │  Router      │  Engine     │    │
│  └──────┬───────┴──────┬───────┴──────┬───────┴──────┬──────┘    │
│         │              │              │              │           │
│  ┌──────▼──────────────▼──────────────▼──────────────▼──────┐    │
│  │          NDJSON Protocol Handler (Bidirectional)          │    │
│  └──────┬────────────────────────────────────────────┬───────┘    │
│         │ ws://localhost:8765/claude                 │            │
│         │ ws://localhost:8766/codex                  │            │
│         │ ws://localhost:8767/gemini                 │            │
│         │ ws://localhost:8768/vibe                   │            │
└─────────┼────────────────────────────────────────────┼────────────┘
          │                                            │
┌─────────▼────────────┐                   ┌───────────▼────────────┐
│  Claude Code         │                   │  Claude Code           │
│  (container/host)    │                   │  (container/host)      │
│  --sdk-url           │                   │  --sdk-url             │
│  --verbose           │                   │  --verbose             │
│  --print             │        ...        │  --print               │
│  --output-format     │                   │  --output-format       │
│    stream-json       │                   │    stream-json         │
└──────────────────────┘                   └────────────────────────┘
```

### Key Design Decisions

1. **Bun + Hono**: Fast WebSocket support, TypeScript, minimal dependencies
2. **One WS port per agent**: Simplifies routing, allows per-agent auth tokens
3. **Session-first**: Each user request creates a `Session` that owns N agent connections
4. **Streaming by default**: `--verbose` flag always on, clients get `stream_event` messages
5. **Permission middleware**: Intercept `control_request.can_use_tool`, apply policy, return approval
6. **Debate Mode as orchestration pattern**: Not a separate server, but a coordination layer on top of sessions

---

## Protocol Flow

### 1. Connection Handshake

**Server starts N WebSocket listeners** (one per agent):

```typescript
// Pseudo-code
const agentPorts = {
  claude: 8765,
  codex: 8766,
  gemini: 8767,
  vibe: 8768,
};

for (const [agent, port] of Object.entries(agentPorts)) {
  Bun.serve({
    port,
    websocket: {
      open(ws) {
        // CLI connects with Authorization header
        const token = ws.data.headers.get('Authorization');
        if (!validateToken(token)) {
          ws.close(1008, 'Unauthorized');
          return;
        }

        // Attach agent identity to connection
        ws.data.agent = agent;
        ws.data.sessionId = extractSessionId(token);

        // Register with session manager
        sessionManager.registerAgent(ws.data.sessionId, agent, ws);
      },

      message(ws, message) {
        handleNDJSONMessage(ws, message);
      },

      close(ws) {
        sessionManager.unregisterAgent(ws.data.sessionId, ws.data.agent);
      },
    },
  });
}
```

**CLI launched with**:

```bash
claude --sdk-url ws://localhost:8765 \
  --print \
  --output-format stream-json \
  --input-format stream-json \
  --verbose
```

**Initial exchange**:

```
CLI → Server: {"type":"system/init","tools":[...],"session_id":""}
Server → CLI: {"type":"user","message":{"role":"user","content":"Fix auth bug"},"parent_tool_use_id":null,"session_id":"abc123"}
CLI → Server: {"type":"stream_event","delta":{"type":"text","text":"I'll"},...}
CLI → Server: {"type":"stream_event","delta":{"type":"text","text":" analyze"},...}
CLI → Server: {"type":"assistant","message":{...},"usage":{...}}
CLI → Server: {"type":"result","status":"success"}
```

### 2. Tool Permission Flow

**CLI encounters a tool use** (e.g., `Edit` tool):

```
CLI → Server: {
  "type": "control_request",
  "subtype": "can_use_tool",
  "uuid": "req-123",
  "tool": "Edit",
  "input": {"file_path": "/workspace/auth.py", "old_string": "...", "new_string": "..."}
}
```

**Server applies permission policy**:

```typescript
async function handleCanUseTool(ws: WebSocket, request: CanUseToolRequest) {
  const policy = permissionController.getPolicy(ws.data.sessionId);

  // Example policies:
  // - "allow_all": auto-approve everything
  // - "deny_file_writes": block Edit/Write tools
  // - "debate_mode": route to review queue for peer approval

  const decision = await policy.evaluate(request);

  ws.send(JSON.stringify({
    type: "control_response",
    subtype: "can_use_tool",
    uuid: request.uuid,
    behavior: decision.allow ? "allow" : "deny",
    updatedInput: decision.modifiedArgs || null,
    updatedPermissions: decision.learnedRules || null,
  }));
}
```

**Server → CLI**: `{"type":"control_response","subtype":"can_use_tool","uuid":"req-123","behavior":"allow"}`

**CLI proceeds** with tool execution, returns `tool_progress` heartbeats, then continues conversation.

### 3. Multi-Turn Conversation

**User sends follow-up** via control API:

```
HTTP POST /sessions/abc123/messages
{
  "agent": "claude",
  "content": "Now add unit tests"
}
```

**Server forwards** to Claude's WebSocket:

```
Server → CLI: {"type":"user","message":{"role":"user","content":"Now add unit tests"},"session_id":"abc123"}
```

**Claude responds** with streaming tokens → server relays to TUI/client.

---

## Core Components

### 1. Session Manager

Tracks active sessions, agent connections, and conversation state.

```typescript
// lib/sdk-server/session-manager.ts

interface Session {
  id: string;
  createdAt: Date;
  prompt: string;
  agents: Map<AgentName, AgentConnection>;
  messageHistory: Message[];
  status: 'initializing' | 'running' | 'completed' | 'failed';
  metadata: Record<string, unknown>;
}

interface AgentConnection {
  agent: AgentName;
  ws: WebSocket;
  status: 'connected' | 'ready' | 'processing' | 'idle' | 'failed';
  lastMessageAt: Date;
  tools: string[]; // from system/init
}

class SessionManager {
  private sessions = new Map<string, Session>();

  createSession(prompt: string, agents: AgentName[]): Session {
    const id = generateSessionId();
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
    return session;
  }

  registerAgent(sessionId: string, agent: AgentName, ws: WebSocket) {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);

    session.agents.set(agent, {
      agent,
      ws,
      status: 'connected',
      lastMessageAt: new Date(),
      tools: [],
    });
  }

  async sendToAgent(sessionId: string, agent: AgentName, message: UserMessage) {
    const session = this.sessions.get(sessionId);
    const connection = session?.agents.get(agent);
    if (!connection) throw new Error(`Agent ${agent} not connected`);

    connection.ws.send(JSON.stringify(message));
    connection.status = 'processing';
    session.messageHistory.push(message);
  }

  async broadcastToAllAgents(sessionId: string, message: UserMessage) {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);

    for (const [agent, connection] of session.agents.entries()) {
      connection.ws.send(JSON.stringify(message));
      connection.status = 'processing';
    }
    session.messageHistory.push(message);
  }

  getSession(sessionId: string): Session | undefined {
    return this.sessions.get(sessionId);
  }

  async cleanupSession(sessionId: string) {
    const session = this.sessions.get(sessionId);
    if (!session) return;

    // Close all agent connections
    for (const connection of session.agents.values()) {
      connection.ws.close(1000, 'Session ended');
    }

    this.sessions.delete(sessionId);
  }
}
```

### 2. Permission Controller

Evaluates tool use requests against policies.

```typescript
// lib/sdk-server/permission-controller.ts

type PermissionPolicy =
  | { type: 'allow_all' }
  | { type: 'deny_all' }
  | { type: 'allowlist', tools: string[] }
  | { type: 'denylist', tools: string[] }
  | { type: 'custom', evaluator: (req: CanUseToolRequest) => Promise<Decision> }
  | { type: 'debate_mode', reviewQueue: ReviewQueue };

interface Decision {
  allow: boolean;
  reason?: string;
  modifiedArgs?: Record<string, unknown>; // e.g., sanitize file paths
  learnedRules?: PermissionRule[]; // future auto-approvals
}

class PermissionController {
  private policies = new Map<string, PermissionPolicy>();

  setPolicy(sessionId: string, policy: PermissionPolicy) {
    this.policies.set(sessionId, policy);
  }

  getPolicy(sessionId: string): PermissionPolicy {
    return this.policies.get(sessionId) || { type: 'allow_all' };
  }

  async evaluate(sessionId: string, request: CanUseToolRequest): Promise<Decision> {
    const policy = this.getPolicy(sessionId);

    switch (policy.type) {
      case 'allow_all':
        return { allow: true };

      case 'deny_all':
        return { allow: false, reason: 'All tools blocked by policy' };

      case 'allowlist':
        return {
          allow: policy.tools.includes(request.tool),
          reason: policy.tools.includes(request.tool)
            ? undefined
            : `Tool ${request.tool} not in allowlist`,
        };

      case 'denylist':
        return {
          allow: !policy.tools.includes(request.tool),
          reason: policy.tools.includes(request.tool)
            ? `Tool ${request.tool} blocked by denylist`
            : undefined,
        };

      case 'custom':
        return await policy.evaluator(request);

      case 'debate_mode':
        // Add to review queue, return pending (handled separately)
        return await policy.reviewQueue.enqueue(sessionId, request);

      default:
        throw new Error(`Unknown policy type: ${(policy as any).type}`);
    }
  }
}
```

### 3. Message Router

Routes messages between server components and WebSocket connections.

```typescript
// lib/sdk-server/message-router.ts

class MessageRouter {
  constructor(
    private sessionManager: SessionManager,
    private permissionController: PermissionController,
    private eventEmitter: EventEmitter,
  ) {}

  async handleMessage(ws: WebSocket, rawMessage: string) {
    const lines = rawMessage.split('\n').filter(l => l.trim());

    for (const line of lines) {
      const msg = JSON.parse(line);
      await this.routeMessage(ws, msg);
    }
  }

  private async routeMessage(ws: WebSocket, msg: NDJSONMessage) {
    const { sessionId, agent } = ws.data;

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
        await this.handleToolProgress(sessionId, agent, msg);
        break;

      default:
        console.warn(`Unknown message type: ${msg.type}`);
    }
  }

  private async handleInit(sessionId: string, agent: AgentName, msg: SystemInitMessage) {
    const session = this.sessionManager.getSession(sessionId);
    const connection = session?.agents.get(agent);
    if (!connection) return;

    connection.tools = msg.tools.map(t => t.name);
    connection.status = 'ready';

    this.eventEmitter.emit('agent:ready', { sessionId, agent, tools: connection.tools });

    // If all agents ready, send initial prompt
    if (this.allAgentsReady(session)) {
      await this.sessionManager.broadcastToAllAgents(sessionId, {
        type: 'user',
        message: { role: 'user', content: session.prompt },
        parent_tool_use_id: null,
        session_id: sessionId,
      });
    }
  }

  private async handleControlRequest(
    ws: WebSocket,
    sessionId: string,
    agent: AgentName,
    msg: ControlRequest,
  ) {
    if (msg.subtype === 'can_use_tool') {
      const decision = await this.permissionController.evaluate(sessionId, msg);

      ws.send(JSON.stringify({
        type: 'control_response',
        subtype: 'can_use_tool',
        uuid: msg.uuid,
        behavior: decision.allow ? 'allow' : 'deny',
        updatedInput: decision.modifiedArgs || null,
        updatedPermissions: decision.learnedRules || null,
      }));

      this.eventEmitter.emit('tool:requested', {
        sessionId,
        agent,
        tool: msg.tool,
        decision,
      });
    }
  }

  private async handleStreamEvent(sessionId: string, agent: AgentName, msg: StreamEvent) {
    this.eventEmitter.emit('stream:token', {
      sessionId,
      agent,
      delta: msg.delta,
    });
  }

  private async handleAssistant(sessionId: string, agent: AgentName, msg: AssistantMessage) {
    const session = this.sessionManager.getSession(sessionId);
    const connection = session?.agents.get(agent);
    if (!connection) return;

    connection.status = 'idle';
    session.messageHistory.push(msg);

    this.eventEmitter.emit('agent:completed', {
      sessionId,
      agent,
      message: msg.message,
      usage: msg.usage,
    });
  }

  private async handleResult(sessionId: string, agent: AgentName, msg: ResultMessage) {
    this.eventEmitter.emit('agent:result', {
      sessionId,
      agent,
      status: msg.status,
    });
  }

  private allAgentsReady(session: Session): boolean {
    return Array.from(session.agents.values()).every(c => c.status === 'ready');
  }
}
```

### 4. Debate Engine

Orchestrates iterative peer review between agents.

```typescript
// lib/sdk-server/debate-engine.ts

interface DebateRound {
  roundNumber: number;
  assignments: Map<AgentName, DebateTask>;
  results: Map<AgentName, AssistantMessage>;
  status: 'pending' | 'in_progress' | 'completed';
}

interface DebateTask {
  agent: AgentName;
  role: 'proposer' | 'reviewer' | 'synthesizer';
  prompt: string;
  reviewTarget?: AgentName; // which agent's work to review
}

class DebateEngine {
  constructor(
    private sessionManager: SessionManager,
    private messageRouter: MessageRouter,
  ) {}

  async startDebate(
    sessionId: string,
    prompt: string,
    pattern: 'sequential' | 'round_robin' | 'tournament',
    maxRounds: number = 3,
  ): Promise<void> {
    const session = this.sessionManager.getSession(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);

    const agents = Array.from(session.agents.keys());

    switch (pattern) {
      case 'sequential':
        await this.runSequentialDebate(sessionId, prompt, agents, maxRounds);
        break;

      case 'round_robin':
        await this.runRoundRobinDebate(sessionId, prompt, agents, maxRounds);
        break;

      case 'tournament':
        await this.runTournamentDebate(sessionId, prompt, agents);
        break;
    }
  }

  /**
   * Sequential: Agent A proposes → B reviews → C reviews B's review → D synthesizes
   */
  private async runSequentialDebate(
    sessionId: string,
    prompt: string,
    agents: AgentName[],
    maxRounds: number,
  ) {
    let context = prompt;

    for (let round = 0; round < maxRounds; round++) {
      const agent = agents[round % agents.length];
      const role = round === maxRounds - 1 ? 'synthesizer' : 'reviewer';

      const augmentedPrompt = this.buildDebatePrompt({
        originalPrompt: prompt,
        role,
        round,
        previousWork: context,
      });

      // Send to agent
      await this.sessionManager.sendToAgent(sessionId, agent, {
        type: 'user',
        message: { role: 'user', content: augmentedPrompt },
        parent_tool_use_id: null,
        session_id: sessionId,
      });

      // Wait for response
      const response = await this.waitForAgentResponse(sessionId, agent);

      // Update context for next round
      context = this.extractResponseText(response);
    }
  }

  /**
   * Round Robin: All agents work in parallel → peer review each other → iterate
   */
  private async runRoundRobinDebate(
    sessionId: string,
    prompt: string,
    agents: AgentName[],
    maxRounds: number,
  ) {
    const workMap = new Map<AgentName, string>();

    // Round 0: All agents propose solutions in parallel
    const initialPrompts = agents.map(agent => ({
      agent,
      prompt: this.buildDebatePrompt({
        originalPrompt: prompt,
        role: 'proposer',
        round: 0,
      }),
    }));

    await Promise.all(
      initialPrompts.map(({ agent, prompt }) =>
        this.sessionManager.sendToAgent(sessionId, agent, {
          type: 'user',
          message: { role: 'user', content: prompt },
          parent_tool_use_id: null,
          session_id: sessionId,
        }),
      ),
    );

    const initialResults = await Promise.all(
      agents.map(agent => this.waitForAgentResponse(sessionId, agent)),
    );

    initialResults.forEach((result, idx) => {
      workMap.set(agents[idx], this.extractResponseText(result));
    });

    // Rounds 1..N: Each agent reviews the NEXT agent's work
    for (let round = 1; round < maxRounds; round++) {
      const reviewPrompts = agents.map((agent, idx) => {
        const reviewTarget = agents[(idx + 1) % agents.length];
        const targetWork = workMap.get(reviewTarget)!;

        return {
          agent,
          prompt: this.buildDebatePrompt({
            originalPrompt: prompt,
            role: 'reviewer',
            round,
            previousWork: targetWork,
            reviewTarget,
          }),
        };
      });

      await Promise.all(
        reviewPrompts.map(({ agent, prompt }) =>
          this.sessionManager.sendToAgent(sessionId, agent, {
            type: 'user',
            message: { role: 'user', content: prompt },
            parent_tool_use_id: null,
            session_id: sessionId,
          }),
        ),
      );

      const reviewResults = await Promise.all(
        agents.map(agent => this.waitForAgentResponse(sessionId, agent)),
      );

      // Check for convergence (optional early stop)
      const currentHash = this.hashAllWork(reviewResults);
      const previousHash = this.hashAllWork(Array.from(workMap.values()));
      if (currentHash === previousHash) {
        console.log(`Debate converged at round ${round}`);
        break;
      }

      // Update work map
      reviewResults.forEach((result, idx) => {
        workMap.set(agents[idx], this.extractResponseText(result));
      });
    }

    // Final synthesis: Pick one agent to merge all perspectives
    const synthesizer = agents[0];
    const allWork = Array.from(workMap.entries())
      .map(([agent, work]) => `## ${agent}\n${work}`)
      .join('\n\n');

    await this.sessionManager.sendToAgent(sessionId, synthesizer, {
      type: 'user',
      message: {
        role: 'user',
        content: this.buildDebatePrompt({
          originalPrompt: prompt,
          role: 'synthesizer',
          round: maxRounds,
          previousWork: allWork,
        }),
      },
      parent_tool_use_id: null,
      session_id: sessionId,
    });

    await this.waitForAgentResponse(sessionId, synthesizer);
  }

  /**
   * Tournament: Pair agents, winners advance, final synthesis
   */
  private async runTournamentDebate(
    sessionId: string,
    prompt: string,
    agents: AgentName[],
  ) {
    let currentRound = agents;
    let roundNumber = 0;

    while (currentRound.length > 1) {
      const nextRound: AgentName[] = [];

      // Pair agents
      for (let i = 0; i < currentRound.length; i += 2) {
        const agent1 = currentRound[i];
        const agent2 = currentRound[i + 1];

        if (!agent2) {
          // Odd number, agent1 advances automatically
          nextRound.push(agent1);
          continue;
        }

        // Both agents solve in parallel
        await Promise.all([
          this.sessionManager.sendToAgent(sessionId, agent1, {
            type: 'user',
            message: { role: 'user', content: prompt },
            parent_tool_use_id: null,
            session_id: sessionId,
          }),
          this.sessionManager.sendToAgent(sessionId, agent2, {
            type: 'user',
            message: { role: 'user', content: prompt },
            parent_tool_use_id: null,
            session_id: sessionId,
          }),
        ]);

        const [result1, result2] = await Promise.all([
          this.waitForAgentResponse(sessionId, agent1),
          this.waitForAgentResponse(sessionId, agent2),
        ]);

        // Arbitration: pick winner (could be LLM-judged or metric-based)
        const winner = this.selectWinner(result1, result2);
        nextRound.push(winner === result1 ? agent1 : agent2);
      }

      currentRound = nextRound;
      roundNumber++;
    }

    // Winner is currentRound[0] — already has the best solution
    console.log(`Tournament winner: ${currentRound[0]}`);
  }

  private buildDebatePrompt(opts: {
    originalPrompt: string;
    role: 'proposer' | 'reviewer' | 'synthesizer';
    round: number;
    previousWork?: string;
    reviewTarget?: AgentName;
  }): string {
    const { originalPrompt, role, round, previousWork, reviewTarget } = opts;

    const rolePrompts = {
      proposer: `You are participating in a debate. Your role is to propose a solution to the following task.\n\n**Task**: ${originalPrompt}\n\nProvide your best solution.`,

      reviewer: `You are participating in a debate (Round ${round}). Your role is to review ${reviewTarget}'s work and provide constructive feedback.\n\n**Original Task**: ${originalPrompt}\n\n**${reviewTarget}'s Work**:\n${previousWork}\n\nProvide specific feedback: strengths, weaknesses, and suggested improvements.`,

      synthesizer: `You are the final synthesizer in a debate. Your role is to merge all perspectives into a coherent, optimal solution.\n\n**Original Task**: ${originalPrompt}\n\n**All Agent Contributions**:\n${previousWork}\n\nSynthesize the best elements from each contribution into a final answer.`,
    };

    return rolePrompts[role];
  }

  private async waitForAgentResponse(
    sessionId: string,
    agent: AgentName,
    timeout = 300000, // 5 min
  ): Promise<AssistantMessage> {
    return new Promise((resolve, reject) => {
      const session = this.sessionManager.getSession(sessionId);
      if (!session) return reject(new Error(`Session ${sessionId} not found`));

      const timer = setTimeout(() => {
        reject(new Error(`Agent ${agent} timed out`));
      }, timeout);

      const handler = (event: { sessionId: string; agent: AgentName; message: any }) => {
        if (event.sessionId === sessionId && event.agent === agent) {
          clearTimeout(timer);
          this.messageRouter.off('agent:completed', handler);
          resolve(event.message);
        }
      };

      this.messageRouter.on('agent:completed', handler);
    });
  }

  private extractResponseText(msg: AssistantMessage): string {
    // Extract text from Claude's response format
    const content = msg.message?.content || [];
    return content
      .filter((block: any) => block.type === 'text')
      .map((block: any) => block.text)
      .join('\n');
  }

  private hashAllWork(work: (AssistantMessage | string)[]): string {
    const texts = work.map(w =>
      typeof w === 'string' ? w : this.extractResponseText(w)
    );
    const combined = texts.join('::');
    return Bun.hash(combined).toString();
  }

  private selectWinner(result1: AssistantMessage, result2: AssistantMessage): AssistantMessage {
    // Simple heuristic: longer response wins (replace with LLM judge)
    const text1 = this.extractResponseText(result1);
    const text2 = this.extractResponseText(result2);
    return text1.length > text2.length ? result1 : result2;
  }
}
```

---

## Debate Mode Design

### Patterns

| Pattern | Description | Use Case |
|---------|-------------|----------|
| **Sequential** | A→B→C→D (each reviews previous) | Code review pipeline |
| **Round Robin** | All parallel → peer review → iterate | Algorithm optimization |
| **Tournament** | Bracket-style elimination | Best solution selection |

### Sequential Debate Flow

```
Round 0: Claude proposes solution
         ↓
Round 1: Codex reviews Claude's solution, suggests improvements
         ↓
Round 2: Gemini reviews Codex's feedback, refines further
         ↓
Round 3: Vibe synthesizes all perspectives into final answer
```

### Round Robin Debate Flow

```
Round 0 (Parallel):
  Claude → Solution A
  Codex → Solution B
  Gemini → Solution C
  Vibe → Solution D

Round 1 (Peer Review):
  Claude reviews Codex's work
  Codex reviews Gemini's work
  Gemini reviews Vibe's work
  Vibe reviews Claude's work

Round 2 (Iterate):
  Each agent refines based on feedback received

Convergence check:
  If hash(round2) == hash(round1), stop early

Final Synthesis:
  Claude merges all refined solutions
```

### Tournament Debate Flow

```
Round 1:
  Claude vs Codex → winner: Claude
  Gemini vs Vibe → winner: Gemini

Round 2:
  Claude vs Gemini → winner: Claude

Claude's solution is the tournament winner
```

### Integration with Council.sh

Add `--debate` flag to `council.sh`:

```bash
# Standard parallel (existing)
./council.sh "Fix auth bug"

# Debate mode (new)
./council.sh "Fix auth bug" --debate
./council.sh "Fix auth bug" --debate --pattern round_robin --rounds 3
```

Orchestration:

1. `council.sh` detects `--debate` flag
2. Launches SDK server: `bun run sdk-server/index.ts --port 8765`
3. Launches agents with `--sdk-url` instead of `--print`
4. SDK server runs debate orchestration
5. Final result written to `output/<session>/debate-synthesis.json`

---

## Implementation Roadmap

### Phase 1: Core SDK Server (Week 1-2)

**Goal**: Basic WebSocket server with session management

- [ ] Project setup: `bun create hono sdk-server`
- [ ] WebSocket handler with NDJSON parsing
- [ ] Session manager (create, register agents, cleanup)
- [ ] Message router (handle `system/init`, `assistant`, `stream_event`)
- [ ] Permission controller (allow_all policy only)
- [ ] CLI launcher integration (spawn `claude --sdk-url`)
- [ ] Basic test: single agent completes a task

**Deliverables**:
- `sdk-server/index.ts` (main server)
- `sdk-server/session-manager.ts`
- `sdk-server/message-router.ts`
- `sdk-server/permission-controller.ts`
- `sdk-server/types.ts` (protocol types)

### Phase 2: Multi-Agent Orchestration (Week 3)

**Goal**: Parallel execution with real-time streaming

- [ ] Multi-port WebSocket listeners (one per agent)
- [ ] Event emitter for TUI integration
- [ ] HTTP API for session control (POST /sessions, GET /sessions/:id)
- [ ] Streaming proxy (relay `stream_event` to TUI)
- [ ] Test: 4 agents in parallel, collect all outputs

**Deliverables**:
- `sdk-server/api.ts` (HTTP routes)
- `sdk-server/event-emitter.ts`
- Updated TUI to consume WebSocket events

### Phase 3: Permission System (Week 4)

**Goal**: Programmatic tool approval

- [ ] Policy types: allowlist, denylist, custom
- [ ] Argument modification (e.g., sanitize file paths)
- [ ] Permission learning (auto-approve similar future requests)
- [ ] Test: deny `Write` tool, allow `Read` only

**Deliverables**:
- Enhanced `permission-controller.ts`
- Policy configuration API

### Phase 4: Debate Engine (Week 5-6)

**Goal**: Iterative peer review

- [ ] Debate patterns: sequential, round_robin, tournament
- [ ] Context passing between rounds
- [ ] Convergence detection (hash-based early stop)
- [ ] LLM-based winner selection (tournament mode)
- [ ] Integration with `council.sh --debate`

**Deliverables**:
- `sdk-server/debate-engine.ts`
- CLI flag: `--debate --pattern round_robin --rounds 3`
- Example debate session output

### Phase 5: Production Hardening (Week 7-8)

**Goal**: Reliability, observability, deployment

- [ ] Authentication: JWT tokens for WebSocket connections
- [ ] Rate limiting per session
- [ ] Metrics: Prometheus-compatible endpoint
- [ ] Logging: structured JSON logs (bunyan/pino)
- [ ] Error recovery: agent reconnection logic
- [ ] Docker packaging
- [ ] Deployment guide (systemd, PM2, or K8s)

**Deliverables**:
- `sdk-server/auth.ts`
- `sdk-server/metrics.ts`
- `Dockerfile`
- `deploy/` directory with systemd service, nginx config

---

## Code Structure

```
ai-council/
├── sdk-server/                    # WebSocket SDK server (Bun + Hono)
│   ├── index.ts                   # Main entry point, WebSocket listeners
│   ├── types.ts                   # Protocol types (NDJSON messages)
│   ├── session-manager.ts         # Session lifecycle, agent registration
│   ├── message-router.ts          # NDJSON message routing
│   ├── permission-controller.ts   # Tool approval policies
│   ├── debate-engine.ts           # Debate orchestration
│   ├── api.ts                     # HTTP control API (Hono routes)
│   ├── event-emitter.ts           # Event bus for TUI integration
│   ├── auth.ts                    # JWT authentication
│   ├── metrics.ts                 # Prometheus metrics
│   └── utils/
│       ├── ndjson.ts              # NDJSON parser/serializer
│       └── process.ts             # CLI process spawner
│
├── lib/                           # Existing Bash libraries (kept)
│   ├── collab.sh
│   ├── container-ops.sh
│   ├── graph-exec.sh
│   ├── output-collector.sh
│   └── synthesizer.sh
│
├── council.sh                     # Enhanced with --debate flag
│
├── cmd/
│   └── council-tui/               # Go TUI (Bubbletea)
│       ├── main.go
│       └── ...
│
├── config/
│   ├── council.conf
│   └── sdk-server.toml            # SDK server config
│
├── package.json                   # Bun dependencies
├── tsconfig.json                  # TypeScript config
└── README.md
```

---

## Example Implementations

### 1. Minimal SDK Server

```typescript
// sdk-server/index.ts

import { Hono } from 'hono';
import { SessionManager } from './session-manager';
import { MessageRouter } from './message-router';
import { PermissionController } from './permission-controller';

const app = new Hono();
const sessionManager = new SessionManager();
const permissionController = new PermissionController();
const messageRouter = new MessageRouter(sessionManager, permissionController);

const agentPorts = {
  claude: 8765,
  codex: 8766,
  gemini: 8767,
  vibe: 8768,
};

for (const [agent, port] of Object.entries(agentPorts)) {
  Bun.serve({
    port,
    fetch(req, server) {
      const url = new URL(req.url);

      if (url.pathname === '/ws') {
        const upgraded = server.upgrade(req, {
          data: {
            agent,
            sessionId: url.searchParams.get('session') || 'default',
          },
        });
        if (upgraded) return undefined;
      }

      return new Response('Expected WebSocket', { status: 400 });
    },

    websocket: {
      open(ws) {
        console.log(`[${ws.data.agent}] connected to session ${ws.data.sessionId}`);
        sessionManager.registerAgent(ws.data.sessionId, ws.data.agent, ws);
      },

      message(ws, message) {
        messageRouter.handleMessage(ws, message.toString());
      },

      close(ws) {
        console.log(`[${ws.data.agent}] disconnected`);
        sessionManager.unregisterAgent(ws.data.sessionId, ws.data.agent);
      },
    },
  });

  console.log(`✓ ${agent} listening on ws://localhost:${port}/ws`);
}

// HTTP API for control
app.post('/sessions', async (c) => {
  const { prompt, agents } = await c.req.json();
  const session = sessionManager.createSession(prompt, agents);

  // Launch CLI processes
  for (const agent of agents) {
    const port = agentPorts[agent];
    const proc = Bun.spawn([
      'claude', // or codex, gemini, vibe
      '--sdk-url', `ws://localhost:${port}/ws?session=${session.id}`,
      '--print',
      '--output-format', 'stream-json',
      '--input-format', 'stream-json',
      '--verbose',
    ]);
  }

  return c.json({ sessionId: session.id });
});

app.get('/sessions/:id', (c) => {
  const session = sessionManager.getSession(c.req.param('id'));
  if (!session) return c.json({ error: 'Not found' }, 404);
  return c.json(session);
});

export default {
  port: 8000,
  fetch: app.fetch,
};
```

### 2. Launch Script Integration

```bash
# lib/sdk-launcher.sh

sdk_launch_agent() {
    local agent="$1"
    local session_id="$2"
    local port="$3"

    local cmd=""
    case "$agent" in
        claude) cmd="claude" ;;
        codex)  cmd="codex" ;;
        gemini) cmd="gemini" ;;
        vibe)   cmd="vibe" ;;
    esac

    "$cmd" \
        --sdk-url "ws://localhost:${port}/ws?session=${session_id}" \
        --print \
        --output-format stream-json \
        --input-format stream-json \
        --verbose &

    echo $!
}

sdk_start_server() {
    local session_id="$1"

    # Start SDK server in background
    bun run "${SCRIPT_DIR}/sdk-server/index.ts" &
    local server_pid=$!

    # Wait for server to be ready
    sleep 2

    echo "$server_pid"
}
```

### 3. Council.sh Integration

```bash
# Add to council.sh after argument parsing

if [ "$DEBATE_MODE" = true ]; then
    # Start SDK server
    SDK_SERVER_PID=$(sdk_start_server "$SESSION_ID")

    # Launch agents with WebSocket URLs
    for agent in $ACTIVE_AGENTS; do
        port="${AGENT_PORTS[$agent]}"
        sdk_launch_agent "$agent" "$SESSION_ID" "$port"
    done

    # Start debate orchestration via HTTP API
    curl -X POST http://localhost:8000/sessions \
        -H "Content-Type: application/json" \
        -d "{\"prompt\":\"$PROMPT\",\"agents\":[$ACTIVE_AGENTS]}"

    # Wait for debate to complete
    # (poll GET /sessions/:id until status=completed)

    # Cleanup
    kill "$SDK_SERVER_PID"

    exit 0
fi
```

---

## Testing Strategy

### Unit Tests

```typescript
// sdk-server/__tests__/session-manager.test.ts

import { describe, test, expect } from 'bun:test';
import { SessionManager } from '../session-manager';

describe('SessionManager', () => {
  test('creates session with unique ID', () => {
    const manager = new SessionManager();
    const session1 = manager.createSession('test prompt', ['claude']);
    const session2 = manager.createSession('test prompt', ['claude']);

    expect(session1.id).not.toBe(session2.id);
  });

  test('registers agent connection', () => {
    const manager = new SessionManager();
    const session = manager.createSession('test', ['claude']);
    const mockWs = {} as WebSocket;

    manager.registerAgent(session.id, 'claude', mockWs);

    const retrieved = manager.getSession(session.id);
    expect(retrieved?.agents.has('claude')).toBe(true);
  });
});
```

### Integration Tests

```typescript
// sdk-server/__tests__/integration.test.ts

import { describe, test, expect } from 'bun:test';
import { WebSocket } from 'ws';

describe('SDK Server Integration', () => {
  test('full session lifecycle', async () => {
    // 1. Create session via HTTP API
    const res = await fetch('http://localhost:8000/sessions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt: 'test', agents: ['claude'] }),
    });
    const { sessionId } = await res.json();

    // 2. Connect WebSocket
    const ws = new WebSocket(`ws://localhost:8765/ws?session=${sessionId}`);
    await new Promise(resolve => ws.on('open', resolve));

    // 3. Send system/init
    ws.send(JSON.stringify({
      type: 'system/init',
      tools: [{ name: 'Read' }, { name: 'Edit' }],
      session_id: '',
    }));

    // 4. Receive user message
    const userMsg = await new Promise(resolve => {
      ws.on('message', (data) => {
        const msg = JSON.parse(data.toString());
        if (msg.type === 'user') resolve(msg);
      });
    });

    expect(userMsg.message.content).toBe('test');

    // 5. Send assistant response
    ws.send(JSON.stringify({
      type: 'assistant',
      message: { role: 'assistant', content: 'Done' },
      session_id: sessionId,
    }));

    ws.close();
  });
});
```

### End-to-End Tests

```bash
#!/bin/bash
# tests/e2e/debate-mode.sh

set -euo pipefail

# 1. Start SDK server
bun run sdk-server/index.ts &
SERVER_PID=$!
sleep 2

# 2. Run debate mode
./council.sh "Write a factorial function" --debate --pattern sequential --rounds 3

# 3. Verify output
if [ -f "output/*/debate-synthesis.json" ]; then
    echo "✓ Debate completed successfully"
else
    echo "✗ Debate output not found"
    exit 1
fi

# 4. Cleanup
kill $SERVER_PID
```

---

## Production Considerations

### 1. Authentication & Authorization

```typescript
// sdk-server/auth.ts

import jwt from 'jsonwebtoken';

interface SessionToken {
  sessionId: string;
  agents: string[];
  expiresAt: number;
}

export function generateSessionToken(sessionId: string, agents: string[]): string {
  const payload: SessionToken = {
    sessionId,
    agents,
    expiresAt: Date.now() + 3600000, // 1 hour
  };

  return jwt.sign(payload, process.env.JWT_SECRET || 'dev-secret');
}

export function validateToken(token: string): SessionToken | null {
  try {
    const payload = jwt.verify(token, process.env.JWT_SECRET || 'dev-secret') as SessionToken;
    if (payload.expiresAt < Date.now()) return null;
    return payload;
  } catch {
    return null;
  }
}
```

Usage:

```typescript
// In WebSocket handler
const authHeader = req.headers.get('Authorization');
if (!authHeader?.startsWith('Bearer ')) {
  return new Response('Unauthorized', { status: 401 });
}

const token = authHeader.slice(7);
const sessionData = validateToken(token);
if (!sessionData) {
  return new Response('Invalid token', { status: 401 });
}

ws.data.sessionId = sessionData.sessionId;
```

### 2. Rate Limiting

```typescript
// sdk-server/rate-limiter.ts

class RateLimiter {
  private requests = new Map<string, number[]>();

  isAllowed(sessionId: string, limit = 100, window = 60000): boolean {
    const now = Date.now();
    const windowStart = now - window;

    const timestamps = (this.requests.get(sessionId) || [])
      .filter(ts => ts > windowStart);

    if (timestamps.length >= limit) return false;

    timestamps.push(now);
    this.requests.set(sessionId, timestamps);
    return true;
  }
}
```

### 3. Observability

```typescript
// sdk-server/metrics.ts

import { Registry, Counter, Histogram } from 'prom-client';

const register = new Registry();

export const metrics = {
  sessionsCreated: new Counter({
    name: 'council_sessions_created_total',
    help: 'Total sessions created',
    registers: [register],
  }),

  messagesRouted: new Counter({
    name: 'council_messages_routed_total',
    help: 'Messages routed by type',
    labelNames: ['type', 'agent'],
    registers: [register],
  }),

  agentLatency: new Histogram({
    name: 'council_agent_latency_seconds',
    help: 'Agent response latency',
    labelNames: ['agent'],
    registers: [register],
  }),
};

// Expose /metrics endpoint
app.get('/metrics', async (c) => {
  return c.text(await register.metrics());
});
```

### 4. Error Recovery

```typescript
// sdk-server/reconnect.ts

class ReconnectionHandler {
  private reconnectAttempts = new Map<string, number>();

  async handleDisconnect(sessionId: string, agent: string, ws: WebSocket) {
    const attempts = this.reconnectAttempts.get(`${sessionId}:${agent}`) || 0;

    if (attempts >= 3) {
      console.error(`Agent ${agent} failed to reconnect after 3 attempts`);
      return;
    }

    // Exponential backoff
    const delay = Math.pow(2, attempts) * 1000;
    await new Promise(resolve => setTimeout(resolve, delay));

    // Attempt reconnect
    // (re-spawn CLI process with --resume flag)
    this.reconnectAttempts.set(`${sessionId}:${agent}`, attempts + 1);
  }
}
```

### 5. Deployment

#### Docker

```dockerfile
# Dockerfile

FROM oven/bun:1

WORKDIR /app

COPY package.json bun.lockb ./
RUN bun install --frozen-lockfile

COPY sdk-server/ ./sdk-server/
COPY tsconfig.json ./

EXPOSE 8000 8765 8766 8767 8768

CMD ["bun", "run", "sdk-server/index.ts"]
```

#### Systemd Service

```ini
# /etc/systemd/system/council-sdk.service

[Unit]
Description=AI Council WebSocket SDK Server
After=network.target

[Service]
Type=simple
User=council
WorkingDirectory=/opt/ai-council
ExecStart=/usr/local/bin/bun run sdk-server/index.ts
Restart=on-failure
RestartSec=5
Environment="NODE_ENV=production"
Environment="JWT_SECRET=<generated-secret>"

[Install]
WantedBy=multi-user.target
```

#### Nginx Reverse Proxy

```nginx
# /etc/nginx/sites-available/council-sdk

upstream council_http {
    server 127.0.0.1:8000;
}

upstream claude_ws {
    server 127.0.0.1:8765;
}

# ... repeat for other agents

server {
    listen 80;
    server_name council.example.com;

    location /api {
        proxy_pass http://council_http;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location /ws/claude {
        proxy_pass http://claude_ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 3600s;
    }
}
```

---

## Next Steps

1. **Validate protocol**: Run `claude --sdk-url ws://localhost:8765` manually, capture NDJSON messages
2. **Build minimal server**: Implement Phase 1 deliverables (basic session + message routing)
3. **Test with single agent**: Verify full conversation lifecycle
4. **Add multi-agent**: Parallel execution with 4 agents
5. **Implement Debate Mode**: Start with sequential pattern
6. **Integrate with TUI**: Stream events to Bubbletea interface
7. **Production hardening**: Auth, metrics, deployment

---

## Questions & Design Choices

### Why Bun instead of Node?

- **Native WebSocket support** (no `ws` library needed)
- **Fast startup** (< 100ms cold boot vs Node's 500ms+)
- **TypeScript-first** (no build step for development)
- **Process spawning** with `Bun.spawn()` (better than `child_process`)

### Why per-agent ports instead of single WS + routing?

- **Simpler auth**: One token per agent, no need to parse messages for routing
- **Isolation**: Agent crash doesn't affect others
- **Backward compat**: Can run SDK + legacy bash orchestrator side-by-side

### Why keep Bash orchestrator?

- **Proven auth handling**: Keychain extraction, OAuth mounting
- **Container lifecycle**: Apple container quirks already solved
- **Progressive enhancement**: SDK is additive, not a rewrite

### How to handle agent failures in Debate Mode?

- **Retry once**: Same agent, fresh connection
- **Fallback**: Skip agent, continue with remaining agents
- **Abort**: If < 50% agents respond, fail entire debate

### How to judge "winner" in tournament mode?

- **LLM judge**: Send both solutions to Claude Opus, ask it to pick
- **Metrics**: Code coverage, test pass rate, execution time
- **User vote**: Present both to user, let them choose

---

**Reference**: [Companion WebSocket Protocol](https://github.com/The-Vibe-Company/companion/blob/main/WEBSOCKET_PROTOCOL_REVERSED.md)

**Status**: Design complete, ready for implementation

**Estimated effort**: 6-8 weeks for full production-ready system
