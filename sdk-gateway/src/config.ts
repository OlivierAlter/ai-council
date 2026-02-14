export interface GatewayConfig {
  host: string;
  port: number;
  publicWsBaseUrl: string;
  authBearerToken?: string;
  claudeBinary: string;
  defaultModel?: string;
  defaultPermissionMode: string;
  defaultAppendSystemPrompt?: string;
  maxControlWaitMs: number;
  maxEventBacklog: number;
  defaultDebateRounds: number;
}

function parseIntWithDefault(raw: string | undefined, fallback: number): number {
  if (!raw) {
    return fallback;
  }

  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

export function loadConfig(): GatewayConfig {
  const host = process.env.HOST ?? "0.0.0.0";
  const port = parseIntWithDefault(process.env.PORT, 8765);

  return {
    host,
    port,
    publicWsBaseUrl: process.env.PUBLIC_WS_BASE_URL ?? `ws://localhost:${port}`,
    authBearerToken: process.env.SDK_GATEWAY_BEARER_TOKEN,
    claudeBinary: process.env.CLAUDE_BINARY ?? "claude",
    defaultModel: process.env.CLAUDE_DEFAULT_MODEL,
    defaultPermissionMode: process.env.CLAUDE_PERMISSION_MODE ?? "default",
    defaultAppendSystemPrompt: process.env.CLAUDE_APPEND_SYSTEM_PROMPT,
    maxControlWaitMs: parseIntWithDefault(process.env.MAX_CONTROL_WAIT_MS, 30000),
    maxEventBacklog: parseIntWithDefault(process.env.MAX_EVENT_BACKLOG, 3000),
    defaultDebateRounds: parseIntWithDefault(process.env.DEFAULT_DEBATE_ROUNDS, 3),
  };
}
