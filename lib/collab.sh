#!/usr/bin/env bash
# collab.sh - Collaborative mode orchestrator
# Decomposes tasks via a planner, executes subtasks across agents with
# dependency resolution, and synthesizes the final result.
#
# Compatible with bash 3.2+ (macOS default).

set -euo pipefail

COLLAB_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Source the graph execution engine
source "${COLLAB_SCRIPT_DIR}/lib/graph-exec.sh"

# ---------------------------------------------------------------------------
# sanitize_output — strip potential prompt injection from agent output
# ---------------------------------------------------------------------------
sanitize_output() {
    local text="$1"
    echo "$text" | sed \
        -e 's/<system>.*<\/system>//g' \
        -e '/<|system|>/d' \
        -e '/^SYSTEM:/d' \
        -e '/^### System Prompt/d' \
        -e '/^You are now /d' \
        -e '/^Ignore previous instructions/d' \
        -e '/^Ignore all previous/d' \
        -e '/^Forget everything/d' \
        -e '/^Disregard all prior/d'
}

# ---------------------------------------------------------------------------
# run_planner — invoke Claude on host to generate the task graph
# ---------------------------------------------------------------------------
run_planner() {
    local prompt="$1"
    local graph_file="$2"
    local pattern="${3:-}"
    local agents="$4"
    local system_prompt_file="${COLLAB_SCRIPT_DIR}/prompts/planner-system.md"

    if [ ! -f "$system_prompt_file" ]; then
        echo "[planner] Error: planner system prompt not found at $system_prompt_file" >&2
        return 1
    fi

    local system_prompt
    system_prompt="$(cat "$system_prompt_file")"

    # Build planner prompt
    local planner_prompt_file
    planner_prompt_file="$(mktemp)"

    local pattern_instruction=""
    if [ -n "$pattern" ]; then
        pattern_instruction="IMPORTANT: You MUST use the '${pattern}' coordination pattern."
    fi

    cat > "$planner_prompt_file" <<PROMPT
Decompose the following task into subtasks for the AI Council collaborative mode.

## Task
${prompt}

## Available Agents
${agents}

${pattern_instruction}

Produce the task graph JSON as specified in your system instructions. Output ONLY the JSON object — no markdown fences, no explanation.
PROMPT

    echo "[planner] Generating task graph..."

    # Run Claude on host to produce the task graph
    if cat "$planner_prompt_file" | CLAUDECODE= claude -p \
        --output-format json \
        --system-prompt "$system_prompt" \
        > "$graph_file" 2>/dev/null; then

        rm -f "$planner_prompt_file"

        # Handle potential wrapper format: if output has .result but no .tasks,
        # the actual graph is inside the result field
        if jq -e '.result' "$graph_file" >/dev/null 2>&1 && ! jq -e '.tasks' "$graph_file" >/dev/null 2>&1; then
            local inner
            inner=$(jq -r '.result' "$graph_file")
            echo "$inner" | jq '.' > "$graph_file" 2>/dev/null || {
                echo "[planner] Failed to extract task graph from wrapper" >&2
                return 1
            }
        fi

        # Validate the output is valid JSON with tasks
        if jq -e '.tasks' "$graph_file" >/dev/null 2>&1; then
            echo "[planner] Task graph generated"
            return 0
        fi
    fi

    rm -f "$planner_prompt_file"
    echo "[planner] Failed to generate valid task graph" >&2
    return 1
}

