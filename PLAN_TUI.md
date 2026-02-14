# AI Council TUI — Bubbletea Implementation Plan

Turn the AI Council Bash orchestrator into a real-time Terminal User Interface using [Bubbletea](https://github.com/charmbracelet/bubbletea) (Go, Elm architecture). This plan was designed by the AI Council itself (all 4 agents unanimous on Bubbletea).

## Strategy: Hybrid → Pure Go

**Phase 1 (MVP)**: Keep existing Bash scripts. Add `--tui-mode` flag that emits JSONL events to stdout. Go TUI consumes the event stream via `os/exec` pipe. Lower risk — auth and container lifecycle stay in battle-tested Bash.

**Phase 2**: Pure Go rewrite — port container ops, auth, process management to Go for first-class streaming and type safety. Keep `council.sh` as headless/CI mode (`--no-tui`).

**Rationale**: The Bash orchestrator has hard-won platform fixes (Keychain extraction, Apple container quirks, bash 3.2 compat). Wrapping it first validates the TUI UX without risking the orchestration layer.

## Charm Library Stack

| Library | Purpose |
|---------|---------|
| `bubbletea` | Core TUI framework (Elm architecture) |
| `lipgloss` | Styling, layout, borders, colors |
| `bubbles/viewport` | Scrollable agent output panels |
| `bubbles/textarea` | Multi-line prompt input |
| `bubbles/spinner` | Loading indicators per agent |
| `bubbles/progress` | Timeout countdown bars |
| `glamour` | Markdown rendering for synthesis output |
| `bubblezone` (optional) | Mouse click support for panels |

**Alternatives considered (all agents agree Bubbletea wins)**:
- `tview`: More widget-based, less aesthetic flexibility
- `termui`: Good for dashboards/charts, weaker for text streaming
- `gocui`: Too low-level for this use case
- `Textual` (Python): Strong UX but adds a second runtime

## UI Layout

```
┌──────────────────────────────────────────────────────────┐
│  AI Council                           Session: abc-123   │
├────────────────────────┬─────────────────────────────────┤
│  ◉ Claude   running    │  ✓ Codex    completed           │
│  12.4s                 │  8.2s | 1.2k tokens | $0.0031  │
│  > Analyzing code...   │  > {"result": "...truncated"}   │
│  > Found 3 patterns    │                                 │
│                        │                                 │
├────────────────────────┼─────────────────────────────────┤
│  ◉ Gemini   running    │  ✗ Vibe     failed              │
│  15.1s                 │  Error: auth token expired      │
│  > Processing prompt   │                                 │
│  > Generating...       │                                 │
│                        │                                 │
├────────────────────────┴─────────────────────────────────┤
│  [Overview] [Claude] [Codex] [Gemini] [Vibe] [Synthesis] │
├──────────────────────────────────────────────────────────┤
│  Session: abc-123 | "Design a TUI..."  ✓2 ⏱2 ✗0  $0.12 │
│  Tab: panels | 1-6: jump | r: new prompt | q: quit      │
└──────────────────────────────────────────────────────────┘
```

### Views

- **Overview** (default): 2x2 grid of agent cards with status, elapsed time, output preview
- **Agent Detail** (1-4): Full scrollable viewport for a single agent's output
- **Synthesis** (5/s): Dedicated panel for the synthesized result with markdown rendering

### Keyboard Navigation

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle between panels |
| `1`-`4` | Jump to agent detail view (Claude/Codex/Gemini/Vibe) |
| `5` or `s` | Jump to synthesis panel |
| `0` | Back to overview grid |
| `Enter` (Alt+Enter) | Submit prompt |
| `j`/`k` or arrows | Scroll within focused viewport |
| `r` | New prompt (when session complete) |
| `q` / `Ctrl+C` | Quit |

### Agent Colors

| Agent | Color | Hex |
|-------|-------|-----|
| Claude | Warm red | `#E57373` |
| Codex | Green | `#81C784` |
| Gemini | Blue | `#64B5F6` |
| Vibe | Yellow | `#FFD54F` |

### Status Icons

| Status | Icon |
|--------|------|
| Pending | `○` |
| Running | `◉` |
| Completed | `✓` |
| Failed | `✗` |
| Timeout | `⏱` |
| Canceled | `⊘` |

## Project Structure

```
ai-council/
├── cmd/
│   └── council-tui/
│       └── main.go              # Entry point, program init
├── internal/
│   ├── tui/
│   │   ├── model.go             # Root Bubbletea Model
│   │   ├── update.go            # Update function + key handling
│   │   ├── view.go              # View function + layout
│   │   ├── messages.go          # All tea.Msg types
│   │   └── styles.go            # Lipgloss styles
│   ├── orchestrator/
│   │   ├── supervisor.go        # Agent lifecycle, timeout watchdog
│   │   ├── executor.go          # Spawns council.sh (Phase 1) or containers (Phase 2)
│   │   ├── parser.go            # JSONL event parsing
│   │   └── bus.go               # Fan-in event channel
│   ├── agents/
│   │   ├── adapter.go           # AgentAdapter interface
│   │   ├── claude.go            # Claude command builder + parser
│   │   ├── codex.go             # Codex JSONL stream parser
│   │   ├── gemini.go            # Gemini command builder + parser
│   │   └── vibe.go              # Vibe command builder + parser
│   ├── components/
│   │   ├── agent_panel.go       # Per-agent status card
│   │   ├── prompt_input.go      # Prompt textarea wrapper
│   │   ├── synthesis_panel.go   # Synthesis result display
│   │   ├── statusbar.go         # Bottom status bar
│   │   └── tabs.go              # Tab/panel navigation
│   └── types/
│       ├── events.go            # Event type definitions
│       └── state.go             # Application state
├── council.sh                   # Existing Bash orchestrator (headless/CI mode)
├── lib/                         # Existing Bash libraries
├── config/
│   └── council.conf
└── go.mod
```

## Core Go Types

### State (`internal/types/state.go`)

```go
package types

import "time"

type AgentID string

const (
    AgentClaude AgentID = "claude"
    AgentCodex  AgentID = "codex"
    AgentGemini AgentID = "gemini"
    AgentVibe   AgentID = "vibe"
)

var AgentOrder = []AgentID{AgentClaude, AgentCodex, AgentGemini, AgentVibe}

type AgentStatus int

const (
    StatusPending AgentStatus = iota
    StatusRunning
    StatusCompleted
    StatusFailed
    StatusTimeout
    StatusCanceled
)

func (s AgentStatus) String() string {
    return [...]string{"pending", "running", "completed", "failed", "timeout", "canceled"}[s]
}

type TokenUsage struct {
    InputTokens  int     `json:"input_tokens"`
    OutputTokens int     `json:"output_tokens"`
    CostUSD      float64 `json:"cost_usd"`
}

type OutputLine struct {
    Text      string
    Stream    string // "stdout" or "stderr"
    Timestamp time.Time
}

type AgentState struct {
    ID        AgentID
    Status    AgentStatus
    StartTime time.Time
    EndTime   *time.Time
    Output    []OutputLine  // Full log
    Stream    []string      // Ring buffer for UI (last N lines)
    FinalJSON string        // Parsed final result
    Error     string
    Usage     *TokenUsage
}

type SynthesisResult struct {
    Summary     string            `json:"summary"`
    Approach    string            `json:"approach"`
    Confidence  string            `json:"confidence"`
    Consensus   []string          `json:"consensus_areas"`
    Divergences []string          `json:"divergence_areas"`
    Warnings    []string          `json:"warnings"`
    AgentEvals  map[string]string `json:"agents"`
}

type SessionState struct {
    ID              string
    Prompt          string
    StartTime       time.Time
    EndTime         *time.Time
    Timeout         time.Duration
    Agents          map[AgentID]*AgentState
    SynthesisResult *SynthesisResult
    SynthesisActive bool
}
```

### Messages (`internal/tui/messages.go`)

```go
package tui

import (
    "time"
    "ai-council/internal/types"
)

// Agent lifecycle messages
type agentStartedMsg struct {
    agent types.AgentID
    time  time.Time
}

type agentOutputMsg struct {
    agent  types.AgentID
    line   string
    stream string // stdout/stderr
}

type agentCompletedMsg struct {
    agent    types.AgentID
    status   types.AgentStatus
    duration time.Duration
    usage    *types.TokenUsage
}

// Synthesis messages
type synthesisStartedMsg struct{}
type synthesisChunkMsg struct{ text string }
type synthesisCompleteMsg struct{ result *types.SynthesisResult }

// System messages
type tickMsg time.Time
type errorMsg struct{ err error }
```

### Model (`internal/tui/model.go`)

```go
package tui

import (
    "github.com/charmbracelet/bubbles/textarea"
    "github.com/charmbracelet/bubbles/viewport"
    "github.com/charmbracelet/bubbles/spinner"
    tea "github.com/charmbracelet/bubbletea"
    "ai-council/internal/types"
)

type ViewMode int
const (
    ModePrompt   ViewMode = iota // Entering prompt
    ModeRunning                   // Agents executing
    ModeComplete                  // Session finished
)

type FocusedPanel int
const (
    PanelOverview  FocusedPanel = iota // 2x2 grid
    PanelClaude
    PanelCodex
    PanelGemini
    PanelVibe
    PanelSynthesis
)

type Model struct {
    // UI state
    mode         ViewMode
    focusedPanel FocusedPanel
    width, height int

    // Components
    promptInput    textarea.Model
    agentViewports map[types.AgentID]*viewport.Model
    synthViewport  viewport.Model
    spinners       map[types.AgentID]spinner.Model

    // Session data
    session *types.SessionState

    // Event bus from orchestrator
    eventChan <-chan tea.Msg

    // Config
    config Config
}

type Config struct {
    Agents      []types.AgentID
    Timeout     int
    Workspace   string
    NoSynthesis bool
    ScriptPath  string // Path to council.sh (Phase 1)
}
```

## Update Function (`internal/tui/update.go`)

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.resizeComponents()
        return m, nil

    case tea.KeyMsg:
        return m.handleKeyPress(msg)

    case tickMsg:
        return m, m.tickCmd()

    case agentStartedMsg:
        agent := m.session.Agents[msg.agent]
        agent.Status = types.StatusRunning
        agent.StartTime = msg.time
        return m, waitForEvent(m.eventChan)

    case agentOutputMsg:
        agent := m.session.Agents[msg.agent]
        agent.Output = append(agent.Output, types.OutputLine{
            Text: msg.line, Stream: msg.stream, Timestamp: time.Now(),
        })
        // Ring buffer for viewport
        agent.Stream = append(agent.Stream, msg.line)
        if len(agent.Stream) > 500 {
            agent.Stream = agent.Stream[len(agent.Stream)-500:]
        }
        if vp, ok := m.agentViewports[msg.agent]; ok {
            vp.SetContent(strings.Join(agent.Stream, "\n"))
            vp.GotoBottom()
        }
        return m, waitForEvent(m.eventChan)

    case agentCompletedMsg:
        agent := m.session.Agents[msg.agent]
        agent.Status = msg.status
        agent.Usage = msg.usage
        now := time.Now()
        agent.EndTime = &now
        if m.allAgentsDone() && !m.config.NoSynthesis {
            return m, tea.Batch(
                waitForEvent(m.eventChan),
                m.startSynthesisCmd(),
            )
        }
        return m, waitForEvent(m.eventChan)

    case synthesisCompleteMsg:
        m.session.SynthesisResult = msg.result
        m.session.SynthesisActive = false
        m.mode = ModeComplete
        m.focusedPanel = PanelSynthesis
        return m, nil
    }

    // Forward to focused component
    if m.mode == ModePrompt {
        var cmd tea.Cmd
        m.promptInput, cmd = m.promptInput.Update(msg)
        return m, cmd
    }
    return m, nil
}

