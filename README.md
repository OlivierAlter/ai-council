# AI Council WebSocket SDK

A WebSocket SDK protocol implementation for the AI Council that enables real-time streaming, programmatic tool permission handling, multi-turn conversations, and session resume with Claude Code.

## Overview

This implementation provides a WebSocket server that orchestrates AI agents using Claude Code's undocumented `--sdk-url` flag. The protocol is based on the reverse-engineered WebSocket SDK protocol from [The-Vibe-Company/companion](https://github.com/The-Vibe-Company/companion/blob/main/WEBSOCKET_PROTOCOL_REVERSED.md).

## Features

- **Real-time Streaming**: Token-by-token streaming via WebSocket
- **Programmatic Control**: Full control over tool permissions and agent behavior
- **Multi-turn Conversations**: Support for extended dialogs and follow-up questions
- **Session Management**: Resume, fork, and manage sessions
- **Debate Mode**: Multi-agent iterative review and consensus building
- **TUI Integration**: Real-time terminal updates and streaming
- **Advanced Permission Engine**: Policy-based tool permission handling

## Architecture

```
┌───────────────────────────────────────────────────────────────────────────────┐
│                        AI Council WebSocket Server                            │
├───────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  ┌─────────────┐    ┌─────────────┐    ┌───────────────────────────────────┐  │
│  │  WebSocket  │    │  Protocol   │    │  Session Manager               │  │
│  │  Endpoint   │───▶│  Handler    │───▶│  - Active sessions             │  │
│  └─────────────┘    └─────────────┘    │  - Session state                │  │
│          ▲               ▲               │  - Resume/fork support          │  │
│          │               │               └───────────────────────────────────┘  │
│          │               │                           ▲                          │
│  ┌───────┴───────┐       │                           │                          │
│  │  Permission   │       │                           │                          │
│  │  Engine       │◀──────┘                           │                          │
│  │  - Policy rules│                                    │                          │
│  │  - Auto-approve│                                    │                          │
│  │  - Deny rules  │                                    │                          │
│  └───────┬───────┘                                    │                          │
│          │                                          │                          │
│  ┌───────▼───────┐    ┌───────────────────────────┐  │                          │
│  │  Debate Mode  │    │  TUI Streaming Bridge    │  │                          │
│  │  Coordinator  │    │  - Real-time updates     │  │                          │
│  │  - Agent turns│    │  - Token streaming       │  │                          │
│  │  - Review cycles│  │  - Progress indicators   │  │                          │
│  │  - Consensus   │    └───────────────────────────┘  │                          │
│  │  building     │                                    │                          │
│  └───────────────┘                                    │                          │
│          ▲                                              │                          │
│          │                                              │                          │
│  ┌───────┴───────┐                                      │                          │
│  │  Agent        │                                      │                          │
│  │  Orchestrator │                                      │                          │
│  │  - Agent pool │                                      │                          │
│  │  - Load balancing│                                   │                          │
│  │  - Sub-agent  │                                      │                          │
│  │    management │                                      │                          │
│  └───────────────┘                                      │                          │
│                                                           │                          │
└───────────────────────────────────────────────────────────┼──────────────────────────┘
                                                            │
                                                            ▼
                                                  ┌─────────────────────┐
                                                  │  Claude Code CLI    │
                                                  │  (WebSocket Client) │
                                                  └─────────────────────┘
```

## Protocol

The protocol uses NDJSON (newline-delimited JSON) over WebSocket with the following message types:

### Core Message Types

- **`user`**: Prompts sent from server to Claude Code
- **`system/init`**: Initialization message from Claude Code
- **`assistant`**: LLM responses
- **`result`**: Query completion status
- **`stream_event`**: Real-time streaming tokens
- **`control_request`**: Permission and control messages
- **`control_response`**: Responses to control requests

### Control Protocol

The control protocol supports 13 subtypes including:

- **`can_use_tool`**: Tool permission requests (most important)
- **`initialize`**: Setup hooks, MCP servers, agents
- **`interrupt`**: Abort current agent turn
- **`set_permission_mode`**: Change permission mode at runtime
- **`set_model`**: Change model at runtime

## Installation

### Prerequisites

