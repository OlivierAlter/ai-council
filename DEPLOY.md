# AI Council — Deployment Guide

Deploy the AI Council on a new machine. Covers both local development setups and dedicated remote machines (with Tailscale).

## Requirements

- Apple Silicon Mac (M1/M2/M3/M4)
- macOS 26 Tahoe or later
- ~16GB RAM recommended (4GB per concurrent agent container)
- ~5GB disk (container image + CLI tooling)

## Step 1: Install System Dependencies

```bash
# Homebrew (if not already installed)
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Apple container CLI
brew install container

# jq (used by output parsing)
brew install jq

# Node.js (for Claude Code and Codex)
brew install node

# Python / uv (for Vibe)
brew install uv
```

Verify:
```bash
container --version   # Apple container CLI
jq --version          # JSON processor
node --version        # Node.js 22+
uv --version          # Python package manager
```

## Step 2: Install AI Agent CLIs

```bash
# Claude Code (Anthropic)
npm install -g @anthropic-ai/claude-code

# OpenAI Codex
npm install -g @openai/codex

# Google Gemini CLI
npm install -g @google/gemini-cli

# Mistral Vibe CLI
uv tool install mistral-vibe
```

Verify all four are on PATH:
```bash
claude --version
codex --version
gemini --version
vibe --version
```

## Step 3: Authenticate Each CLI

Each CLI needs to be logged in on the host. This requires a browser for the OAuth flows.

