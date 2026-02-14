import { Hono } from "hono";
import { createBunWebSocket } from "hono/bun";
import { loadConfig } from "./config";
import { ClaudeLauncher } from "./core/claude-launcher";
import { DebateCoordinator } from "./core/debate-mode";
import { GatewayState, type WSData } from "./core/gateway-state";
import { MessageRouter } from "./core/message-router";
import { DefaultPermissionPolicy } from "./core/permission-policy";
import type { ControlRequestPayload } from "./protocol/types";

const config = loadConfig();
const state = new GatewayState(config.maxEventBacklog);
const permissions = new DefaultPermissionPolicy();
const debates = new DebateCoordinator(state);
const router = new MessageRouter(state, permissions, debates);
const launcher = new ClaudeLauncher(state, config);

const app = new Hono();
const { upgradeWebSocket, websocket } = createBunWebSocket<WSData>();

app.get("/healthz", (c) =>
  c.json({
    ok: true,
    service: "ai-council-sdk-gateway",
    timestamp: new Date().toISOString(),
    sessions: state.listSessions().length,
  }),
);

app.get("/api/sessions", (c) => {
  return c.json({
    sessions: state.listSessions().map((session) => ({
      id: session.id,
      mode: session.mode,
      createdAt: new Date(session.createdAt).toISOString(),
      agents: [...session.agentIds],
      subscribers: session.subscribers.size,
      eventCount: session.events.length,
      debateActive: Boolean(session.debate),
    })),
  });
});

app.post("/api/sessions", async (c) => {
  const body = await safeJson(c.req);
  const sessionId = asNonEmptyString(body.sessionId) ?? crypto.randomUUID();
  state.ensureSession(sessionId);
  state.emit(sessionId, "session.created", {
    sessionId,
    source: "api",
  });
  return c.json({ sessionId });
});

app.get("/api/sessions/:sessionId", (c) => {
  const sessionId = c.req.param("sessionId");
  return c.json(state.sessionSnapshot(sessionId));
});

app.post("/api/sessions/:sessionId/agents/:agentId/launch", async (c) => {
  const sessionId = c.req.param("sessionId");
  const agentId = c.req.param("agentId");
  const body = await safeJson(c.req);

  state.ensureSession(sessionId);

  const result = launcher.launch({
    sessionId,
    agentId,
    model: asOptionalString(body.model),
    resumeSessionId: asOptionalString(body.resumeSessionId),
    maxTurns: asOptionalNumber(body.maxTurns),
  });

  return c.json({
    launched: true,
    agentId,
    ...result,
  });
});

app.post("/api/sessions/:sessionId/initialize", async (c) => {
  const sessionId = c.req.param("sessionId");
  const body = await safeJson(c.req);

  const payload: ControlRequestPayload = {
    subtype: "initialize",
    ...(asOptionalString(body.systemPrompt) ? { systemPrompt: asOptionalString(body.systemPrompt) } : {}),
    ...(asOptionalString(body.appendSystemPrompt)
      ? { appendSystemPrompt: asOptionalString(body.appendSystemPrompt) }
      : config.defaultAppendSystemPrompt
        ? { appendSystemPrompt: config.defaultAppendSystemPrompt }
        : {}),
    ...(body.hooks && typeof body.hooks === "object" ? { hooks: body.hooks as Record<string, unknown> } : {}),
    ...(Array.isArray(body.sdkMcpServers) ? { sdkMcpServers: body.sdkMcpServers.map(String) } : {}),
    ...(body.agents && typeof body.agents === "object" ? { agents: body.agents as Record<string, unknown> } : {}),
    ...(body.jsonSchema && typeof body.jsonSchema === "object"
      ? { jsonSchema: body.jsonSchema as Record<string, unknown> }
      : {}),
  };

  const targetAgentIds = resolveTargetAgents(state, sessionId, toStringArray(body.agentIds));
  const results = await Promise.allSettled(
    targetAgentIds.map((agentId) => state.sendControlRequest(agentId, payload, config.maxControlWaitMs)),
  );

  const response = targetAgentIds.map((agentId, index) => {
    const item = results[index];
    if (item.status === "fulfilled") {
      return {
        agentId,
        ok: item.value.subtype === "success",
        response: item.value,
      };
    }

    return {
      agentId,
      ok: false,
      error: item.reason instanceof Error ? item.reason.message : String(item.reason),
    };
  });

  state.emit(sessionId, "session.initialize_sent", {
    agentIds: targetAgentIds,
    payload,
    response,
  });

  return c.json({
    sessionId,
    agents: response,
  });
});

