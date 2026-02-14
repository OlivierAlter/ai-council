import { encodeNDJSON, NDJSONFramer } from "../protocol/codec";
import type { AnySdkMessage, ControlRequestPayload, SdkControlResponseMessage } from "../protocol/types";

export type WSData = {
  role: "agent" | "tui";
  agentId?: string;
  sessionId?: string;
};

export interface SocketLike {
  send(data: string): void;
  close(code?: number, reason?: string): void;
}

export interface GatewayEvent {
  id: string;
  ts: string;
  sessionId: string;
  type: string;
  agentId?: string;
  payload: unknown;
}

export interface AgentConnection {
  agentId: string;
  councilSessionId: string;
  ws: SocketLike;
  framer: NDJSONFramer;
  connectedAt: number;
  lastMessageAt: number;
  sdkSessionId?: string;
  model?: string;
  tools: string[];
  pendingRequests: Map<string, PendingControlRequest>;
}

export interface CouncilSession {
  id: string;
  mode: "standard" | "debate";
  createdAt: number;
  agentIds: Set<string>;
  subscribers: Set<SocketLike>;
  events: GatewayEvent[];
  debate?: unknown;
}

interface PendingControlRequest {
  resolve: (value: SdkControlResponseMessage["response"]) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
}

export class GatewayState {
  private readonly sessions = new Map<string, CouncilSession>();
  private readonly agents = new Map<string, AgentConnection>();
  private readonly wsToAgentId = new WeakMap<SocketLike, string>();
  private readonly wsToSessionId = new WeakMap<SocketLike, string>();

  constructor(private readonly maxEventBacklog: number) {}

  ensureSession(sessionId: string): CouncilSession {
    const existing = this.sessions.get(sessionId);
    if (existing) {
      return existing;
    }

    const created: CouncilSession = {
      id: sessionId,
      mode: "standard",
      createdAt: Date.now(),
      agentIds: new Set<string>(),
      subscribers: new Set<SocketLike>(),
      events: [],
    };
    this.sessions.set(sessionId, created);
    return created;
  }

  listSessions(): CouncilSession[] {
    return [...this.sessions.values()];
  }

  getSession(sessionId: string): CouncilSession | undefined {
    return this.sessions.get(sessionId);
  }

  getAgent(agentId: string): AgentConnection | undefined {
    return this.agents.get(agentId);
  }

  getAgentBySocket(ws: SocketLike): AgentConnection | undefined {
    const agentId = this.wsToAgentId.get(ws);
    if (!agentId) {
      return undefined;
    }
    return this.agents.get(agentId);
  }

  connectAgent(agentId: string, sessionId: string, ws: SocketLike): AgentConnection {
    const session = this.ensureSession(sessionId);

    const existing = this.agents.get(agentId);
    if (existing) {
      try {
        existing.ws.close(1012, "replaced by newer connection");
      } catch {
        // ignore close errors
      }
      for (const pending of existing.pendingRequests.values()) {
        clearTimeout(pending.timer);
        pending.reject(new Error(`Agent ${agentId} connection replaced`));
      }
      this.agents.delete(agentId);
    }

    const conn: AgentConnection = {
      agentId,
      councilSessionId: sessionId,
      ws,
      framer: new NDJSONFramer(),
      connectedAt: Date.now(),
      lastMessageAt: Date.now(),
      tools: [],
      pendingRequests: new Map<string, PendingControlRequest>(),
    };

    this.agents.set(agentId, conn);
    this.wsToAgentId.set(ws, agentId);
    session.agentIds.add(agentId);

    return conn;
  }

  disconnectAgentSocket(ws: SocketLike): void {
    const agentId = this.wsToAgentId.get(ws);
    if (!agentId) {
      return;
    }

    const conn = this.agents.get(agentId);
    if (!conn) {
      return;
    }

    for (const pending of conn.pendingRequests.values()) {
      clearTimeout(pending.timer);
      pending.reject(new Error(`Agent ${agentId} disconnected before control response`));
    }

    const session = this.sessions.get(conn.councilSessionId);
    if (session) {
      session.agentIds.delete(agentId);
    }

    this.agents.delete(agentId);
  }

  connectSubscriber(sessionId: string, ws: SocketLike): GatewayEvent[] {
    const session = this.ensureSession(sessionId);
    session.subscribers.add(ws);
    this.wsToSessionId.set(ws, sessionId);
    return session.events;
  }

  disconnectSubscriberSocket(ws: SocketLike): void {
    const sessionId = this.wsToSessionId.get(ws);
    if (!sessionId) {
      return;
    }

    const session = this.sessions.get(sessionId);
    if (!session) {
      return;
    }

    session.subscribers.delete(ws);
  }

