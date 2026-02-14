import type { ToolPermissionDecision } from "../protocol/types";

export interface CanUseToolContext {
  sessionId: string;
  agentId: string;
  toolName: string;
  input: Record<string, unknown>;
}

export interface PermissionPolicy {
  evaluate(context: CanUseToolContext): ToolPermissionDecision;
}

const SAFE_DEFAULT_TOOLS = new Set([
  "Read",
  "Glob",
  "Grep",
  "LS",
  "WebFetch",
  "WebSearch",
  "TodoRead",
]);

const HIGH_RISK_BASH_PATTERNS: RegExp[] = [
  /(^|\s)rm\s+-rf\s+\/(\s|$)/,
  /(^|\s)shutdown(\s|$)/,
  /(^|\s)reboot(\s|$)/,
  /:\(\)\{\s*:\|:&\s*\};:/,
];

export class DefaultPermissionPolicy implements PermissionPolicy {
  evaluate(context: CanUseToolContext): ToolPermissionDecision {
    if (SAFE_DEFAULT_TOOLS.has(context.toolName)) {
      return {
        behavior: "allow",
        updatedInput: context.input,
      };
    }

    if (context.toolName === "Bash") {
      const command = String(context.input.command ?? "");
      for (const pattern of HIGH_RISK_BASH_PATTERNS) {
        if (pattern.test(command)) {
          return {
            behavior: "deny",
            message: `Blocked high-risk Bash command by policy: ${command}`,
            interrupt: false,
          };
        }
      }

      return {
        behavior: "allow",
        updatedInput: context.input,
      };
    }

    if (context.toolName === "Write" || context.toolName === "Edit" || context.toolName === "MultiEdit") {
      return {
        behavior: "allow",
        updatedInput: context.input,
      };
    }

    return {
      behavior: "deny",
      message: `Tool ${context.toolName} is not allowed by default policy`,
      interrupt: false,
    };
  }
}