app.post("/api/sessions/:sessionId/turn", async (c) => {
  const sessionId = c.req.param("sessionId");
  const body = await safeJson(c.req);
  const content = asNonEmptyString(body.content);

  if (!content) {
    return c.json({ error: "content is required" }, 400);
  }

  const targetAgents = resolveTargetAgents(state, sessionId, toStringArray(body.agentIds));
  let delivered = 0;

  for (const agentId of targetAgents) {
    if (state.sendUserMessage(agentId, content, null)) {
      delivered += 1;
      state.emit(sessionId, "session.turn_dispatched", { content, parent_tool_use_id: null }, agentId);
    }
  }

  return c.json({
    sessionId,
    delivered,
    attempted: targetAgents.length,
    targetAgents,
  });
});

app.post("/api/sessions/:sessionId/control/interrupt", async (c) => {
  const sessionId = c.req.param("sessionId");
  const body = await safeJson(c.req);
  const agentId = asNonEmptyString(body.agentId);

  if (!agentId) {
    return c.json({ error: "agentId is required" }, 400);
  }

  try {
    const response = await state.sendControlRequest(agentId, { subtype: "interrupt" }, config.maxControlWaitMs);
    state.emit(sessionId, "session.interrupt_sent", { agentId, response });
    return c.json({ ok: true, agentId, response });
  } catch (error) {
    return c.json({ ok: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

app.post("/api/sessions/:sessionId/debate/start", async (c) => {
  const sessionId = c.req.param("sessionId");
  const body = await safeJson(c.req);
  const prompt = asNonEmptyString(body.prompt);

  if (!prompt) {
    return c.json({ error: "prompt is required" }, 400);
  }

  const requestedAgents = toStringArray(body.agentIds);
  const agentIds = resolveTargetAgents(state, sessionId, requestedAgents.length > 0 ? requestedAgents : undefined);
  const rounds = asOptionalNumber(body.rounds) ?? config.defaultDebateRounds;

  try {
    debates.start(sessionId, prompt, agentIds, rounds);
    return c.json({ ok: true, sessionId, rounds, agentIds });
  } catch (error) {
    return c.json({ ok: false, error: error instanceof Error ? error.message : String(error) }, 400);
  }
});

app.post("/api/sessions/:sessionId/agents/:agentId/stop", (c) => {
  const sessionId = c.req.param("sessionId");
  const agentId = c.req.param("agentId");
  const stopped = launcher.stop(agentId);
  state.emit(sessionId, "agent.process.stop_requested", { agentId, stopped });
  return c.json({ sessionId, agentId, stopped });
});

app.get(
  "/ws/agent/:agentId",
  upgradeWebSocket((c) => {
    const auth = c.req.header("authorization");
    if (!isAuthorized(config.authBearerToken, auth)) {
      throw new Error("Unauthorized WebSocket connection");
    }

    const agentId = c.req.param("agentId");
    const sessionId = c.req.query("sessionId") ?? `session-${agentId}`;

    return {
      onOpen: (_event, ws) => {
        state.connectAgent(agentId, sessionId, ws);
        state.emit(sessionId, "agent.ws.connected", { agentId });
      },
      onMessage: async (event, ws) => {
        const connection = state.getAgentBySocket(ws);
        if (!connection) {
          return;
        }

        const payload = toText(event.data);
        const messages = connection.framer.push(payload);
        for (const message of messages) {
          await router.routeAgentMessage(connection.agentId, message);
        }
      },
      onClose: (_event, ws) => {
        const conn = state.getAgentBySocket(ws);
        if (conn) {
          state.emit(conn.councilSessionId, "agent.ws.disconnected", { agentId: conn.agentId }, conn.agentId);
        }
        state.disconnectAgentSocket(ws);
      },
      onError: (event, ws) => {
        const conn = state.getAgentBySocket(ws);
        if (!conn) {
          return;
        }
        state.emit(conn.councilSessionId, "agent.ws.error", { agentId: conn.agentId, error: String(event) }, conn.agentId);
      },
    };
  }),
);

app.get(
  "/ws/tui/:sessionId",
  upgradeWebSocket((c) => {
    const sessionId = c.req.param("sessionId");

    return {
      onOpen: (_event, ws) => {
        const backlog = state.connectSubscriber(sessionId, ws);

        ws.send(
          JSON.stringify({
            type: "session.snapshot",
            payload: state.sessionSnapshot(sessionId),
          }),
        );

        for (const event of backlog.slice(-200)) {
          ws.send(JSON.stringify(event));
        }
      },
      onMessage: async (event) => {
        const payload = safeParseJSON(toText(event.data));
        if (!payload || typeof payload !== "object") {
          return;
        }

        const action = asOptionalString((payload as Record<string, unknown>).action);
        if (action === "send_user") {
          const content = asNonEmptyString((payload as Record<string, unknown>).content);
          if (!content) {
            return;
          }

          const agents = resolveTargetAgents(
            state,
            sessionId,
            toStringArray((payload as Record<string, unknown>).agentIds),
          );
          for (const agentId of agents) {
            state.sendUserMessage(agentId, content);
            state.emit(sessionId, "tui.turn_dispatched", { content }, agentId);
          }
        }

        if (action === "interrupt") {
          const agentId = asNonEmptyString((payload as Record<string, unknown>).agentId);
          if (!agentId) {
            return;
          }

          try {
            const response = await state.sendControlRequest(agentId, { subtype: "interrupt" }, config.maxControlWaitMs);
            state.emit(sessionId, "tui.interrupt_sent", { agentId, response });
          } catch (error) {
            state.emit(sessionId, "tui.interrupt_failed", {
              agentId,
              error: error instanceof Error ? error.message : String(error),
            });
          }
        }
      },
      onClose: (_event, ws) => {
        state.disconnectSubscriberSocket(ws);
      },
    };
  }),
);

app.onError((error, c) => {
  return c.json(
    {
      ok: false,
      error: error.message,
    },
    500,
  );
});

const server = Bun.serve({
  fetch: app.fetch,
  websocket,
  hostname: config.host,
  port: config.port,
});

console.log(
  JSON.stringify({
    level: "info",
    message: "AI Council SDK gateway listening",
    port: config.port,
    host: config.host,
    wsAgentEndpoint: `${config.publicWsBaseUrl}/ws/agent/:agentId?sessionId=:sessionId`,
    wsTuiEndpoint: `${config.publicWsBaseUrl}/ws/tui/:sessionId`,
  }),
);

process.on("SIGTERM", () => {
  server.stop();
});

process.on("SIGINT", () => {
  server.stop();
});

function isAuthorized(expectedBearerToken: string | undefined, header: string | undefined): boolean {
  if (!expectedBearerToken) {
    return true;
  }

  if (!header || !header.startsWith("Bearer ")) {
    return false;
  }

  return header.slice("Bearer ".length).trim() === expectedBearerToken;
}

function resolveTargetAgents(state: GatewayState, sessionId: string, candidates?: string[]): string[] {
  const session = state.getSession(sessionId);
  if (!session) {
    return candidates ?? [];
  }

  const all = [...session.agentIds];
  if (!candidates || candidates.length === 0) {
    return all;
  }

  const existing = new Set(all);
  return candidates.filter((candidate) => existing.has(candidate));
}

async function safeJson(req: { json: () => Promise<unknown> }): Promise<Record<string, unknown>> {
  try {
    const parsed = await req.json();
    return parsed && typeof parsed === "object" ? (parsed as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

function asNonEmptyString(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }

  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}

function asOptionalString(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }

  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}

function asOptionalNumber(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }

  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number.parseInt(value, 10);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }

  return undefined;
}

function toStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return value.map((item) => String(item)).filter((item) => item.length > 0);
}

function safeParseJSON(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return undefined;
  }
}

function toText(data: unknown): string {
  if (typeof data === "string") {
    return data;
  }

  if (data instanceof ArrayBuffer) {
    return new TextDecoder().decode(new Uint8Array(data));
  }

  if (typeof SharedArrayBuffer !== "undefined" && data instanceof SharedArrayBuffer) {
    return new TextDecoder().decode(new Uint8Array(data));
  }

  if (ArrayBuffer.isView(data)) {
    return new TextDecoder().decode(
      new Uint8Array(data.buffer, data.byteOffset, data.byteLength),
    );
  }

  return String(data ?? "");
}