// waitForEvent is the standard Bubbletea pattern for consuming channel events
func waitForEvent(ch <-chan tea.Msg) tea.Cmd {
    return func() tea.Msg {
        return <-ch
    }
}
```

## Key Press Handling

```go
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch m.mode {
    case ModePrompt:
        switch msg.Type {
        case tea.KeyCtrlC, tea.KeyEsc:
            return m, tea.Quit
        case tea.KeyEnter:
            if msg.Alt {
                return m.submitPrompt()
            }
            var cmd tea.Cmd
            m.promptInput, cmd = m.promptInput.Update(msg)
            return m, cmd
        }

    case ModeRunning, ModeComplete:
        switch msg.String() {
        case "ctrl+c":
            return m, tea.Quit
        case "tab":
            m.focusedPanel = (m.focusedPanel + 1) % 6
        case "shift+tab":
            m.focusedPanel = (m.focusedPanel - 1 + 6) % 6
        case "1": m.focusedPanel = PanelClaude
        case "2": m.focusedPanel = PanelCodex
        case "3": m.focusedPanel = PanelGemini
        case "4": m.focusedPanel = PanelVibe
        case "5", "s": m.focusedPanel = PanelSynthesis
        case "0": m.focusedPanel = PanelOverview
        case "r":
            if m.mode == ModeComplete { return m.resetToPrompt() }
        case "q":
            if m.mode == ModeComplete { return m, tea.Quit }
        }

        if vp := m.getFocusedViewport(); vp != nil {
            var cmd tea.Cmd
            *vp, cmd = vp.Update(msg)
            return m, cmd
        }
    }
    return m, nil
}
```

## Lipgloss Styles (`internal/tui/styles.go`)

```go
var (
    colorClaude  = lipgloss.Color("#E57373") // Warm red
    colorCodex   = lipgloss.Color("#81C784") // Green
    colorGemini  = lipgloss.Color("#64B5F6") // Blue
    colorVibe    = lipgloss.Color("#FFD54F") // Yellow

    colorSuccess = lipgloss.Color("#4CAF50")
    colorError   = lipgloss.Color("#F44336")
    colorWarning = lipgloss.Color("#FF9800")
    colorDim     = lipgloss.Color("#757575")

    titleStyle = lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("#FFFFFF")).
        Background(lipgloss.Color("#7D56F4")).
        Padding(0, 1)

    dimStyle  = lipgloss.NewStyle().Foreground(colorDim)
    helpStyle = lipgloss.NewStyle().Foreground(colorDim).Italic(true)
)

