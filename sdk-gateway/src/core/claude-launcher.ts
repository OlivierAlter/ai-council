import type { GatewayConfig } from "../config";
import { GatewayState } from "./gateway-state";

export interface LaunchClaudeAgentInput {
  sessionId: string;
  agentId: string;
  model?: string;
  resumeSessionId?: string;
  maxTurns?: number;
}

export class ClaudeLauncher {
  private readonly processes = new Map<string, Bun.Subprocess>();

  constructor(
    private readonly state: GatewayState,
    private readonly config: GatewayConfig,
  ) {}

  launch(input: LaunchClaudeAgentInput): { pid: number; cmd: string[] } {
    const sdkUrl = `${this.config.publicWsBaseUrl}/ws/agent/${encodeURIComponent(input.agentId)}?sessionId=${encodeURIComponent(
      input.sessionId,
    )}`;

    const cmd: string[] = [
      this.config.claudeBinary,
      "--sdk-url",
      sdkUrl,
      "--print",
      "--output-format",
      "stream-json",
      "--input-format",
      "stream-json",
      "--verbose",
      "-p",
      "",
    ];

    if (input.model ?? this.config.defaultModel) {
      cmd.push("--model", input.model ?? this.config.defaultModel ?? "");
    }

    if (input.resumeSessionId) {
      cmd.push("--resume", input.resumeSessionId);
    }

    if (typeof input.maxTurns === "number" && Number.isFinite(input.maxTurns)) {
      cmd.push("--max-turns", String(input.maxTurns));
    }

    const proc = Bun.spawn({
      cmd,
      env: {
        ...process.env,
      },
      stdout: "pipe",
      stderr: "pipe",
    });

    this.processes.set(input.agentId, proc);

    this.state.emit(input.sessionId, "agent.process.spawned", {
      agentId: input.agentId,
      pid: proc.pid,
      cmd,
      sdkUrl,
    });

    void this.pumpStream(input.sessionId, input.agentId, proc.stdout, "stdout");
    void this.pumpStream(input.sessionId, input.agentId, proc.stderr, "stderr");
    void this.watchExit(input.sessionId, input.agentId, proc);

    return {
      pid: proc.pid,
      cmd,
    };
  }

  stop(agentId: string): boolean {
    const proc = this.processes.get(agentId);
    if (!proc) {
      return false;
    }

    try {
      proc.kill();
      return true;
    } catch {
      return false;
    }
  }

  private async watchExit(sessionId: string, agentId: string, proc: Bun.Subprocess): Promise<void> {
    const exitCode = await proc.exited;
    this.processes.delete(agentId);

    this.state.emit(sessionId, "agent.process.exited", {
      agentId,
      pid: proc.pid,
      exitCode,
    });
  }

  private async pumpStream(
    sessionId: string,
    agentId: string,
    stream: ReadableStream<Uint8Array> | null,
    streamName: "stdout" | "stderr",
  ): Promise<void> {
    if (!stream) {
      return;
    }

    const reader = stream.getReader();
    const decoder = new TextDecoder();
    let buffered = "";

    for (;;) {
      const { value, done } = await reader.read();
      if (done) {
        break;
      }

      buffered += decoder.decode(value, { stream: true });

      for (;;) {
        const nl = buffered.indexOf("\n");
        if (nl < 0) {
          break;
        }

        const line = buffered.slice(0, nl).trimEnd();
        buffered = buffered.slice(nl + 1);

        if (!line) {
          continue;
        }

        this.state.emit(
          sessionId,
          "agent.process.output",
          {
            agentId,
            stream: streamName,
            line,
          },
          agentId,
        );
      }
    }
  }
}
