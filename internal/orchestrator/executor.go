package orchestrator

import (
    "bufio"
    "context"
    "encoding/json"
    "os/exec"
    "strconv"
    "time"

    "ai-council/internal/msgs"
    "ai-council/internal/types"

    tea "github.com/charmbracelet/bubbletea"
)

type Event struct {
    Type string          `json:"type"`
    TS   string          `json:"ts"`
    Data json.RawMessage `json:"data"`
}

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

func (e *Executor) Run(ctx context.Context, prompt string, agents string, timeout int, workspace string) error {
    args := []string{prompt, "--tui-mode"}
    if agents != "" {
        args = append(args, "--agents", agents)
    }
    if timeout > 0 {
        args = append(args, "--timeout", strconv.Itoa(timeout))
    }
    if workspace != "" {
        args = append(args, "--workspace", workspace)
    }

    cmd := exec.CommandContext(ctx, e.scriptPath, args...)
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return err
    }
    // We don't pipe stderr because council.sh --tui-mode emits everything to stdout as JSONL
    // or we might want to capture it just in case of bash errors.
    // stderr, _ := cmd.StderrPipe()

    if err := cmd.Start(); err != nil {
        return err
    }

    // Parse JSONL events from stdout
    go func() {
        scanner := bufio.NewScanner(stdout)
        // Allow for very long lines in case of large JSON outputs
        scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
        for scanner.Scan() {
            line := scanner.Text()
            if msg := e.parseEvent(line); msg != nil {
                e.eventChan <- msg
            }
        }
    }()

    return cmd.Wait()
}

func (e *Executor) Events() <-chan tea.Msg {
    return e.eventChan
}

func (e *Executor) parseEvent(line string) tea.Msg {
    var ev Event
    if err := json.Unmarshal([]byte(line), &ev); err != nil {
        // Not JSONL or invalid event, ignore
        return nil
    }

    ts, _ := time.Parse(time.RFC3339, ev.TS)

    switch ev.Type {
    case "session.started":
        var d struct {
            ID     string `json:"id"`
            Prompt string `json:"prompt"`
            Agents string `json:"agents"`
        }
        json.Unmarshal(ev.Data, &d)
        // Parse agents string back to slice
        return msgs.SessionStartedMsg{ID: d.ID, Prompt: d.Prompt}

    case "agent.starting":
        var d struct {
            Agent types.AgentID `json:"agent"`
        }
        json.Unmarshal(ev.Data, &d)
        return msgs.AgentStartingMsg{Agent: d.Agent, Time: ts}

    case "agent.started":
        var d struct {
            Agent types.AgentID `json:"agent"`
        }
        json.Unmarshal(ev.Data, &d)
        return msgs.AgentStartedMsg{Agent: d.Agent, Time: ts}

    case "agent.output":
        var d struct {
            Agent  types.AgentID `json:"agent"`
            Line   string        `json:"line"`
            Stream string        `json:"stream"`
        }
        json.Unmarshal(ev.Data, &d)
        return msgs.AgentOutputMsg{Agent: d.Agent, Line: d.Line, Stream: d.Stream}

    case "agent.completed":
        var d struct {
            Agent  types.AgentID     `json:"agent"`
            Status string            `json:"status"`
            Usage  *types.TokenUsage `json:"usage"`
        }
        json.Unmarshal(ev.Data, &d)
        var status types.AgentStatus
        switch d.Status {
        case "success":
            status = types.StatusCompleted
        case "timeout":
            status = types.StatusTimedOut
        case "killed":
            status = types.StatusKilled
        default:
            status = types.StatusFailed
        }
        return msgs.AgentCompletedMsg{Agent: d.Agent, Status: status, Usage: d.Usage}

    case "synthesis.started":
        return msgs.SynthesisStartedMsg{}

    case "synthesis.completed":
        var d struct {
            Result *types.SynthesisResult `json:"result"`
        }
        json.Unmarshal(ev.Data, &d)
        return msgs.SynthesisCompleteMsg{Result: d.Result}
    }

    return nil
}
