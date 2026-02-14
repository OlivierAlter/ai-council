export type PermissionMode =
  | "default"
  | "acceptEdits"
  | "bypassPermissions"
  | "plan"
  | "delegate"
  | "dontAsk";

export type ToolPermissionDecision =
  | {
      behavior: "allow";
      updatedInput: Record<string, unknown>;
      updatedPermissions?: unknown[];
      toolUseID?: string;
    }
  | {
      behavior: "deny";
      message: string;
      interrupt?: boolean;
      toolUseID?: string;
    };

export type ControlRequestPayload =
  | {
      subtype: "initialize";
      systemPrompt?: string;
      appendSystemPrompt?: string;
      hooks?: Record<string, unknown>;
      sdkMcpServers?: string[];
      agents?: Record<string, unknown>;
      jsonSchema?: Record<string, unknown>;
    }
  | { subtype: "interrupt" }
  | { subtype: "set_permission_mode"; mode: PermissionMode }
  | { subtype: "set_model"; model?: string }
  | { subtype: "set_max_thinking_tokens"; max_thinking_tokens: number | null }
  | { subtype: "mcp_status" }
  | { subtype: "mcp_message"; server_name: string; message: unknown }
  | { subtype: "mcp_reconnect"; serverName: string }
  | { subtype: "mcp_toggle"; serverName: string; enabled: boolean }
  | {
      subtype: "mcp_set_servers";
      servers: Record<
        string,
        {
          type: "stdio" | "sse" | "http" | "sdk";
          command?: string;
          args?: string[];
          env?: Record<string, string>;
          url?: string;
        }
      >;
    }
  | { subtype: "rewind_files"; user_message_id: string; dry_run?: boolean }
  | {
      subtype: "can_use_tool";
      tool_name: string;
      input: Record<string, unknown>;
      permission_suggestions?: unknown[];
      blocked_path?: string;
      decision_reason?: string;
      tool_use_id: string;
      agent_id?: string;
      description?: string;
    }
  | {
      subtype: "hook_callback";
      callback_id: string;
      input: unknown;
      tool_use_id?: string;
    }
  | {
      subtype: string;
      [key: string]: unknown;
    };

export type SdkControlRequestMessage = {
  type: "control_request";
  request_id: string;
  request: ControlRequestPayload;
};

export type SdkControlResponseMessage = {
  type: "control_response";
  response:
    | {
        subtype: "success";
        request_id: string;
        response?: Record<string, unknown>;
      }
    | {
        subtype: "error";
        request_id: string;
        error: string;
        pending_permission_requests?: unknown[];
      };
};

export type SdkUserMessage = {
  type: "user";
  message: {
    role: "user";
    content: string | Array<Record<string, unknown>>;
  };
  parent_tool_use_id: string | null;
  session_id: string;
  uuid?: string;
  isSynthetic?: boolean;
};

export type SdkMessageBase = {
  type: string;
  session_id?: string;
  uuid?: string;
  [key: string]: unknown;
};

export type SdkSystemInitMessage = SdkMessageBase & {
  type: "system";
  subtype: "init";
  session_id: string;
  model?: string;
  tools?: string[];
  permissionMode?: PermissionMode;
  claude_code_version?: string;
};

export type SdkAssistantMessage = SdkMessageBase & {
  type: "assistant";
  message?: {
    content?: Array<{
      type?: string;
      text?: string;
      [key: string]: unknown;
    }>;
    [key: string]: unknown;
  };
};

export type SdkResultMessage = SdkMessageBase & {
  type: "result";
  subtype?: string;
  is_error?: boolean;
  result?: string;
  total_cost_usd?: number;
  num_turns?: number;
};

export type AnySdkMessage =
  | SdkControlRequestMessage
  | SdkControlResponseMessage
  | SdkUserMessage
  | SdkSystemInitMessage
  | SdkAssistantMessage
  | SdkResultMessage
  | SdkMessageBase;

export function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

export function isControlRequest(msg: AnySdkMessage): msg is SdkControlRequestMessage {
  return msg.type === "control_request" && isObject((msg as Record<string, unknown>).request);
}

export function isControlResponse(msg: AnySdkMessage): msg is SdkControlResponseMessage {
  return msg.type === "control_response" && isObject((msg as Record<string, unknown>).response);
}

export function isSystemInit(msg: AnySdkMessage): msg is SdkSystemInitMessage {
  return msg.type === "system" && (msg as Record<string, unknown>).subtype === "init";
}

export function isResult(msg: AnySdkMessage): msg is SdkResultMessage {
  return msg.type === "result";
}
