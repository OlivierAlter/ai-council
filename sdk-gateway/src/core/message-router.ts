import {
  isControlRequest,
  isControlResponse,
  isResult,
  isSystemInit,
  type AnySdkMessage,
  type SdkAssistantMessage,
  type SdkControlRequestMessage,
} from "../protocol/types";
import { DebateCoordinator } from "./debate-mode";
import { GatewayState } from "./gateway-state";
import type { PermissionPolicy } from "./permission-policy";

export class MessageRouter {
  constructor(
    private readonly state: GatewayState,
    private readonly permissions: PermissionPolicy,
    private readonly debates: DebateCoordinator,
  ) {}

  async routeAgentMessage(agentId: string, message: AnySdkMessage): Promise<void> {
    const agent = this.state.getAgent(agentId);
    if (!agent) {
      return;
    }

    const sessionId = agent.councilSessionId;
    this.state.markAgentMessageSeen(agentId);

    if (message.type !== "keep_alive") {
      this.state.emit(sessionId, "sdk.message", message, agentId);
    }

    if (message.type === "protocol_error") {
      this.state.emit(sessionId, "sdk.protocol_error", message, agentId);
      return;
    }

    if (isSystemInit(message)) {
      this.state.updateAgentInit(agentId, {
        sdkSessionId: message.session_id,
        model: typeof message.model === "string" ? message.model : undefined,
        tools: Array.isArray(message.tools) ? message.tools.map(String) : [],
      });

      this.state.emit(sessionId, "sdk.system_init", {
        sdkSessionId: message.session_id,
        model: message.model,
        tools: message.tools,
        permissionMode: message.permissionMode,
        claudeCodeVersion: message.claude_code_version,
      }, agentId);
      return;
    }

    if (isControlResponse(message)) {
      this.state.resolveControlResponse(agentId, message.response);
      return;
    }

    if (isControlRequest(message)) {
      await this.handleControlRequest(agentId, sessionId, message);
      return;
    }

    if (message.type === "assistant") {
      const text = extractAssistantText(message as SdkAssistantMessage);
      if (text) {
        this.state.emit(sessionId, "sdk.assistant_text", { text }, agentId);
      }
      return;
    }

    if (isResult(message)) {
      const resultText = typeof message.result === "string" ? message.result : "";
      this.state.emit(
        sessionId,
        "sdk.result",
        {
          subtype: message.subtype,
          isError: message.is_error,
          result: resultText,
          totalCostUSD: message.total_cost_usd,
          numTurns: message.num_turns,
        },
        agentId,
      );

      await this.debates.onAgentResult(sessionId, agentId, resultText);
      return;
    }

    if (message.type === "keep_alive") {
      this.state.sendRawToAgent(agentId, { type: "keep_alive" });
      return;
    }
  }

  private async handleControlRequest(
    agentId: string,
    sessionId: string,
    message: SdkControlRequestMessage,
  ): Promise<void> {
    const subtype = message.request.subtype;

    if (subtype === "can_use_tool") {
      const request = message.request as {
        tool_name: string;
        input: Record<string, unknown>;
      };
      const decision = this.permissions.evaluate({
        sessionId,
        agentId,
        toolName: request.tool_name,
        input: request.input,
      });

      this.state.sendRawToAgent(agentId, {
        type: "control_response",
        response: {
          subtype: "success",
          request_id: message.request_id,
          response: decision,
        },
      });

      this.state.emit(
        sessionId,
        "sdk.permission_decision",
        {
          requestId: message.request_id,
          toolName: request.tool_name,
          behavior: decision.behavior,
        },
        agentId,
      );
      return;
    }

    if (subtype === "hook_callback") {
      this.state.sendRawToAgent(agentId, {
        type: "control_response",
        response: {
          subtype: "success",
          request_id: message.request_id,
          response: {
            continue: true,
          },
        },
      });

      this.state.emit(
        sessionId,
        "sdk.hook_callback_ack",
        {
          callbackId: message.request.callback_id,
          requestId: message.request_id,
        },
        agentId,
      );
      return;
    }

    this.state.sendRawToAgent(agentId, {
      type: "control_response",
      response: {
        subtype: "error",
        request_id: message.request_id,
        error: `Unsupported control request subtype: ${subtype}`,
      },
    });

    this.state.emit(
      sessionId,
      "sdk.unsupported_control_request",
      {
        requestId: message.request_id,
        subtype,
      },
      agentId,
    );
  }
}

function extractAssistantText(msg: SdkAssistantMessage): string {
  const blocks = msg.message?.content;
  if (!Array.isArray(blocks)) {
    return "";
  }

  return blocks
    .filter((block) => block.type === "text")
    .map((block) => String(block.text ?? ""))
    .join("\n")
    .trim();
}
