# AI Council SDK Server

This WebSocket server acts as the central orchestrator for the AI Council, enabling:
- Real-time streaming of agent outputs.
- Programmatic control of Claude Code instances via the `--sdk-url` flag.
- Multi-agent coordination (e.g., Debate Mode).
- A unified API for TUI clients.

## Architecture

The system consists of three main components:
1.  **SDK Server (this repo):** A Node.js WebSocket server that manages connections.
2.  **Agents (Claude Code):** Instances of the Claude Code CLI running in WebSocket mode.
3.  **Clients (TUI/Web):** Interfaces for users to interact with the council.

## Protocol Design

The server uses two distinct protocols over WebSocket:

### 1. Agent Protocol (NDJSON)
Agents connect to `ws://localhost:3000` (default). They communicate using Newline Delimited JSON (NDJSON).

**Message Types (Server -> Agent):**
- `user`: Represents a user prompt or instruction.
  ```json
  { "type": "user", "text": "Please analyze this code." }
  ```

**Message Types (Agent -> Server):**
- `system/init`: Sent by the agent upon connection.
- `assistant`: The final response from the agent.
- `stream_event`: Real-time token generation (if verbose).
- `tool_use`: When the agent wants to execute a tool.

### 2. Client Protocol (JSON)
Clients connect to `ws://localhost:3000?role=client`. They communicate using standard JSON.

**Message Types (Client -> Server):**
- `list_agents`: Request the current status of all connected agents.
- `submit_prompt`: Send a prompt to a specific agent.
  ```json
  { "type": "submit_prompt", "agentId": "<uuid>", "prompt": "Hello!" }
  ```
- `start_debate`: Initiate a debate between two agents.
  ```json
  { "type": "start_debate", "topic": "Rust vs Go", "agentA": "<uuid>", "agentB": "<uuid>" }
  ```
- `forward_to_agent`: Send a control message directly to an agent (e.g., set permissions).
  ```json
  { "type": "forward_to_agent", "agentId": "<uuid>", "payload": { "type": "set_permission_mode", "mode": "trusted" } }
  ```

**Message Types (Server -> Client):**
- `status_update`: Broadcasts the list of connected agents and active debates.
- `agent_event`: Forwards raw events from agents (e.g., streaming text).
- `debate_finished`: Notification that a debate has concluded.

## Setup & Usage

### 1. Start the Server
```bash
cd sdk-server
npm install
node server.js
```

### 2. Connect an Agent (Claude Code)
To connect a Claude Code instance to this server, use the undocumented `--sdk-url` flag:

```bash
claude --sdk-url ws://localhost:3000 --print --output-format stream-json --input-format stream-json
```
*Note: You may need to set `CLAUDE_CODE_SESSION_ACCESS_TOKEN` if authentication is required by your specific Claude Code build.*

### 3. Connect a Client (TUI)
The TUI should connect to `ws://localhost:3000?role=client`.
Send `{ "type": "list_agents" }` to discover available agents.

## Implementation Details

The server (`server.js`) handles:
- **Session Management:** Tracks active agents and clients using UUIDs.
- **Message Routing:** Routes prompts to specific agents and broadcasts responses to all clients.
- **Debate Logic:** Orchestrates a turn-based conversation between two agents by intercepting their responses and feeding them as prompts to the opponent.

## Future Work
- **Authentication:** Verify `Authorization` headers from agents.
- **Persistence:** Store conversation history in a database.
- **Advanced Orchestration:** Implement "Council Mode" where multiple agents vote on a decision.