- [Bun](https://bun.sh/) (recommended) or Node.js
- Claude Code CLI with `--sdk-url` support

### Setup

```bash
# Install dependencies
bun install

# Start the server
bun start

# Or in development mode with auto-restart
bun dev
```

## Usage

### Basic Connection

```bash
claude --sdk-url ws://localhost:8765/ws \
       --print \
       --output-format stream-json \
       --input-format stream-json \
       --verbose \
       -p ""
```

### Session Resume

```bash
claude --sdk-url ws://localhost:8765/ws \
       --print \
       --output-format stream-json \
       --input-format stream-json \
       --resume <session-id> \
       -p ""
```

### Debate Mode

The debate mode enables multi-agent iterative review:

```typescript
import { EnhancedDebateModeCoordinator } from './debate-mode'

const debateCoordinator = new EnhancedDebateModeCoordinator(sessionManager)

const agents = [
  {
    id: 'agent-1',
    name: 'SecurityExpert',
    role: 'Security Architect',
    expertise: ['security', 'compliance', 'risk assessment']
  },
  {
    id: 'agent-2',
    name: 'DevOpsSpecialist',
    role: 'DevOps Engineer',
    expertise: ['CI/CD', 'infrastructure', 'scaling']
  }
]

const debate = await debateCoordinator.startDebate(
  'session-123',
  agents,
  'Design a secure and scalable architecture for our new SaaS platform'
)
```

### Permission Engine

The advanced permission engine supports policy-based decisions:

```typescript
import { AdvancedPermissionEngine } from './permission-engine'

const permissionEngine = new AdvancedPermissionEngine(sessionManager)

// Add a custom policy
permissionEngine.addPolicy({
  id: 'production-safety',
  name: 'Production Environment Safety',
  description: 'Extra restrictions for production environments',
  rules: [
    {
      id: 'no-production-writes',
      toolName: 'Write',
      pattern: '*production*',
      behavior: 'deny',
      scope: 'global',
      reason: 'No direct writes to production'
    }
  ],
  defaultBehavior: 'ask',
  appliesTo: 'all'
})
```

## API Endpoints

- **`GET /ws`**: WebSocket endpoint for Claude Code connections
- **`GET /health`**: Health check with active session count
- **`GET /static/*`**: Static files for any UI components

## Configuration

Environment variables:

- `PORT`: Server port (default: 8765)
- `MAX_SESSIONS`: Maximum concurrent sessions
- `DEBUG`: Enable debug logging

## Debate Mode Protocol

The debate mode implements a structured multi-agent workflow:

1. **Analysis Phase**: Each agent provides initial analysis
2. **Review Phase**: Agents review each other's work
3. **Refinement Phase**: Agents improve their analyses based on feedback
4. **Consensus Check**: Agents evaluate if consensus is reached

### Debate Turn Structure

```typescript
type DebateTurn = {
  agentId: string
  turnNumber: number
  action: 'analysis' | 'review' | 'refinement' | 'consensus_check'
  content: string
  timestamp: Date
  toolUses?: ToolUse[]
}
```

## TUI Streaming

Real-time terminal updates are handled by the `TUIStreamingBridge`:

```typescript
const tuiBridge = new TUIStreamingBridge(sessionManager, debateCoordinator)

// Start streaming to a TUI component
tuiBridge.startStreaming(sessionId, (data) => {
  // Update terminal display
  process.stdout.write(data)
})
```

## Permission Policies

The permission engine supports multiple policy types:

- **Global Rules**: Apply to all sessions
- **Session Rules**: Apply to specific sessions
- **Project Rules**: Apply to specific projects
- **Dynamic Policies**: Can be changed at runtime

### Example Policy

```json
{
  "id": "security-compliance",
  "name": "Security Compliance Policy",
  "description": "Ensure compliance with security standards",
  "rules": [
    {
      "id": "deny-dangerous-commands",
      "toolName": "Bash",
      "pattern": "*rm -rf*",
      "behavior": "deny",
      "scope": "global",
      "reason": "Prevent data loss"
    }
  ],
  "defaultBehavior": "ask",
  "appliesTo": "all"
}
```

## Development

### Running Tests

```bash
bun test
```

### Building

```bash
bun build
```

### Project Structure

```
.
├── websocket-server         # Main WebSocket server
├── debate-mode.ts           # Debate mode implementation
├── permission-engine.ts      # Advanced permission engine
├── package.json             # Project configuration
└── README.md                # Documentation
```

## Security

- All WebSocket connections require authentication via `Authorization` header
- Sensitive operations are logged in the audit trail
- Dangerous commands are automatically blocked by default policies

## Roadmap

- [x] Basic WebSocket server implementation
- [x] Protocol message handling
- [x] Session management
- [x] Debate mode coordination
- [x] TUI streaming integration
- [x] Advanced permission engine
- [ ] Persistent session storage
- [ ] Web-based monitoring dashboard
- [ ] Multi-agent orchestration
- [ ] Performance optimization

## Contributing

Contributions are welcome! Please open issues and pull requests.

## License

MIT