  updateAgentInit(agentId: string, payload: { sdkSessionId: string; model?: string; tools?: string[] }): void {
    const conn = this.agents.get(agentId);
    if (!conn) {
      return;
    }
    conn.sdkSessionId = payload.sdkSessionId;
    conn.model = payload.model;
    conn.tools = payload.tools ?? [];
  }

  markAgentMessageSeen(agentId: string): void {
    const conn = this.agents.get(agentId);
    if (!conn) {
      return;
    }
    conn.lastMessageAt = Date.now();
  }

  setSessionMode(sessionId: string, mode: "standard" | "debate"): void {
    const session = this.ensureSession(sessionId);
    session.mode = mode;
  }

  setDebateRuntime(sessionId: string, runtime: unknown): void {
    const session = this.ensureSession(sessionId);
    session.debate = runtime;
  }

  getDebateRuntime<T>(sessionId: string): T | undefined {
    const session = this.sessions.get(sessionId);
    if (!session) {
      return undefined;
    }
    return session.debate as T | undefined;
  }

  clearDebateRuntime(sessionId: string): void {
    const session = this.sessions.get(sessionId);
    if (!session) {
      return;
    }
    delete session.debate;
  }

  emit(sessionId: string, type: string, payload: unknown, agentId?: string): GatewayEvent {
    const session = this.ensureSession(sessionId);
    const event: GatewayEvent = {
      id: crypto.randomUUID(),
      ts: new Date().toISOString(),
      sessionId,
      type,
      payload,
      ...(agentId ? { agentId } : {}),
    };

    session.events.push(event);
    if (session.events.length > this.maxEventBacklog) {
      session.events.splice(0, session.events.length - this.maxEventBacklog);
    }

    const serialized = JSON.stringify(event);
    for (const subscriber of session.subscribers) {
      try {
        subscriber.send(serialized);
      } catch {
        session.subscribers.delete(subscriber);
      }
    }

    return event;
  }

  sendRawToAgent(agentId: string, message: AnySdkMessage | Record<string, unknown>): boolean {
    const conn = this.agents.get(agentId);
    if (!conn) {
      return false;
    }

    try {
      conn.ws.send(encodeNDJSON(message));
      return true;
    } catch {
      return false;
    }
  }

  sendUserMessage(agentId: string, content: string, parentToolUseId: string | null = null): boolean {
    const conn = this.agents.get(agentId);
    if (!conn) {
      return false;
    }

    const message = {
      type: "user",
      message: {
        role: "user",
        content,
      },
      parent_tool_use_id: parentToolUseId,
      session_id: conn.sdkSessionId ?? "",
    };

    return this.sendRawToAgent(agentId, message);
  }

  async sendControlRequest(
    agentId: string,
    request: ControlRequestPayload,
    timeoutMs: number,
  ): Promise<SdkControlResponseMessage["response"]> {
    const conn = this.agents.get(agentId);
    if (!conn) {
      throw new Error(`Agent not connected: ${agentId}`);
    }

    const requestId = crypto.randomUUID();
    const payload = {
      type: "control_request",
      request_id: requestId,
      request,
    };

    return new Promise<SdkControlResponseMessage["response"]>((resolve, reject) => {
      const timer = setTimeout(() => {
        conn.pendingRequests.delete(requestId);
        reject(new Error(`Control request timed out: ${request.subtype}`));
      }, timeoutMs);

      conn.pendingRequests.set(requestId, {
        resolve,
        reject,
        timer,
      });

      const sent = this.sendRawToAgent(agentId, payload);
      if (!sent) {
        clearTimeout(timer);
        conn.pendingRequests.delete(requestId);
        reject(new Error(`Failed sending control request ${request.subtype} to ${agentId}`));
      }
    });
  }

  resolveControlResponse(agentId: string, response: SdkControlResponseMessage["response"]): void {
    const conn = this.agents.get(agentId);
    if (!conn) {
      return;
    }

    const pending = conn.pendingRequests.get(response.request_id);
    if (!pending) {
      return;
    }

    clearTimeout(pending.timer);
    conn.pendingRequests.delete(response.request_id);
    pending.resolve(response);
  }

  sessionSnapshot(sessionId: string): Record<string, unknown> {
    const session = this.sessions.get(sessionId);
    if (!session) {
      return {
        id: sessionId,
        exists: false,
      };
    }

    return {
      id: session.id,
      mode: session.mode,
      createdAt: new Date(session.createdAt).toISOString(),
      agents: [...session.agentIds].map((agentId) => {
        const conn = this.agents.get(agentId);
        return {
          agentId,
          connected: Boolean(conn),
          sdkSessionId: conn?.sdkSessionId,
          model: conn?.model,
          tools: conn?.tools ?? [],
          lastMessageAt: conn ? new Date(conn.lastMessageAt).toISOString() : null,
        };
      }),
      subscribers: session.subscribers.size,
      eventCount: session.events.length,
      debateActive: Boolean(session.debate),
      recentEvents: session.events.slice(-20),
    };
  }
}
