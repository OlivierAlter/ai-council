package sdk

// NDJSON message types for the Claude Code WebSocket SDK protocol.
// Reference: https://github.com/The-Vibe-Company/companion/blob/main/WEBSOCKET_PROTOCOL_REVERSED.md
//
// The Claude CLI is the WebSocket client, connecting to our server.
// Protocol is NDJSON (newline-delimited JSON).

// MessageType identifies the type of NDJSON message.
type MessageType string

const (
	// CLI -> Server
	MsgTypeSystemInit     MessageType = "system/init"
	MsgTypeAssistant      MessageType = "assistant"
	MsgTypeStreamEvent    MessageType = "stream_event"
	MsgTypeResult         MessageType = "result"
	MsgTypeControlRequest MessageType = "control_request"
	MsgTypeKeepAlive      MessageType = "keep_alive"

	// Server -> CLI
	MsgTypeUser            MessageType = "user"
	MsgTypeControlResponse MessageType = "control_response"
)

// Envelope is the top-level NDJSON message structure.
type Envelope struct {
	Type      MessageType    `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// ControlAction identifies the type of control request from the CLI.
type ControlAction string

const (
	ControlCanUseTool ControlAction = "can_use_tool"
)

// ControlRequest is sent by the CLI when it needs permission to use a tool.
type ControlRequest struct {
	RequestID string        `json:"request_id"`
	Action    ControlAction `json:"action"`
	Tool      string        `json:"tool"`
	Input     map[string]any `json:"input,omitempty"`
}

// ControlResponse is sent by the server to approve/deny a tool use request.
type ControlResponse struct {
	RequestID    string         `json:"request_id"`
	Allow        bool           `json:"allow"`
	UpdatedInput map[string]any `json:"updatedInput,omitempty"`
	Reason       string         `json:"reason,omitempty"`
}

// ResultData contains the outcome of a completed query.
type ResultData struct {
	Success   bool    `json:"success"`
	Result    string  `json:"result,omitempty"`
	Error     string  `json:"error,omitempty"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
	SessionID string  `json:"session_id,omitempty"`
}