# ---------------------------------------------------------------------------
# build_task_prompt — assemble augmented prompt with dependency context
# Writes to collab/<task-id>-prompt.txt
# ---------------------------------------------------------------------------
build_task_prompt() {
    local task_idx="$1"
    local collab_dir="$2"
    local prompt_file="${collab_dir}/${TASK_IDS[$task_idx]}-prompt.txt"

    local task_prompt="${TASK_PROMPTS[$task_idx]}"
    local deps="${TASK_DEPS[$task_idx]}"

    if [ -z "$deps" ]; then
        # No dependencies — use prompt as-is
        echo "$task_prompt" > "$prompt_file"
        return
    fi

    # Build context from dependency outputs
    local context=""
    for dep_id in $deps; do
        local dep_output_file
        dep_output_file=$(read_task_output_file "$dep_id")
        local dep_status
        dep_status=$(read_task_status "$dep_id")

        if [ "$dep_status" = "completed" ] && [ -n "$dep_output_file" ] && [ -f "$dep_output_file" ] && [ -s "$dep_output_file" ]; then
            # Find the agent for this dependency
            local dep_agent=""
            local di=0
            while [ $di -lt "$TASK_COUNT" ]; do
                if [ "${TASK_IDS[$di]}" = "$dep_id" ]; then
                    dep_agent="${TASK_AGENTS[$di]}"
                    break
                fi
                di=$((di + 1))
            done

            # Extract readable text from the agent output
            local dep_json="${collab_dir}/${dep_id}.json"
            extract_json "$dep_agent" "$dep_output_file" > "$dep_json" 2>/dev/null || true
            local dep_text
            dep_text=$(extract_result_text "$dep_agent" "$dep_json" 2>/dev/null || true)

            if [ -z "$dep_text" ] || [ "$dep_text" = "null" ]; then
                dep_text="$(cat "$dep_output_file")"
            fi

            # Truncate if too long
            local max_chars="${COLLAB_MAX_CONTEXT_CHARS:-50000}"
            if [ ${#dep_text} -gt "$max_chars" ]; then
                dep_text="${dep_text:0:$max_chars}
...[truncated at ${max_chars} chars]"
            fi

            # Sanitize to prevent prompt injection
            dep_text=$(sanitize_output "$dep_text")

            context+="
--- Output from ${dep_id} (${dep_agent}) ---
${dep_text}
--- End ${dep_id} ---
"
        else
            context+="
--- Output from ${dep_id} ---
[NOTE: This dependency failed or produced no output. Proceed as best you can without it.]
--- End ${dep_id} ---
"
        fi
    done

    # Write augmented prompt to file
    cat > "$prompt_file" <<AUGMENTED
## Context from Previous Tasks

The following outputs were produced by earlier tasks in the pipeline. Use them as context for your work.
${context}

## Your Task

${task_prompt}
AUGMENTED
}

# ---------------------------------------------------------------------------
# generate_review_round — create round-N tasks from round N-1 outputs
# Returns 1 to signal early termination (output hash unchanged).
# ---------------------------------------------------------------------------
generate_review_round() {
    local round_num="$1"
    local collab_dir="$2"

    # Collect outputs from previous round
    local prev_round=$((round_num - 1))
    local prev_outputs=""
    local i=0
    while [ $i -lt "$TASK_COUNT" ]; do
        if [ "${TASK_ROUNDS[$i]}" = "$prev_round" ]; then
            local tid="${TASK_IDS[$i]}"
            local agent="${TASK_AGENTS[$i]}"
            local result_file="${collab_dir}/${tid}-result.txt"

            if [ -f "$result_file" ] && [ -s "$result_file" ]; then
                local dep_json="${collab_dir}/${tid}.json"
                extract_json "$agent" "$result_file" > "$dep_json" 2>/dev/null || true
                local text
                text=$(extract_result_text "$agent" "$dep_json" 2>/dev/null || cat "$result_file")
                text=$(sanitize_output "$text")

                prev_outputs+="
--- Round ${prev_round} output from ${agent} (${tid}) ---
${text}
--- End ---
"
            fi
        fi
        i=$((i + 1))
    done

    # Early termination: check if outputs are the same as last round
    local round_hash
    round_hash=$(echo "$prev_outputs" | shasum 2>/dev/null | awk '{print $1}' || echo "none")
    local prev_hash_file="${collab_dir}/round-${prev_round}-hash.txt"
    if [ -f "$prev_hash_file" ]; then
        local prev_hash
        prev_hash=$(cat "$prev_hash_file")
        if [ "$round_hash" = "$prev_hash" ]; then
            echo "[collab] Round ${round_num}: outputs unchanged from previous round, stopping early"
            return 1
        fi
    fi
    echo "$round_hash" > "${collab_dir}/round-${round_num}-hash.txt"

    # Create review tasks for this round
    local base_count=$TASK_COUNT
    i=0
    while [ $i -lt "$base_count" ]; do
        if [ "${TASK_ROUNDS[$i]}" = "$prev_round" ]; then
            local new_id="task-r${round_num}-${TASK_AGENTS[$i]}"
            local new_prompt="Review and improve the following outputs from round ${prev_round}. Identify strengths, weaknesses, and produce an improved version.
${prev_outputs}

Original task: ${TASK_PROMPTS[$i]}

Produce an improved response that addresses any weaknesses you find."

            TASK_IDS+=("$new_id")
            TASK_DESCRIPTIONS+=("Round ${round_num} review by ${TASK_AGENTS[$i]}")
            TASK_AGENTS+=("${TASK_AGENTS[$i]}")
            TASK_PROMPTS+=("$new_prompt")
            TASK_DEPS+=("")  # Review tasks within the same round are independent
            TASK_ROUNDS+=("$round_num")
            TASK_COUNT=$((TASK_COUNT + 1))

            append_state "$new_id" "pending" "${TASK_AGENTS[$i]}"
        fi
        i=$((i + 1))
    done
}

# ---------------------------------------------------------------------------
# handle_task_failure — retry same agent, then reassign, then skip
# ---------------------------------------------------------------------------
handle_task_failure() {
    local task_idx="$1"
    local collab_dir="$2"
    local workspace="$3"
    local tui_mode="$4"

    local tid="${TASK_IDS[$task_idx]}"
    local agent="${TASK_AGENTS[$task_idx]}"
    local output_file="${collab_dir}/${tid}-result.txt"
    local log_file="${collab_dir}/${tid}.log"
    local prompt_file="${collab_dir}/${tid}-prompt.txt"

    # Step 1: Retry same agent once
    echo "[collab] Retrying ${tid} with ${agent}..."
    append_state "$tid" "running" "$agent"

    if run_agent "$agent" "$workspace" "" "$output_file" "$log_file" "$tui_mode" "$prompt_file"; then
        append_state "$tid" "completed" "$agent" "$output_file"
        echo -e "  ${GREEN:-}✓${NC:-} ${tid} (${agent}) succeeded on retry"
        return 0
    fi

    # Step 2: Try alternative agents
    local alt_agents="claude codex gemini vibe"
    for alt in $alt_agents; do
        if [ "$alt" != "$agent" ]; then
            echo "[collab] Reassigning ${tid} to ${alt}..."
            append_state "$tid" "running" "$alt"

            if run_agent "$alt" "$workspace" "" "$output_file" "$log_file" "$tui_mode" "$prompt_file"; then
                append_state "$tid" "completed" "$alt" "$output_file"
                echo -e "  ${GREEN:-}✓${NC:-} ${tid} (${alt}) succeeded after reassignment"
                return 0
            fi
        fi
    done

    # Step 3: Give up — mark failed permanently
    append_state "$tid" "failed" "$agent"
    echo -e "  ${RED:-}✗${NC:-} ${tid} permanently failed after retry and reassignment"
    return 1
}

# ---------------------------------------------------------------------------
# build_collab_envelope — combine all task outputs into envelope JSON
# ---------------------------------------------------------------------------
build_collab_envelope() {
    local collab_dir="$1"
    local prompt="$2"
    local graph_file="$3"

    local envelope
    envelope=$(jq -n \
        --arg prompt "$prompt" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --arg pattern "$(jq -r '.pattern // "unknown"' "$graph_file")" \
        --arg description "$(jq -r '.description // ""' "$graph_file")" \
        '{
            mode: "collaborative",
            prompt: $prompt,
            timestamp: $timestamp,
            pattern: $pattern,
            description: $description,
            tasks: []
        }')

    local i=0
    while [ $i -lt "$TASK_COUNT" ]; do
        local tid="${TASK_IDS[$i]}"
        local agent="${TASK_AGENTS[$i]}"
        local status
        status=$(read_task_status "$tid")
        local output_file="${collab_dir}/${tid}-result.txt"

        local task_output="null"
        if [ -f "$output_file" ] && [ -s "$output_file" ]; then
            local json_file="${collab_dir}/${tid}.json"
            extract_json "$agent" "$output_file" > "$json_file" 2>/dev/null || true
            if [ -f "$json_file" ] && jq '.' "$json_file" >/dev/null 2>&1; then
                task_output=$(jq -c '.' "$json_file")
            fi
        fi

        envelope=$(echo "$envelope" | jq \
            --arg tid "$tid" \
            --arg agent "$agent" \
            --arg status "$status" \
            --arg desc "${TASK_DESCRIPTIONS[$i]}" \
            --argjson output "${task_output}" \
            '.tasks += [{
                task_id: $tid,
                agent: $agent,
                status: $status,
                description: $desc,
                output: $output
            }]')

        i=$((i + 1))
    done

    echo "$envelope"
}

# ---------------------------------------------------------------------------
# run_collab — main entry point for collaborative mode
# ---------------------------------------------------------------------------
run_collab() {
    local prompt="$1"
    local pattern="${COLLAB_PATTERN:-}"
    local max_rounds="${COLLAB_ROUNDS:-${COLLAB_MAX_ROUNDS:-2}}"
    local graph_file_arg="${COLLAB_GRAPH:-}"
    local dry_run="${COLLAB_DRY_RUN:-false}"

    local collab_dir="${SESSION_DIR}/collab"
    mkdir -p "$collab_dir"

    local graph_file="${collab_dir}/taskgraph.json"
    local state_file="${collab_dir}/state.jsonl"
    : > "$state_file"  # Initialize empty state file

    # =======================================================================
    # PHASE 1: PLAN
    # =======================================================================
    echo -e "${BOLD}Phase 1: Planning...${NC}"

    if [ -n "$graph_file_arg" ]; then
        # Use pre-built graph
        if [ ! -f "$graph_file_arg" ]; then
            echo -e "${RED}Error:${NC} Graph file not found: $graph_file_arg"
            return 1
        fi
        cp "$graph_file_arg" "$graph_file"
        echo "[collab] Using pre-built task graph: $graph_file_arg"
    else
        # Run planner
        if ! run_planner "$prompt" "$graph_file" "$pattern" "$ACTIVE_AGENTS"; then
            echo -e "${YELLOW}Planner failed. Falling back to standard council mode.${NC}"
            return 1  # Caller handles fallback
        fi
    fi

    # Parse and validate the graph
    if ! parse_task_graph "$graph_file" "$state_file"; then
        echo -e "${YELLOW}Invalid task graph. Falling back to standard council mode.${NC}"
        return 1
    fi

    # Display the plan
    echo ""
    local detected_pattern
    detected_pattern=$(jq -r '.pattern // "unknown"' "$graph_file")
    echo -e "${BOLD}Task Graph:${NC}"
    echo -e "  Pattern: ${detected_pattern}"
    echo -e "  Tasks:   ${TASK_COUNT}"

    local i=0
    while [ $i -lt "$TASK_COUNT" ]; do
        local deps_display="${TASK_DEPS[$i]}"
        if [ -z "$deps_display" ]; then deps_display="none"; fi
        echo -e "    ${TASK_IDS[$i]} -> ${TASK_AGENTS[$i]}: ${TASK_DESCRIPTIONS[$i]} ${DIM}(deps: ${deps_display})${NC}"
        i=$((i + 1))
    done
    echo ""

    # Dry run: print graph and exit
    if [ "$dry_run" = "true" ]; then
        echo -e "${BOLD}Dry run — task graph JSON:${NC}"
        jq '.' "$graph_file"
        return 0
    fi

    # =======================================================================
    # PHASE 2: EXECUTE GRAPH
    # =======================================================================
    echo -e "${BOLD}Phase 2: Executing task graph...${NC}"

    local wave_num=1
    while true; do
        if is_graph_complete; then
            echo "[collab] All tasks complete"
            break
        fi

        if has_deadlock; then
            echo -e "${RED}[collab] Deadlock detected — pending tasks with unresolvable dependencies${NC}"
            break
        fi

        local ready
        ready=$(find_ready_tasks)

        if [ -z "$ready" ]; then
            # This shouldn't happen if has_deadlock is correct, but guard against it
            sleep 1
            continue
        fi

        echo ""
        echo -e "${BOLD}Wave ${wave_num}:${NC}"

        # Build prompts for ready tasks (with dependency context)
        for idx in $ready; do
            build_task_prompt "$idx" "$collab_dir"
        done

        # Execute wave
        execute_wave "$ready" "$collab_dir" "$WORKSPACE" "$TUI_MODE"

        # Handle failures: retry → reassign → skip
        for idx in $ready; do
            local tid="${TASK_IDS[$idx]}"
            local status
            status=$(read_task_status "$tid")
            if [ "$status" = "failed" ]; then
                handle_task_failure "$idx" "$collab_dir" "$WORKSPACE" "$TUI_MODE" || true
            fi
        done

        wave_num=$((wave_num + 1))
    done

    # --- Iterative rounds ---
    if [ "$detected_pattern" = "iterative" ]; then
        local round=1
        while [ "$round" -lt "$max_rounds" ]; do
            echo ""
            echo -e "${BOLD}Iterative Round ${round}:${NC}"

            if ! generate_review_round "$round" "$collab_dir"; then
                echo "[collab] Early termination — outputs converged"
                break
            fi

            # Execute the new round's tasks
            while true; do
                if is_graph_complete; then break; fi
                if has_deadlock; then break; fi

                local ready
                ready=$(find_ready_tasks)
                if [ -z "$ready" ]; then break; fi

                for idx in $ready; do
                    build_task_prompt "$idx" "$collab_dir"
                done

                execute_wave "$ready" "$collab_dir" "$WORKSPACE" "$TUI_MODE"
            done

            round=$((round + 1))
        done
    fi

    echo ""

    # =======================================================================
    # PHASE 3: FINAL SYNTHESIS
    # =======================================================================
    echo -e "${BOLD}Phase 3: Synthesizing collaborative outputs...${NC}"

    local envelope_file="${collab_dir}/envelope.json"
    build_collab_envelope "$collab_dir" "$prompt" "$graph_file" > "$envelope_file"

    local synth_file="${SESSION_DIR}/synthesis.json"
    local synthesis_prompt
    synthesis_prompt=$(jq -r '.final_synthesis_prompt // "Combine all task outputs into a coherent final answer."' "$graph_file")

    emit_event "synthesis.started" "{}"

    if run_collab_synthesis "$envelope_file" "$synth_file" "$synthesis_prompt"; then
        local synth_result
        synth_result=$(cat "$synth_file" 2>/dev/null | jq -c . || echo "null")
        local synth_comp_data
        synth_comp_data=$(jq -cn --argjson r "$synth_result" '{"result":$r}')
        emit_event "synthesis.completed" "$synth_comp_data"
        echo ""
        echo -e "${BOLD}${GREEN}=== Council Collaborative Result ===${NC}"
        echo ""
        jq '.' "$synth_file" 2>/dev/null || cat "$synth_file"
    else
        emit_event "synthesis.completed" '{"result":null}'
        echo -e "${YELLOW}Synthesis failed. Showing collab envelope:${NC}"
        jq '.' "$envelope_file"
    fi

    # --- Summary ---
    echo ""
    echo -e "${DIM}────────────────────────────────────────${NC}"
    echo -e "  Session:    ${SESSION_DIR}"
    echo -e "  Collab dir: ${collab_dir}"
    echo -e "  Pattern:    ${detected_pattern}"

    i=0
    while [ $i -lt "$TASK_COUNT" ]; do
        local status
        status=$(read_task_status "${TASK_IDS[$i]}")
        local icon
        case "$status" in
            completed) icon="${GREEN}✓${NC}" ;;
            failed)    icon="${RED}✗${NC}" ;;
            *)         icon="${YELLOW}?${NC}" ;;
        esac
        echo -e "  ${icon} ${TASK_IDS[$i]} (${TASK_AGENTS[$i]}): ${status}"
        i=$((i + 1))
    done

    echo -e "${DIM}────────────────────────────────────────${NC}"
    echo ""

    return 0
}
