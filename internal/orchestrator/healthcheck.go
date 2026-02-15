package orchestrator

import (
	"ai-council/internal/types"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CheckSeverity indicates how critical a health check failure is.
type CheckSeverity int

const (
	SeverityWarning  CheckSeverity = iota // Degraded but can proceed
	SeverityCritical                      // Cannot proceed
)

func (s CheckSeverity) String() string {
	if s == SeverityWarning {
		return "warning"
	}
	return "critical"
}

// HealthCheckResult describes the outcome of a single pre-launch check.
type HealthCheckResult struct {
	Name     string
	OK       bool
	Severity CheckSeverity
	Message  string
	FixHint  string // Self-healing instruction for the user
}

// RunPreLaunchChecks performs all pre-launch health checks and returns
// the results. Checks run for the given agents, workspace, and config.
func RunPreLaunchChecks(agents []types.AgentID, workspace string, scriptDir string) []HealthCheckResult {
	var results []HealthCheckResult

	results = append(results, checkContainerCLI())
	results = append(results, checkContainerImage())
	results = append(results, checkWorkspace(workspace))
	results = append(results, checkDiskSpace())

	for _, agent := range agents {
		results = append(results, checkAgentAuth(agent, scriptDir)...)
	}

	return results
}

// HasCriticalFailure returns true if any check has a critical failure.
func HasCriticalFailure(results []HealthCheckResult) bool {
	for _, r := range results {
		if !r.OK && r.Severity == SeverityCritical {
			return true
		}
	}
	return false
}

// FormatResults returns a human-readable summary of health check results.
func FormatResults(results []HealthCheckResult) string {
	var sb strings.Builder
	for _, r := range results {
		if r.OK {
			sb.WriteString(fmt.Sprintf("  [ok] %s\n", r.Name))
		} else {
			sb.WriteString(fmt.Sprintf("  [%s] %s: %s\n", r.Severity, r.Name, r.Message))
			if r.FixHint != "" {
				sb.WriteString(fmt.Sprintf("         Fix: %s\n", r.FixHint))
			}
		}
	}
	return sb.String()
}

func checkContainerCLI() HealthCheckResult {
	result := HealthCheckResult{Name: "Container CLI"}

	_, err := exec.LookPath("container")
	if err != nil {
		result.OK = false
		result.Severity = SeverityCritical
		result.Message = "Apple container CLI not found in PATH"
		result.FixHint = "Install: brew install container (or download from https://github.com/apple/container/releases)"
		return result
	}

	result.OK = true
	return result
}

func checkContainerImage() HealthCheckResult {
	result := HealthCheckResult{Name: "Container image"}

	out, err := exec.Command("container", "images").CombinedOutput()
	if err != nil {
		result.OK = false
		result.Severity = SeverityCritical
		result.Message = "Failed to list container images"
		result.FixHint = "Ensure containermanagerd is running (may require GUI session)"
		return result
	}

	if !strings.Contains(string(out), "council") {
		result.OK = false
		result.Severity = SeverityCritical
		result.Message = "council:latest image not found"
		result.FixHint = "Run: ./setup.sh (or ./council.sh --rebuild)"
		return result
	}

	result.OK = true
	return result
}

func checkWorkspace(workspace string) HealthCheckResult {
	result := HealthCheckResult{Name: "Workspace"}

	if workspace == "" {
		result.OK = true
		return result
	}

	info, err := os.Stat(workspace)
	if err != nil {
		result.OK = false
		result.Severity = SeverityCritical
		result.Message = fmt.Sprintf("Workspace path not accessible: %s", workspace)
		result.FixHint = fmt.Sprintf("Verify path exists: ls -la %s", workspace)
		return result
	}

	if !info.IsDir() {
		result.OK = false
		result.Severity = SeverityCritical
		result.Message = fmt.Sprintf("Workspace path is not a directory: %s", workspace)
		result.FixHint = "Workspace must be a directory (Apple containers only mount directories)"
		return result
	}

	result.OK = true
	return result
}

func checkDiskSpace() HealthCheckResult {
	result := HealthCheckResult{Name: "Disk space"}

	// Use df to check available space on the root volume
	out, err := exec.Command("df", "-g", "/").CombinedOutput()
	if err != nil {
		result.OK = false
		result.Severity = SeverityWarning
		result.Message = "Could not check disk space"
		return result
	}

	// Parse df output — look for available GB in the second line
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		result.OK = true
		return result
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		result.OK = true
		return result
	}

	// fields[3] is available space in GB
	var availGB int
	fmt.Sscanf(fields[3], "%d", &availGB)

	if availGB < 2 {
		result.OK = false
		result.Severity = SeverityWarning
		result.Message = fmt.Sprintf("Low disk space: %dGB available", availGB)
		result.FixHint = "Free space by cleaning old sessions: rm -rf ai-council/output/*/  ai-council/logs/*/"
		return result
	}

	result.OK = true
	return result
}

