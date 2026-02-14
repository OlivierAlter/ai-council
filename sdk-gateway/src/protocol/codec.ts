import type { AnySdkMessage } from "./types";

export class NDJSONFramer {
  private buffer = "";

  push(chunk: string): AnySdkMessage[] {
    const messages: AnySdkMessage[] = [];
    this.buffer += chunk;

    for (;;) {
      const idx = this.buffer.indexOf("\n");
      if (idx < 0) {
        break;
      }

      const line = this.buffer.slice(0, idx).trim();
      this.buffer = this.buffer.slice(idx + 1);
      if (!line) {
        continue;
      }

      try {
        const parsed = JSON.parse(line) as AnySdkMessage;
        messages.push(parsed);
      } catch {
        messages.push({
          type: "protocol_error",
          raw: line,
        });
      }
    }

    return messages;
  }
}

export function encodeNDJSON(message: unknown): string {
  return `${JSON.stringify(message)}\n`;
}
