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

    case msgs.AgentStartedMsg:
        agent := m.Session.Agents[msg.Agent]
        agent.Status = types.StatusRunning
        agent.StartTime = msg.Time
        return m, m.WaitForEvent()

    case msgs.AgentOutputMsg:
        agent := m.Session.Agents[msg.Agent]
        agent.Output = append(agent.Output, types.OutputLine{
            Text: msg.Line, Stream: msg.Stream, Timestamp: time.Now(),
        })
        agent.Stream = append(agent.Stream, msg.Line)
        if len(agent.Stream) > 500 {
            agent.Stream = agent.Stream[len(agent.Stream)-500:]
        }
        if vp, ok := m.AgentViewports[msg.Agent]; ok {
            vp.SetContent(strings.Join(agent.Stream, "\n"))
            vp.GotoBottom()
        }
        return m, m.WaitForEvent()

    case msgs.AgentCompletedMsg:
        agent := m.Session.Agents[msg.Agent]
        agent.Status = msg.Status
        agent.Usage = msg.Usage
        now := time.Now()
        agent.EndTime = &now
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
        switch msg.String() {
        case "ctrl+c":
            return m, tea.Quit
        case "tab":
            m.FocusedPanel = (m.FocusedPanel + 1) % 6
        case "shift+tab":
            m.FocusedPanel = (m.FocusedPanel - 1 + 6) % 6
        case "1": m.FocusedPanel = PanelClaude
        case "2": m.FocusedPanel = PanelCodex
        case "3": m.FocusedPanel = PanelGemini
        case "4": m.FocusedPanel = PanelVibe
        case "5", "s": m.FocusedPanel = PanelSynthesis
        case "0": m.FocusedPanel = PanelOverview
        case "q":
            return m, tea.Quit
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