func checkAgentAuth(agent types.AgentID, scriptDir string) []HealthCheckResult {
	switch agent {
	case types.AgentClaude:
		return []HealthCheckResult{checkClaudeAuth()}
	case types.AgentCodex:
		return []HealthCheckResult{checkFileAuth(agent, "~/.codex/auth.json", "codex")}
	case types.AgentGemini:
		return []HealthCheckResult{checkFileAuth(agent, "~/.gemini/settings.json", "gemini")}
	case types.AgentVibe:
		return checkVibeAuth(scriptDir)
	}
	return nil
}

func checkClaudeAuth() HealthCheckResult {
	result := HealthCheckResult{Name: "Claude auth (Keychain)"}

	out, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").CombinedOutput()
	if err != nil {
		result.OK = false
		result.Severity = SeverityCritical
		result.Message = "Claude OAuth not found in macOS Keychain"
		result.FixHint = "Run: claude /login (requires browser)"
		return result
	}

	// Check if output contains valid JSON with accessToken
	if !strings.Contains(string(out), "accessToken") {
		result.OK = false
		result.Severity = SeverityCritical
		result.Message = "Keychain entry exists but missing accessToken"
		result.FixHint = "Re-authenticate: claude /login"
		return result
	}

	result.OK = true
	return result
}

func checkFileAuth(agent types.AgentID, path string, cliName string) HealthCheckResult {
	result := HealthCheckResult{Name: fmt.Sprintf("%s auth", agent)}

	expandedPath := expandHome(path)
	if _, err := os.Stat(expandedPath); err != nil {
		result.OK = false
		result.Severity = SeverityCritical
		result.Message = fmt.Sprintf("Auth file not found: %s", path)
		result.FixHint = fmt.Sprintf("Run: %s (complete OAuth login)", cliName)
		return result
	}

	result.OK = true
	return result
}

func checkVibeAuth(scriptDir string) []HealthCheckResult {
	var results []HealthCheckResult

	envResult := HealthCheckResult{Name: "Vibe auth (.env)"}
	envPath := expandHome("~/.vibe/.env")
	if _, err := os.Stat(envPath); err != nil {
		envResult.OK = false
		envResult.Severity = SeverityCritical
		envResult.Message = "Vibe .env not found at ~/.vibe/.env"
		envResult.FixHint = "Run: vibe --setup (stores MISTRAL_API_KEY)"
	} else {
		envResult.OK = true
	}
	results = append(results, envResult)

	configResult := HealthCheckResult{Name: "Vibe container config"}
	configPath := filepath.Join(scriptDir, "config", "vibe-container.toml")
	if _, err := os.Stat(configPath); err != nil {
		configResult.OK = false
		configResult.Severity = SeverityWarning
		configResult.Message = "vibe-container.toml not found"
		configResult.FixHint = fmt.Sprintf("Expected at: %s", configPath)
	} else {
		configResult.OK = true
	}
	results = append(results, configResult)

	return results
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