func agentColor(id types.AgentID) lipgloss.Color {
    switch id {
    case types.AgentClaude:  return colorClaude
    case types.AgentCodex:   return colorCodex
    case types.AgentGemini:  return colorGemini
    case types.AgentVibe:    return colorVibe
    default:                 return lipgloss.Color("#FFFFFF")
    }
}
```

## Phase 1: Bash `--tui-mode` Integration

### JSONL Event Protocol

Add `--tui-mode` to `council.sh`. When active, emit JSONL events to stdout:

```jsonl
{"type":"agent.started","ts":"2026-02-14T11:00:00Z","data":{"agent":"claude"}}
{"type":"agent.output","ts":"2026-02-14T11:00:01Z","data":{"agent":"claude","line":"Analyzing code...","stream":"stdout"}}
{"type":"agent.completed","ts":"2026-02-14T11:00:12Z","data":{"agent":"claude","status":"success","usage":{"input_tokens":3,"output_tokens":13,"cost_usd":0.0124}}}
{"type":"synthesis.started","ts":"2026-02-14T11:00:15Z","data":{}}
{"type":"synthesis.chunk","ts":"2026-02-14T11:00:16Z","data":{"text":"Analyzing agent outputs..."}}
{"type":"synthesis.completed","ts":"2026-02-14T11:00:30Z","data":{"result":{...}}}
```

### Bash Modifications

```bash
# Add to council.sh arg parsing
TUI_MODE=false
case "$1" in --tui-mode) TUI_MODE=true; shift ;; esac

