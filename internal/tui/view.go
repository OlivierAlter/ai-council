package tui

import (
    "ai-council/internal/types"
    "fmt"
    "strings"

    "github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
    if m.Width == 0 || m.Height == 0 {
        return "Initializing..."
    }

    switch m.Mode {
    case ModePrompt:
        return m.promptView()
    case ModeRunning, ModeComplete:
        return m.runningView()
    default:
        return "Unknown mode"
    }
}

func (m Model) promptView() string {
    return lipgloss.Place(
        m.Width, m.Height,
        lipgloss.Center, lipgloss.Center,
        lipgloss.JoinVertical(
            lipgloss.Center,
            TitleStyle.Render("AI Council"),
            "",
            m.PromptInput.View(),
            "",
            HelpStyle.Render("Alt+Enter to submit • Ctrl+C to quit"),
        ),
    )
}

func (m Model) runningView() string {
    var s strings.Builder

    // Header
    s.WriteString(TitleStyle.Render("AI Council"))
    s.WriteString(fmt.Sprintf(" | Session: %s\n", m.Session.ID))
    s.WriteString(DimStyle.Render(strings.Repeat("─", m.Width)))
    s.WriteString("\n\n")

    // Main content based on focused panel
    switch m.FocusedPanel {
    case PanelOverview:
        s.WriteString(m.overviewGrid())
    case PanelClaude, PanelCodex, PanelGemini, PanelVibe:
        s.WriteString(m.agentDetailView())
    case PanelSynthesis:
        s.WriteString(m.synthesisView())
    }

    // Footer
    s.WriteString("\n" + DimStyle.Render(strings.Repeat("─", m.Width)) + "\n")
    s.WriteString(HelpStyle.Render("1-4: Agents | 5: Synthesis | 0: Overview | Tab: Cycle | Q: Quit"))

    return s.String()
}

func (m Model) overviewGrid() string {
    var cards []string
    for _, id := range types.AgentOrder {
        agent := m.Session.Agents[id]
        status := agent.Status.String()
        color := AgentColor(id)
        
        content := fmt.Sprintf("%s\nStatus: %s", id, status)
        if agent.Usage != nil {
            content += fmt.Sprintf("\nTokens: %d", agent.Usage.InputTokens+agent.Usage.OutputTokens)
        }

        card := lipgloss.NewStyle().
            Border(lipgloss.RoundedBorder()).
            BorderForeground(color).
            Padding(1).
            Width(m.Width/2 - 4).
            Height(m.Height/3).
            Render(content)
        cards = append(cards, card)
    }

    row1 := lipgloss.JoinHorizontal(lipgloss.Top, cards[0], cards[1])
    row2 := lipgloss.JoinHorizontal(lipgloss.Top, cards[2], cards[3])
    return lipgloss.JoinVertical(lipgloss.Left, row1, row2)
}

func (m Model) agentDetailView() string {
    var id types.AgentID
    switch m.FocusedPanel {
    case PanelClaude: id = types.AgentClaude
    case PanelCodex:  id = types.AgentCodex
    case PanelGemini: id = types.AgentGemini
    case PanelVibe:   id = types.AgentVibe
    }

    vp := m.AgentViewports[id]
    
    header := lipgloss.NewStyle().
        Foreground(AgentColor(id)).
        Bold(true).
        Render(fmt.Sprintf("─── %s ───", strings.ToUpper(string(id))))

    return lipgloss.JoinVertical(lipgloss.Left, header, "", vp.View())
}

func (m Model) synthesisView() string {
    if m.Session.SynthesisResult == nil {
        if m.Session.SynthesisActive {
            return "Synthesizing results..."
        }
        return "Waiting for agents to complete..."
    }

    res := m.Session.SynthesisResult
    return fmt.Sprintf("CONFIDENCE: %s\n\n%s", res.Confidence, res.Summary)
}
