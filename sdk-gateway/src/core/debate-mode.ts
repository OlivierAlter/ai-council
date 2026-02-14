import { GatewayState } from "./gateway-state";

export interface DebateRuntime {
  active: boolean;
  rootPrompt: string;
  round: number;
  maxRounds: number;
  agentIds: string[];
  awaiting: Set<string>;
  outputsByRound: Map<number, Map<string, string>>;
}

export class DebateCoordinator {
  constructor(private readonly state: GatewayState) {}

  start(sessionId: string, rootPrompt: string, agentIds: string[], maxRounds: number): void {
    if (agentIds.length < 2) {
      throw new Error("Debate mode requires at least 2 agents");
    }

    this.state.setSessionMode(sessionId, "debate");

    const runtime: DebateRuntime = {
      active: true,
      rootPrompt,
      round: 1,
      maxRounds,
      agentIds,
      awaiting: new Set(agentIds),
      outputsByRound: new Map<number, Map<string, string>>(),
    };

    this.state.setDebateRuntime(sessionId, runtime);

    for (const agentId of agentIds) {
      const delivered = this.state.sendUserMessage(agentId, rootPrompt);
      this.state.emit(
        sessionId,
        "debate.round_dispatched",
        {
          round: 1,
          delivered,
          prompt: rootPrompt,
        },
        agentId,
      );
    }

    this.state.emit(sessionId, "debate.started", {
      maxRounds,
      agentIds,
      rootPrompt,
    });
  }

  async onAgentResult(sessionId: string, agentId: string, resultText: string): Promise<void> {
    const runtime = this.state.getDebateRuntime<DebateRuntime>(sessionId);
    if (!runtime || !runtime.active) {
      return;
    }

    if (!runtime.agentIds.includes(agentId)) {
      return;
    }

    let outputs = runtime.outputsByRound.get(runtime.round);
    if (!outputs) {
      outputs = new Map<string, string>();
      runtime.outputsByRound.set(runtime.round, outputs);
    }
    outputs.set(agentId, resultText);
    runtime.awaiting.delete(agentId);

    if (runtime.awaiting.size > 0) {
      this.state.emit(sessionId, "debate.waiting", {
        round: runtime.round,
        waitingFor: [...runtime.awaiting],
      });
      return;
    }

    if (runtime.round >= runtime.maxRounds) {
      runtime.active = false;
      this.state.emit(sessionId, "debate.completed", {
        totalRounds: runtime.round,
        outputs: Object.fromEntries(
          [...runtime.outputsByRound.entries()].map(([round, byAgent]) => [round, Object.fromEntries(byAgent)]),
        ),
      });
      this.state.clearDebateRuntime(sessionId);
      this.state.setSessionMode(sessionId, "standard");
      return;
    }

    const priorOutputs = runtime.outputsByRound.get(runtime.round) ?? new Map<string, string>();
    runtime.round += 1;
    runtime.awaiting = new Set(runtime.agentIds);

    for (const targetAgent of runtime.agentIds) {
      const prompt = buildDebatePrompt(runtime.rootPrompt, runtime.round, targetAgent, priorOutputs);
      const delivered = this.state.sendUserMessage(targetAgent, prompt);
      this.state.emit(
        sessionId,
        "debate.round_dispatched",
        {
          round: runtime.round,
          delivered,
          prompt,
        },
        targetAgent,
      );
    }

    this.state.emit(sessionId, "debate.round_started", {
      round: runtime.round,
      agentIds: runtime.agentIds,
    });
  }
}

function buildDebatePrompt(
  rootPrompt: string,
  round: number,
  targetAgent: string,
  priorOutputs: Map<string, string>,
): string {
  const peers = [...priorOutputs.entries()]
    .filter(([agentId]) => agentId !== targetAgent)
    .map(([agentId, output]) => {
      const clipped = output.length > 6000 ? `${output.slice(0, 6000)}\n...[truncated]` : output;
      return `Peer ${agentId} output:\n${clipped}`;
    })
    .join("\n\n");

  return [
    `Debate Mode Round ${round}`,
    `Original task:\n${rootPrompt}`,
    "Review peer outputs, identify weaknesses, and produce a stronger revised answer.",
    "Explicitly state what you changed and why.",
    peers || "No peer outputs available.",
  ].join("\n\n");
}