emit_event() {
    local type="$1" data="$2"
    [ "$TUI_MODE" = true ] && \
        printf '{"type":"%s","ts":"%s","data":%s}\n' \
            "$type" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$data"
}

# In agent launch:
emit_event "agent.started" '{"agent":"'"$agent"'"}'

# Stream each line:
container run ... 2>&1 | while IFS= read -r line; do
    echo "$line" >> "$output_file"
    emit_event "agent.output" '{"agent":"'"$agent"'","line":'"$(jq -Rn --arg l "$line" '$l')"'}'
done

# On completion:
emit_event "agent.completed" '{"agent":"'"$agent"'","status":"'"$status"'"}'
```

### Go Executor (Phase 1)

```go
// internal/orchestrator/executor.go
type Executor struct {
    scriptPath string
    eventChan  chan tea.Msg
}

func NewExecutor(scriptPath string) *Executor {
    return &Executor{
        scriptPath: scriptPath,
        eventChan:  make(chan tea.Msg, 256),
    }
}

func (e *Executor) Run(ctx context.Context, prompt string, cfg Config) error {
    args := []string{prompt, "--tui-mode"}
    if cfg.Timeout > 0 {
        args = append(args, "--timeout", strconv.Itoa(cfg.Timeout))
    }
    if cfg.Workspace != "" {
        args = append(args, "--workspace", cfg.Workspace)
    }

    cmd := exec.CommandContext(ctx, e.scriptPath, args...)
    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()

    if err := cmd.Start(); err != nil {
        return err
    }

    // Parse JSONL events from stdout
    go func() {
        scanner := bufio.NewScanner(stdout)
        scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
        for scanner.Scan() {
            if msg := parseEvent(scanner.Text()); msg != nil {
                e.eventChan <- msg
            }
        }
    }()

    // Capture stderr as error events
    go func() {
        scanner := bufio.NewScanner(stderr)
        for scanner.Scan() {
            e.eventChan <- errorMsg{err: fmt.Errorf("%s", scanner.Text())}
        }
    }()

    return cmd.Wait()
}

