#!/usr/bin/env bash
# synthesizer.sh - Run Claude Code on host to synthesize agent outputs

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

run_synthesis() {
    local envelope_file="$1"
    local output_file="$2"
    local system_prompt_file="${SCRIPT_DIR}/prompts/synthesizer-system.md"

    if [ ! -f "$system_prompt_file" ]; then
        echo "[synth] Error: synthesizer system prompt not found" >&2
        return 1
    fi

    if [ ! -f "$envelope_file" ]; then
        echo "[synth] Error: envelope file not found" >&2
        return 1
    fi

    local system_prompt
    system_prompt="$(cat "$system_prompt_file")"

    local envelope
    envelope="$(cat "$envelope_file")"

    # Count successful agents
    local success_count
    success_count=$(echo "$envelope" | jq '[.agents[] | select(.status == "success")] | length')

    if [ "$success_count" -eq 0 ]; then
        echo "[synth] No agents succeeded - skipping synthesis"
        jq -n '{
            "summary": "All agents failed. No synthesis possible.",
            "approach": null,
            "confidence": "none",
            "agents": {},
            "consensus_areas": [],
            "divergence_areas": [],
            "warnings": ["All agents failed to produce output"]
        }' > "$output_file"
        return 1
    fi

    echo "[synth] Synthesizing results from $success_count agent(s)..."

    # Write prompt to a temp file to avoid shell quoting issues with code/JSON
    local synth_prompt_file
    synth_prompt_file="$(mktemp)"
    trap "rm -f '$synth_prompt_file'" RETURN

    cat > "$synth_prompt_file" <<PROMPT
Here are the results from the AI Council. Each agent was given the same task independently.

## Original Task
$(echo "$envelope" | jq -r '.prompt')

## Agent Outputs
$(echo "$envelope" | jq '.')

Analyze all agent outputs and produce your synthesized response as specified in your system instructions.
PROMPT

    # Pipe prompt via stdin to avoid shell quoting issues with special chars
    # Unset CLAUDECODE to allow nested claude invocation (synthesis runs on the host)
    if cat "$synth_prompt_file" | CLAUDECODE= claude -p \
        --output-format json \
        --system-prompt "$system_prompt" \
        > "$output_file" 2>/dev/null; then
        echo "[synth] Synthesis complete"
        return 0
    else
        echo "[synth] Synthesis failed" >&2
        return 1
    fi
}

# Collaborative mode synthesis — assembles complementary task outputs
run_collab_synthesis() {
    local envelope_file="$1"
    local output_file="$2"
    local extra_instructions="${3:-Combine all task outputs into a coherent final answer.}"
    local system_prompt_file="${SCRIPT_DIR}/prompts/collab-synthesizer-system.md"

    if [ ! -f "$system_prompt_file" ]; then
        echo "[synth-collab] Error: collab synthesizer system prompt not found" >&2
        return 1
    fi

    if [ ! -f "$envelope_file" ]; then
        echo "[synth-collab] Error: collab envelope file not found" >&2
        return 1
    fi

    local system_prompt
    system_prompt="$(cat "$system_prompt_file")"

    local envelope
    envelope="$(cat "$envelope_file")"

    # Count completed tasks
    local completed_count
    completed_count=$(echo "$envelope" | jq '[.tasks[] | select(.status == "completed")] | length')

    if [ "$completed_count" -eq 0 ]; then
        echo "[synth-collab] No tasks completed — skipping synthesis"
        jq -n \
            --arg extra "$extra_instructions" \
            '{
                "summary": "All collaborative tasks failed. No synthesis possible.",
                "result": null,
                "plan_executed": "Task decomposition succeeded but all subtasks failed.",
                "tasks_completed": [],
                "gaps": ["All subtasks failed to produce output"],
                "confidence": "none",
                "warnings": ["Complete failure — no task outputs available"]
            }' > "$output_file"
        return 1
    fi

    echo "[synth-collab] Synthesizing $completed_count task output(s)..."

    local synth_prompt_file
    synth_prompt_file="$(mktemp)"
    trap "rm -f '$synth_prompt_file'" RETURN

    cat > "$synth_prompt_file" <<PROMPT
Here are the results from the AI Council running in collaborative mode. Each agent worked on a different subtask.

## Original Task
$(echo "$envelope" | jq -r '.prompt')

## Decomposition
Pattern: $(echo "$envelope" | jq -r '.pattern // "unknown"')
Description: $(echo "$envelope" | jq -r '.description // "N/A"')

## Task Outputs
$(echo "$envelope" | jq '.')

## Additional Instructions
${extra_instructions}

Assemble all task outputs into a coherent, complete result as specified in your system instructions.
PROMPT

    if cat "$synth_prompt_file" | CLAUDECODE= claude -p \
        --output-format json \
        --system-prompt "$system_prompt" \
        > "$output_file" 2>/dev/null; then
        echo "[synth-collab] Synthesis complete"
        return 0
    else
        echo "[synth-collab] Synthesis failed" >&2
        return 1
    fi
}
