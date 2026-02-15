package tui

import (
    "ai-council/internal/msgs"
    "ai-council/internal/orchestrator"
    "ai-council/internal/types"
    "context"
    "strings"
    "time"

    "github.com/charmbracelet/bubbles/spinner"
    tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.WindowSizeMsg:
        m.Width = msg.Width
        m.Height = msg.Height
        return m, nil

    case tea.KeyMsg:
        return m.handleKeyPress(msg)

    case spinner.TickMsg:
        var cmds []tea.Cmd
        for id, s := range m.Spinners {
            var cmd tea.Cmd
            m.Spinners[id], cmd = s.Update(msg)
            cmds = append(cmds, cmd)
        }
        return m, tea.Batch(cmds...)

    case msgs.SessionStartedMsg:
        m.Session.ID = msg.ID
        m.Session.Prompt = msg.Prompt
        m.Session.StartTime = time.Now()
        return m, m.WaitForEvent()

    case msgs.AgentStartingMsg:
        agent := m.Session.Agents[msg.Agent]
        agent.Transition(types.StatusStarting)
        agent.StartTime = msg.Time
        return m, m.WaitForEvent()

    case msgs.AgentStartedMsg:
        agent := m.Session.Agents[msg.Agent]
        agent.Transition(types.StatusRunning)
        return m, m.WaitForEvent()

    case msgs.AgentOutputMsg:
        agent := m.Session.Agents[msg.Agent]
        now := time.Now()
        agent.Output = append(agent.Output, types.OutputLine{
            Text: msg.Line, Stream: msg.Stream, Timestamp: now,
        })
        agent.LastOutputAt = &now
        // Route through PanelBuffer for foreground/background awareness
        if buf, ok := m.PanelBuffers[msg.Agent]; ok {
            buf.Append(msg.Line)
        }
        // Update viewport if this agent is currently foregrounded
        if m.isAgentFocused(msg.Agent) {
            if vp, ok := m.AgentViewports[msg.Agent]; ok {
                if buf, ok := m.PanelBuffers[msg.Agent]; ok {
                    vp.SetContent(buf.Content())
                }
                vp.GotoBottom()
            }
        }
        return m, m.WaitForEvent()

    case msgs.AgentCompletedMsg:
        agent := m.Session.Agents[msg.Agent]
        agent.Transition(msg.Status)
        agent.Usage = msg.Usage
        return m, m.WaitForEvent()

    case msgs.SynthesisStartedMsg:
        m.Session.SynthesisActive = true
        return m, m.WaitForEvent()

    case msgs.SynthesisCompleteMsg:
        m.Session.SynthesisResult = msg.Result
        m.Session.SynthesisActive = false
        m.Mode = ModeComplete
        m.FocusedPanel = PanelSynthesis
        return m, nil

    case msgs.ErrorMsg:
        // Handle error
        return m, nil
    }

    if m.Mode == ModePrompt {
        var cmd tea.Cmd
        m.PromptInput, cmd = m.PromptInput.Update(msg)
        return m, cmd
    }

    return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch m.Mode {
    case ModePrompt:
        switch msg.Type {
        case tea.KeyCtrlC, tea.KeyEsc:
            return m, tea.Quit
        case tea.KeyEnter:
            if msg.Alt {
                return m.submitPrompt()
            }
            var cmd tea.Cmd
            m.PromptInput, cmd = m.PromptInput.Update(msg)
            return m, cmd
        }

    case ModeRunning, ModeComplete:
        var newPanel FocusedPanel = -1
        switch msg.String() {
        case "ctrl+c":
            return m, tea.Quit
        case "tab":
            newPanel = (m.FocusedPanel + 1) % 6
        case "shift+tab":
            newPanel = (m.FocusedPanel - 1 + 6) % 6
        case "1": newPanel = PanelClaude
        case "2": newPanel = PanelCodex
        case "3": newPanel = PanelGemini
        case "4": newPanel = PanelVibe
        case "5", "s": newPanel = PanelSynthesis
        case "0": newPanel = PanelOverview
        case "q":
            return m, tea.Quit
        }
        if newPanel >= 0 && newPanel != m.FocusedPanel {
            m.switchPanel(newPanel)
        }
    }
    return m, nil
}

func (m Model) submitPrompt() (tea.Model, tea.Cmd) {
    prompt := m.PromptInput.Value()
    if strings.TrimSpace(prompt) == "" {
        return m, nil
    }

    m.Mode = ModeRunning
    m.Session.Prompt = prompt
    m.Session.StartTime = time.Now()

    exec := orchestrator.NewExecutor(m.Config.ScriptPath)
    m.EventChan = exec.Events()

    runCmd := func() tea.Msg {
        err := exec.Run(context.Background(), prompt, "", m.Config.Timeout, m.Config.Workspace)
        if err != nil {
            return msgs.ErrorMsg{Err: err}
        }
        return nil
    }

    return m, tea.Batch(runCmd, m.WaitForEvent())
}

func (m Model) WaitForEvent() tea.Cmd {
    return func() tea.Msg {
        return <-m.EventChan
    }
}

// switchPanel handles the foreground/background transition with catchup.
func (m *Model) switchPanel(newPanel FocusedPanel) {
    // Background the old panel's agent
    if agent, ok := m.panelToAgent(m.FocusedPanel); ok {
        if buf, ok := m.PanelBuffers[agent]; ok {
            buf.SetBackground()
        }
    }

    m.FocusedPanel = newPanel

    // Foreground the new panel's agent and flush catchup
    if agent, ok := m.panelToAgent(newPanel); ok {
        if buf, ok := m.PanelBuffers[agent]; ok {
            buf.SetForeground()
            // Refresh viewport with full ring buffer content (includes catchup)
            if vp, ok := m.AgentViewports[agent]; ok {
                vp.SetContent(buf.Content())
                vp.GotoBottom()
            }
        }
    }
}

// panelToAgent maps a FocusedPanel to its AgentID. Returns false for
// non-agent panels (Overview, Synthesis).
func (m *Model) panelToAgent(panel FocusedPanel) (types.AgentID, bool) {
    switch panel {
    case PanelClaude:  return types.AgentClaude, true
    case PanelCodex:   return types.AgentCodex, true
    case PanelGemini:  return types.AgentGemini, true
    case PanelVibe:    return types.AgentVibe, true
    }
    return "", false
}

// isAgentFocused returns true if the given agent's panel is currently visible.
func (m *Model) isAgentFocused(agent types.AgentID) bool {
    focused, ok := m.panelToAgent(m.FocusedPanel)
    return ok && focused == agent
}