func (e *Executor) Events() <-chan tea.Msg {
    return e.eventChan
}
```

## Phase 2: Agent Adapter Interface (Pure Go)

```go
// internal/agents/adapter.go
type AgentAdapter interface {
    ID() types.AgentID
    BuildCommand(prompt string, cfg Config) *exec.Cmd
    ParseLine(line string) []tea.Msg
    ExtractResult(logPath string) (*AgentResult, error)
}
```

Per-agent parsers handle format differences:
- **Claude/Gemini/Vibe**: Extract last valid JSON object from stdout
- **Codex**: Parse JSONL event stream, extract `item.completed` with `agent_message` type

## Failure Handling in UI

- Per-agent status badge changes color: red for failed, orange for timeout
- Error message shown in agent's panel
- Synthesis still runs with partial results, warns about missing agents
- Status bar countdown shows remaining timeout per agent
- Full raw logs persisted to `logs/<session>/<agent>.log`

## Data Flow

```
council.sh --tui-mode
    │
    │  JSONL events (stdout)
    ▼
Go Executor (goroutine)
    │
    │  bufio.Scanner → parseEvent()
    ▼
eventChan (chan tea.Msg, buffered 256)
    │
    │  waitForEvent() tea.Cmd
    ▼
Bubbletea Update loop
    │
    │  Model mutations
    ▼
Bubbletea View()
    │
    │  lipgloss rendering
    ▼
Terminal output
```

## Build Plan

| Phase | Scope | Effort |
|-------|-------|--------|
| **1a** | Go module init, types, styles, empty TUI shell | Small |
| **1b** | Bash `--tui-mode` JSONL events, Go JSONL parser | Medium |
| **1c** | Prompt input → launch → overview grid with live status | Medium |
| **1d** | Agent detail viewports with streaming output | Medium |
| **1e** | Synthesis panel with markdown rendering (glamour) | Small |
| **1f** | Status bar, keyboard navigation, error handling | Small |
| **2** | Pure Go orchestrator (container ops, auth, process mgmt) | Large |
| **3** | Enhancements: mouse support, session export, retry individual agents, cost history | Medium |

## Warnings

- Apple containers on macOS 26 may not expose a Docker-compatible API — container operations in Go (Phase 2) will need to use the `container` CLI tool via `os/exec` rather than Docker SDK
- Ring buffer size (500 lines) for UI display is a heuristic — may need tuning for verbose agents
- `lipgloss.Style.Copy()` is deprecated in newer versions — use assignment instead
- The channel-based event pattern requires careful cleanup — close the channel when the orchestrator exits to avoid goroutine leaks
- Auth extraction from macOS Keychain is the primary reason to keep Bash in Phase 1; if using Go's `os/exec` to call `security find-generic-password`, the hybrid benefit diminishes
- Bash `--tui-mode` modifications assume `jq` is available for JSON escaping
