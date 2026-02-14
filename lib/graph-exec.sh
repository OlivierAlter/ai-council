#!/usr/bin/env bash
# graph-exec.sh - DAG execution engine for collaborative mode
# Parses task graphs, resolves dependencies, executes in waves.
# Compatible with bash 3.2+ (macOS default).

set -euo pipefail

# --- Read-only task arrays (populated once by parse_task_graph) ---
TASK_IDS=()
TASK_DESCRIPTIONS=()
TASK_AGENTS=()
TASK_PROMPTS=()
TASK_DEPS=()         # space-separated dependency IDs per task
TASK_ROUNDS=()
TASK_COUNT=0

# --- State file path (set by parse_task_graph) ---
COLLAB_STATE_FILE=""

# ---------------------------------------------------------------------------
# parse_task_graph — read JSON, validate schema + cycles, populate arrays
# ---------------------------------------------------------------------------
parse_task_graph() {
    local graph_file="$1"
    local state_file="$2"

    COLLAB_STATE_FILE="$state_file"

    # Basic JSON validity
    if ! jq '.' "$graph_file" >/dev/null 2>&1; then
        echo "[graph] Error: invalid JSON in task graph" >&2
        return 1
    fi

    # --- Schema validation ---
    local errors=""

    # Required top-level fields
    if ! jq -e '.version' "$graph_file" >/dev/null 2>&1; then
        errors+="Missing 'version' field. "
    fi
    if ! jq -e '.tasks | type == "array"' "$graph_file" >/dev/null 2>&1; then
        errors+="Missing or invalid 'tasks' field (must be array). "
    fi

    local task_count
    task_count=$(jq '.tasks | length' "$graph_file")

    if [ "$task_count" -eq 0 ]; then
        errors+="No tasks in graph. "
    fi

    # Validate each task
    local valid_agents="claude codex gemini vibe"
    local all_ids=""
    local i=0
    while [ $i -lt "$task_count" ]; do
        local tid tagent
        tid=$(jq -r ".tasks[$i].id // empty" "$graph_file")
        tagent=$(jq -r ".tasks[$i].agent // empty" "$graph_file")

        if [ -z "$tid" ]; then
            errors+="Task at index $i missing 'id'. "
        fi
        if [ -z "$tagent" ]; then
            errors+="Task $tid missing 'agent'. "
        elif ! echo "$valid_agents" | grep -qw "$tagent"; then
            errors+="Task $tid has invalid agent '$tagent'. "
        fi
        if ! jq -e ".tasks[$i].prompt" "$graph_file" >/dev/null 2>&1; then
            errors+="Task $tid missing 'prompt'. "
        fi

        # Duplicate ID check
        if [ -n "$all_ids" ] && echo "$all_ids" | grep -qw "$tid" 2>/dev/null; then
            errors+="Duplicate task ID '$tid'. "
        fi
        all_ids+=" $tid"

        i=$((i + 1))
    done

    # Validate dependency references
    i=0
    while [ $i -lt "$task_count" ]; do
        local deps
        deps=$(jq -r ".tasks[$i].depends_on[]?" "$graph_file" 2>/dev/null || true)
        for dep in $deps; do
            if ! echo "$all_ids" | grep -qw "$dep"; then
                local src_id
                src_id=$(jq -r ".tasks[$i].id" "$graph_file")
                errors+="Task $src_id depends on unknown task '$dep'. "
            fi
        done
        i=$((i + 1))
    done

    if [ -n "$errors" ]; then
        echo "[graph] Validation errors: $errors" >&2
        return 1
    fi

    # --- Cycle detection ---
    if ! detect_cycles "$graph_file" "$task_count"; then
        echo "[graph] Error: dependency cycle detected" >&2
        return 1
    fi

    # --- Populate arrays ---
    i=0
    while [ $i -lt "$task_count" ]; do
        TASK_IDS+=("$(jq -r ".tasks[$i].id" "$graph_file")")
        TASK_DESCRIPTIONS+=("$(jq -r ".tasks[$i].description // \"\"" "$graph_file")")
        TASK_AGENTS+=("$(jq -r ".tasks[$i].agent" "$graph_file")")
        TASK_PROMPTS+=("$(jq -r ".tasks[$i].prompt" "$graph_file")")
        TASK_DEPS+=("$(jq -r '.tasks['"$i"'].depends_on | join(" ")' "$graph_file")")
        TASK_ROUNDS+=("$(jq -r ".tasks[$i].round // 0" "$graph_file")")

        # Initialize state
        append_state "${TASK_IDS[$i]}" "pending" "${TASK_AGENTS[$i]}"

        i=$((i + 1))
    done

    TASK_COUNT=$task_count
    echo "[graph] Parsed $TASK_COUNT tasks"
}

# ---------------------------------------------------------------------------
# detect_cycles — Kahn's algorithm (topological sort)
# Returns 0 if no cycles, 1 if cycle found.
# ---------------------------------------------------------------------------
detect_cycles() {
    local graph_file="$1"
    local task_count="$2"

    # Use temp files for in-degree tracking (bash 3.2 compatible — no assoc arrays)
    local tmp_dir
    tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/council-topo.XXXXXX")

    local i=0
    while [ $i -lt "$task_count" ]; do
        local tid dep_count
        tid=$(jq -r ".tasks[$i].id" "$graph_file")
        dep_count=$(jq -r ".tasks[$i].depends_on | length" "$graph_file")
        echo "$dep_count" > "$tmp_dir/indegree-$tid"

        # Record adjacency: for each dep, this task is a successor
        local j=0
        while [ $j -lt "$dep_count" ]; do
            local dep
            dep=$(jq -r ".tasks[$i].depends_on[$j]" "$graph_file")
            echo "$tid" >> "$tmp_dir/adj-$dep"
            j=$((j + 1))
        done

        i=$((i + 1))
    done

    # Seed queue with zero-indegree nodes
    local queue=""
    local visited=0
    i=0
    while [ $i -lt "$task_count" ]; do
        local tid indeg
        tid=$(jq -r ".tasks[$i].id" "$graph_file")
        indeg=$(cat "$tmp_dir/indegree-$tid")
        if [ "$indeg" -eq 0 ]; then
            queue+=" $tid"
        fi
        i=$((i + 1))
    done

    # Process queue (BFS)
    while [ -n "$(echo "$queue" | xargs)" ]; do
        # Pop first element
        local current
        current=$(echo "$queue" | awk '{print $1}')
        queue=$(echo "$queue" | awk '{for(i=2;i<=NF;i++) printf "%s ", $i}')
        visited=$((visited + 1))

        # Reduce in-degree of successors
        if [ -f "$tmp_dir/adj-$current" ]; then
            while IFS= read -r neighbor; do
                local indeg
                indeg=$(cat "$tmp_dir/indegree-$neighbor")
                indeg=$((indeg - 1))
                echo "$indeg" > "$tmp_dir/indegree-$neighbor"
                if [ "$indeg" -eq 0 ]; then
                    queue+=" $neighbor"
                fi
            done < "$tmp_dir/adj-$current"
        fi
    done

    rm -rf "$tmp_dir"

    if [ "$visited" -ne "$task_count" ]; then
        return 1  # Cycle detected
    fi
    return 0
}

# ---------------------------------------------------------------------------
# JSONL state file operations
# ---------------------------------------------------------------------------

# Append a state entry
append_state() {
    local task_id="$1"
    local status="$2"
    local agent="${3:-}"
    local output_file="${4:-}"

    local ts
    ts="$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)"

    jq -cn \
        --arg tid "$task_id" \
        --arg s "$status" \
        --arg a "$agent" \
        --arg f "$output_file" \
        --arg ts "$ts" \
        '{task_id:$tid, status:$s, agent:$a, output_file:$f, ts:$ts}' >> "$COLLAB_STATE_FILE"
}

# Read latest status for a task
read_task_status() {
    local task_id="$1"
    grep "\"task_id\":\"${task_id}\"" "$COLLAB_STATE_FILE" 2>/dev/null | tail -1 | jq -r '.status' 2>/dev/null || echo "unknown"
}

# Read latest output file path for a task
read_task_output_file() {
    local task_id="$1"
    grep "\"task_id\":\"${task_id}\"" "$COLLAB_STATE_FILE" 2>/dev/null | tail -1 | jq -r '.output_file' 2>/dev/null || echo ""
}

# ---------------------------------------------------------------------------
# find_ready_tasks — indices of tasks whose deps are all completed
# ---------------------------------------------------------------------------
find_ready_tasks() {
    local ready=""
    local i=0
    while [ $i -lt "$TASK_COUNT" ]; do
        local tid="${TASK_IDS[$i]}"
        local status
        status=$(read_task_status "$tid")

        if [ "$status" = "pending" ]; then
            local deps="${TASK_DEPS[$i]}"
            local all_deps_met=true

            if [ -n "$deps" ]; then
                for dep in $deps; do
                    local dep_status
                    dep_status=$(read_task_status "$dep")
                    if [ "$dep_status" != "completed" ]; then
                        all_deps_met=false
                        break
                    fi
                done
            fi

            if [ "$all_deps_met" = true ]; then
                ready+="$i "
            fi
        fi
        i=$((i + 1))
    done
    echo "$ready" | xargs
}

# ---------------------------------------------------------------------------
# execute_wave — launch ready tasks in parallel, wait for all
# ---------------------------------------------------------------------------
execute_wave() {
    local wave_indices="$1"
    local collab_dir="$2"
    local workspace="$3"
    local tui_mode="$4"

    local wave_pids=()
    local wave_idx_arr=()

    for idx in $wave_indices; do
        wave_idx_arr+=("$idx")
        local tid="${TASK_IDS[$idx]}"
        local agent="${TASK_AGENTS[$idx]}"
        local output_file="${collab_dir}/${tid}-result.txt"
        local log_file="${collab_dir}/${tid}.log"
        local prompt_file="${collab_dir}/${tid}-prompt.txt"

        append_state "$tid" "running" "$agent"

        echo "  [wave] Running ${tid} (${agent})"

        run_agent "$agent" "$workspace" "" "$output_file" "$log_file" "$tui_mode" "$prompt_file" &
        wave_pids+=($!)
    done

    # Wait for all tasks in this wave
    local j=0
    while [ $j -lt ${#wave_pids[@]} ]; do
        local idx="${wave_idx_arr[$j]}"
        local tid="${TASK_IDS[$idx]}"
        local agent="${TASK_AGENTS[$idx]}"
        local pid="${wave_pids[$j]}"
        local output_file="${collab_dir}/${tid}-result.txt"

        if wait "$pid" 2>/dev/null; then
            append_state "$tid" "completed" "$agent" "$output_file"
            echo -e "  ${GREEN:-}✓${NC:-} ${tid} (${agent}) completed"
        else
            append_state "$tid" "failed" "$agent" "$output_file"
            echo -e "  ${RED:-}✗${NC:-} ${tid} (${agent}) failed"
        fi
        j=$((j + 1))
    done
}

# ---------------------------------------------------------------------------
# is_graph_complete — true if all tasks are completed or failed
# ---------------------------------------------------------------------------
is_graph_complete() {
    local i=0
    while [ $i -lt "$TASK_COUNT" ]; do
        local status
        status=$(read_task_status "${TASK_IDS[$i]}")
        if [ "$status" = "pending" ] || [ "$status" = "running" ]; then
            return 1
        fi
        i=$((i + 1))
    done
    return 0
}

# ---------------------------------------------------------------------------
# has_deadlock — true if pending tasks exist but none are ready
# ---------------------------------------------------------------------------
has_deadlock() {
    local has_pending=false
    local i=0
    while [ $i -lt "$TASK_COUNT" ]; do
        local status
        status=$(read_task_status "${TASK_IDS[$i]}")
        if [ "$status" = "pending" ]; then
            has_pending=true
            break
        fi
        i=$((i + 1))
    done

    if [ "$has_pending" = true ]; then
        local ready
        ready=$(find_ready_tasks)
        if [ -z "$ready" ]; then
            return 0  # Deadlock: pending tasks but none ready
        fi
    fi
    return 1
}