> **Remote/headless machine?** Do this step via Screen Sharing (VNC) over Tailscale or directly at the machine. See [Remote Deployment](#remote-deployment) below.

```bash
# Claude Code — OAuth via browser, stores token in macOS Keychain
claude
# Follow the browser login prompt, then exit (Ctrl+C)

# OpenAI Codex — OAuth via browser, stores in ~/.codex/auth.json
codex
# Follow the ChatGPT login prompt, then exit

# Google Gemini — OAuth via browser, stores in ~/.gemini/
gemini
# Follow the Google login prompt, then exit

# Mistral Vibe — API key, stores in ~/.vibe/.env
vibe --setup
# Enter your MISTRAL_API_KEY when prompted
```

### Verify credentials exist

```bash
# Claude: check macOS Keychain
security find-generic-password -s "Claude Code-credentials" -w | jq '.claudeAiOauth.accessToken' 2>/dev/null && echo "Claude: OK"

# Codex: check file
[ -f ~/.codex/auth.json ] && echo "Codex: OK"

# Gemini: check directory
[ -d ~/.gemini ] && echo "Gemini: OK"

# Vibe: check file
[ -f ~/.vibe/.env ] && echo "Vibe: OK"
```

### Optional: API key fallback

If you prefer API keys over OAuth (simpler but costs money):

```bash
cp config/council.env.example config/council.env
```

Edit `config/council.env`:
```bash
ANTHROPIC_API_KEY=sk-ant-api03-...
OPENAI_API_KEY=sk-...
GEMINI_API_KEY=...
MISTRAL_API_KEY=...
```

Then set `AUTH_MODE="apikey"` in `config/council.conf`.

## Step 4: Copy the Council

```bash
# From your existing machine to the new one
scp -r ai-council/ user@newmachine:~/ai-council/

# Or clone from git
git clone <your-repo> ~/ai-council
```

## Step 5: Run Setup

```bash
cd ~/ai-council
./setup.sh
```

This will:
1. Verify macOS version and Apple Silicon
2. Check for the `container` CLI
3. Check for Claude CLI on host (needed for synthesis)
4. Verify agent credentials (OAuth dirs or API keys)
5. Build the container image (~2-3 minutes first time)
6. Run smoke tests for each authenticated agent
7. Optionally symlink `council` to `/usr/local/bin/`

## Step 6: Verify

```bash
# Quick test with one agent
council "What is 2+2?" --agents claude

# All agents, no synthesis
council "Explain HTTP in one paragraph" --no-synth

# Full council with synthesis
council "Write a Python function to check if a string is a palindrome"

# Collab mode dry run
council "Build a calculator CLI" --collab --dry-run
```

## Step 7: Build the TUI (Optional)

```bash
# Requires Go 1.24+
brew install go

cd ~/ai-council
go build -o council-tui ./cmd/council-tui/

# Run with TUI
./council-tui --timeout 300 --workspace /path/to/project
```

---

## Remote Deployment

Run the council on a dedicated Mac and access it from anywhere via Tailscale.

### Machine Setup

#### Power and Sleep

```bash
# Prevent sleep
sudo pmset -a sleep 0 displaysleep 0 disksleep 0

# Auto-restart after power failure
sudo pmset -a autorestart 1

# Wake on network access
sudo pmset -a womp 1

# Verify
pmset -g
```

#### Auto-Login

Enable automatic login in **System Settings > Users & Groups > Login Options**.

This is required because:
- macOS Keychain is locked without a GUI session (Claude OAuth fails)
- Apple container runtime may require a user session
- Screen Sharing needs a logged-in user for maintenance

#### Enable Remote Access

In **System Settings > General > Sharing**, enable:
- **Remote Login** (SSH)
- **Screen Sharing** (VNC) — for initial CLI auth and maintenance

#### Tailscale

```bash
# Install
brew install tailscale

# Start and authenticate
sudo tailscale up --ssh

# Verify the machine appears in your tailnet
tailscale status
```

The `--ssh` flag enables Tailscale SSH, meaning you can connect without managing SSH keys:
```bash
# From your laptop
tailscale ssh macmini
```

### Initial Authentication (One-Time)

Connect via **Screen Sharing** (VNC) over Tailscale:

1. Open Finder on your laptop
2. Go > Connect to Server > `vnc://macmini.tailnet`
3. In the Screen Sharing session, open Terminal
4. Run the authentication commands from [Step 3](#step-3-authenticate-each-cli)
5. Disconnect Screen Sharing — everything else is SSH from here

### Daily Usage

#### SSH + CLI

```bash
# Single command
tailscale ssh macmini "council 'Fix the auth bug in login.py'"

# Interactive TUI (needs -t for TTY allocation)
tailscale ssh -t macmini "council 'Build a REST API' --tui"

# Interactive session
tailscale ssh macmini
cd ~/ai-council
council "Design a database schema" --collab --verbose
```

#### Running Long Jobs

```bash
# Use tmux/screen for long-running councils that survive SSH disconnects
tailscale ssh macmini
tmux new -s council
council "Refactor the entire auth module" --collab --timeout 600
# Ctrl+B, D to detach

# Reconnect later
tailscale ssh macmini
tmux attach -t council
```

---

## Troubleshooting

### "OAuth token has expired" (Claude 401)

The `ensure_claude_oauth_token()` function auto-refreshes expired tokens. If the refresh token itself has expired:
```bash
# Re-authenticate via Screen Sharing, or:
# If you have physical/VNC access:
claude /login
```

### Keychain locked over SSH

```bash
# Unlock the login keychain manually
security unlock-keychain -p "your-login-password" ~/Library/Keychains/login.keychain-db
```

Or ensure auto-login is enabled (keeps keychain unlocked permanently).

### Container CLI not working over SSH

```bash
# Check if containermanagerd is running
container list 2>&1

# If it fails, you may need a GUI session — ensure auto-login is enabled
```

### Agent smoke test fails

```bash
# Check logs from the last run
ls -la logs/
cat logs/<latest-session>/claude.log

# Run a single agent with verbose output
council "test" --agents claude --verbose
```

### "command not found: council"

```bash
# Re-create the symlink
sudo ln -sf ~/ai-council/council.sh /usr/local/bin/council

# Or add to PATH
echo 'export PATH="$HOME/ai-council:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

### Container image build fails

```bash
# Force rebuild
council "test" --rebuild --agents claude

# Or rebuild manually
cd ~/ai-council
container build --tag council:latest .
```

### Token/credential status check

```bash
# Claude: check token expiry
security find-generic-password -s "Claude Code-credentials" -w 2>/dev/null | \
  jq '{expires: (.claudeAiOauth.expiresAt / 1000 | strftime("%Y-%m-%d %H:%M")), subscription: .claudeAiOauth.subscriptionType}'

# Codex: check credential file age
ls -la ~/.codex/auth.json

# Gemini: check credential directory
ls -la ~/.gemini/

# Vibe: check API key exists
grep -c 'MISTRAL_API_KEY=' ~/.vibe/.env 2>/dev/null && echo "Vibe key present"
```

---

## File Reference

| File | Purpose | Portable? |
|------|---------|-----------|
| `council.sh` | Main orchestrator | Yes |
| `lib/*.sh` | Shell libraries | Yes |
| `config/council.conf` | Configuration | Yes (edit for new machine) |
| `config/council.env` | API keys (gitignored) | No (machine-specific secrets) |
| `config/vibe-container.toml` | Vibe container config | Yes |
| `prompts/*.md` | System prompts | Yes |
| `Containerfile` | Container image spec | Yes (rebuild on new machine) |
| `setup.sh` | First-time setup | Yes |
| `cmd/`, `internal/` | Go TUI source | Yes (recompile on new machine) |
| `council-tui` | Compiled TUI binary | No (recompile for target arch) |
| `output/` | Session results (gitignored) | No (generated per-machine) |
| `logs/` | Agent logs (gitignored) | No (generated per-machine) |
| `PLAN.md` | Architecture docs | Yes |
