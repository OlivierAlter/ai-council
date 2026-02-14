#!/usr/bin/env bash
# output-collector.sh - JSON extraction and validation from agent outputs

set -euo pipefail

# Extract valid JSON from agent stdout
# Different agents format output differently:
#   Claude/Gemini: single JSON object - extract last valid {...} block
#   Codex: JSONL - filter for final message event
extract_json() {
    local agent="$1"
    local raw_file="$2"

    if [ ! -f "$raw_file" ] || [ ! -s "$raw_file" ]; then
        echo '{"error": "no_output", "agent": "'"$agent"'"}'
        return 1
    fi

    local content
    content="$(cat "$raw_file")"

    case "$agent" in
        claude|gemini|vibe)
            # Extract the last valid JSON object from output
            # Agent may print status lines before/after JSON
            extract_last_json_object "$content"
            ;;
        codex)
            # Codex outputs JSONL - get the last complete JSON line
            extract_codex_result "$content"
            ;;
        *)
            echo '{"error": "unknown_agent", "agent": "'"$agent"'"}'
            return 1
            ;;
    esac
}

extract_last_json_object() {
    local content="$1"

    # Try parsing the entire content as JSON first
    if echo "$content" | jq '.' 2>/dev/null; then
        return 0
    fi

    # Find lines that could be JSON object boundaries and extract
    # the last valid JSON block
    local json_block=""
    local in_json=false
    local depth=0
    local current_block=""

    while IFS= read -r line; do
        if [ "$in_json" = false ]; then
            if echo "$line" | grep -q '^{'; then
                in_json=true
                depth=0
                current_block=""
            fi
        fi

        if [ "$in_json" = true ]; then
            current_block+="$line"$'\n'
            # Count braces (rough heuristic)
            local opens closes
            opens=$(echo "$line" | tr -cd '{' | wc -c)
            closes=$(echo "$line" | tr -cd '}' | wc -c)
            depth=$((depth + opens - closes))

            if [ $depth -le 0 ]; then
                # Try to validate this block
                if echo "$current_block" | jq '.' 2>/dev/null >/dev/null; then
                    json_block="$current_block"
                fi
                in_json=false
                current_block=""
            fi
        fi
    done <<< "$content"

    if [ -n "$json_block" ]; then
        echo "$json_block" | jq '.'
        return 0
    fi

    # Last resort: try to find any JSON-like content
    echo '{"raw_output": '"$(echo "$content" | jq -Rs '.')"'}'
}

extract_codex_result() {
    local content="$1"

    # Codex JSONL event stream:
    #   thread.started  - session info
    #   turn.started    - turn begins
    #   item.completed  - type:"reasoning" (CoT) or type:"agent_message" (actual answer)
    #   turn.completed  - usage stats only (no answer text)
    #
    # We want the last item.completed with type:"agent_message"
    local agent_message=""
    local turn_completed=""
    while IFS= read -r line; do
        if echo "$line" | jq -e '.type == "item.completed" and .item.type == "agent_message"' 2>/dev/null >/dev/null; then
            agent_message="$line"
        elif echo "$line" | jq -e '.type == "turn.completed"' 2>/dev/null >/dev/null; then
            turn_completed="$line"
        fi
    done <<< "$content"

    if [ -n "$agent_message" ]; then
        # Merge the agent_message text with usage from turn.completed
        local text usage
        text=$(echo "$agent_message" | jq -r '.item.text')
        if [ -n "$turn_completed" ]; then
            usage=$(echo "$turn_completed" | jq '.usage')
        else
            usage='null'
        fi
        jq -n \
            --arg text "$text" \
            --argjson usage "$usage" \
            '{agent: "codex", result: $text, usage: $usage}'
        return 0
    fi

    # Fallback: return the last valid JSON line
    local last_valid=""
    while IFS= read -r line; do
        if echo "$line" | jq '.' 2>/dev/null >/dev/null; then
            last_valid="$line"
        fi
    done <<< "$content"

    if [ -n "$last_valid" ]; then
        echo "$last_valid" | jq '.'
        return 0
    fi

    echo '{"raw_output": '"$(echo "$content" | jq -Rs '.')"'}'
}

# Extract human-readable text from agent output JSON
# Used by collab mode to pass context between dependent tasks
extract_result_text() {
    local agent="$1"
    local json_file="$2"

    if [ ! -f "$json_file" ] || [ ! -s "$json_file" ]; then
        echo ""
        return 1
    fi

    local text=""
    case "$agent" in
        claude)
            text=$(jq -r '.result // .response // .raw_output // empty' "$json_file" 2>/dev/null)
            ;;
        codex)
            text=$(jq -r '.result // .raw_output // empty' "$json_file" 2>/dev/null)
            ;;
        gemini)
            text=$(jq -r '.response // .result // .raw_output // empty' "$json_file" 2>/dev/null)
            ;;
        vibe)
            # Vibe may output a messages array — get last assistant message
            text=$(jq -r '
                if type == "array" then
                    [.[] | select(.role == "assistant")] | last | .content
                else
                    .result // .response // .raw_output // empty
                end
            ' "$json_file" 2>/dev/null)
            ;;
        *)
            text=$(jq -r '.result // .response // .raw_output // empty' "$json_file" 2>/dev/null)
            ;;
    esac

    if [ -z "$text" ] || [ "$text" = "null" ]; then
        # Fallback: stringify the whole JSON
        text=$(jq -r 'tostring' "$json_file" 2>/dev/null || cat "$json_file")
    fi

    echo "$text"
}

# Validate and classify agent result
validate_result() {
    local agent="$1"
    local json_file="$2"

    if [ ! -f "$json_file" ] || [ ! -s "$json_file" ]; then
        echo "error"
        return
    fi

    # Check if it's valid JSON
    if ! jq '.' "$json_file" >/dev/null 2>&1; then
        echo "error"
        return
    fi

    # Check for error markers
    if jq -e '.error' "$json_file" >/dev/null 2>&1; then
        local err
        err=$(jq -r '.error' "$json_file")
        if [ "$err" = "timeout" ]; then
            echo "timeout"
        else
            echo "error"
        fi
        return
    fi

    echo "success"
}

# Build the combined envelope JSON from all agent results
build_envelope() {
    local session_dir="$1"
    local prompt="$2"
    shift 2
    local agents=("$@")

    local envelope
    envelope=$(jq -n \
        --arg prompt "$prompt" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{
            prompt: $prompt,
            timestamp: $timestamp,
            agents: {}
        }')

    for agent in "${agents[@]}"; do
        local result_file="${session_dir}/${agent}.json"
        local status
        status=$(validate_result "$agent" "$result_file")

        local agent_data
        if [ -f "$result_file" ] && [ -s "$result_file" ] && jq '.' "$result_file" >/dev/null 2>&1; then
            agent_data=$(jq -n \
                --arg status "$status" \
                --slurpfile output "$result_file" \
                '{status: $status, output: $output[0]}')
        else
            agent_data=$(jq -n \
                --arg status "$status" \
                '{status: $status, output: null}')
        fi

        envelope=$(echo "$envelope" | jq \
            --arg agent "$agent" \
            --argjson data "$agent_data" \
            '.agents[$agent] = $data')
    done

    echo "$envelope"
}
