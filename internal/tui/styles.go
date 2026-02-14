package tui

import (
    "ai-council/internal/types"
    "github.com/charmbracelet/lipgloss"
)

var (
    ColorClaude  = lipgloss.Color("#E57373") // Warm red
    ColorCodex   = lipgloss.Color("#81C784") // Green
    ColorGemini  = lipgloss.Color("#64B5F6") // Blue
    ColorVibe    = lipgloss.Color("#FFD54F") // Yellow

    ColorSuccess = lipgloss.Color("#4CAF50")
    ColorError   = lipgloss.Color("#F44336")
    ColorWarning = lipgloss.Color("#FF9800")
    ColorDim     = lipgloss.Color("#757575")

    TitleStyle = lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("#FFFFFF")).
        Background(lipgloss.Color("#7D56F4")).
        Padding(0, 1)

    DimStyle  = lipgloss.NewStyle().Foreground(ColorDim)
    HelpStyle = lipgloss.NewStyle().Foreground(ColorDim).Italic(true)
)

func AgentColor(id types.AgentID) lipgloss.Color {
    switch id {
    case types.AgentClaude:  return ColorClaude
    case types.AgentCodex:   return ColorCodex
    case types.AgentGemini:  return ColorGemini
    case types.AgentVibe:    return ColorVibe
    default:                 return lipgloss.Color("#FFFFFF")
    }
}
