package tui

import (
    "ai-council/internal/types"
    "github.com/charmbracelet/bubbles/spinner"
    "github.com/charmbracelet/bubbles/textarea"
    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
)

type ViewMode int

const (
    ModePrompt ViewMode = iota
    ModeRunning
    ModeComplete
)

type FocusedPanel int

const (
    PanelOverview FocusedPanel = iota
    PanelClaude
    PanelCodex
    PanelGemini
    PanelVibe
    PanelSynthesis
)

type Config struct {
    Agents      []types.AgentID
    Timeout     int
    Workspace   string
    NoSynthesis bool
    ScriptPath  string
}

type Model struct {
    Mode         ViewMode
    FocusedPanel FocusedPanel
    Width        int
    Height       int

    PromptInput    textarea.Model
    AgentViewports map[types.AgentID]*viewport.Model
    SynthViewport  viewport.Model
    Spinners       map[types.AgentID]spinner.Model

    Session *types.SessionState

    EventChan <-chan tea.Msg
    Config    Config
}

func InitialModel(cfg Config, eventChan <-chan tea.Msg) Model {
    ta := textarea.New()
    ta.Placeholder = "Enter your prompt here (Alt+Enter to submit)..."
    ta.Focus()
    ta.Prompt = "┃ "
    ta.CharLimit = 2000

    agents := make(map[types.AgentID]*types.AgentState)
    agentViewports := make(map[types.AgentID]*viewport.Model)
    spinners := make(map[types.AgentID]spinner.Model)

    for _, id := range types.AgentOrder {
        agents[id] = &types.AgentState{ID: id, Status: types.StatusPending}
        vp := viewport.New(0, 0)
        agentViewports[id] = &vp
        s := spinner.New()
        s.Spinner = spinner.Dot
        spinners[id] = s
    }

    return Model{
        Mode:           ModePrompt,
        FocusedPanel:   PanelOverview,
        PromptInput:    ta,
        AgentViewports: agentViewports,
        Spinners:       spinners,
        Session: &types.SessionState{
            Agents: agents,
        },
        EventChan: eventChan,
        Config:    cfg,
    }
}

func (m Model) Init() tea.Cmd {
    cmds := []tea.Cmd{textarea.Blink}
    for _, s := range m.Spinners {
        cmds = append(cmds, s.Tick)
    }
    return tea.Batch(cmds...)
}
