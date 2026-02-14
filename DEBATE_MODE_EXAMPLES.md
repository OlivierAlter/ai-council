# Debate Mode — Concrete Examples

This document provides **working examples** of how Debate Mode operates in practice, with actual prompts, message flows, and expected outputs.

---

## Example 1: Code Review Pipeline (Sequential Pattern)

### User Command

```bash
./council.sh "Review and improve the authentication module in auth.py" \
  --debate \
  --pattern sequential \
  --rounds 4
```

### Execution Flow

#### Round 0: Claude (Proposer)

**Prompt sent to Claude**:
```
You are participating in a debate. Your role is to propose a solution to the following task.

**Task**: Review and improve the authentication module in auth.py

Provide your best solution.
```

**Claude's response**:
```
I've reviewed auth.py and identified 3 security issues:

1. Password hashing uses MD5 (line 34) — should use bcrypt
2. No rate limiting on login attempts
3. Session tokens stored in plain text

I'll implement fixes:

[Edit auth.py to use bcrypt]
[Add rate limiter middleware]
[Encrypt session tokens with AES-256]

All tests passing. Changes ready for review.
```

#### Round 1: Codex (Reviewer)

**Prompt sent to Codex**:
```
You are participating in a debate (Round 1). Your role is to review Claude's work and provide constructive feedback.

**Original Task**: Review and improve the authentication module in auth.py

**Claude's Work**:
I've reviewed auth.py and identified 3 security issues:
[... full response ...]

Provide specific feedback: strengths, weaknesses, and suggested improvements.
```

**Codex's response**:
```
Strong improvements overall. However:

**Strengths**:
✓ Correct identification of MD5 weakness
✓ Bcrypt is the right choice

**Weaknesses**:
✗ Rate limiter implementation has a race condition (line 67)
✗ AES-256 key is hardcoded — should use env var
✗ Missing unit tests for new code

**Suggested improvements**:
1. Use atomic Redis operations for rate limiter
2. Load encryption key from process.env.SESSION_KEY
3. Add test coverage for bcrypt migration

[Read auth.py]
[Implement fixes for race condition]
[Add tests]
```

#### Round 2: Gemini (Reviewer)

**Prompt sent to Gemini**:
```
You are participating in a debate (Round 2). Your role is to review Codex's work and provide constructive feedback.

**Original Task**: Review and improve the authentication module in auth.py

**Codex's Work**:
Strong improvements overall. However:
[... full response ...]

Provide specific feedback: strengths, weaknesses, and suggested improvements.
```

**Gemini's response**:
```
Excellent catch on the race condition. Additional considerations:

**Performance**:
- Bcrypt work factor of 12 may be too high for high-traffic apps
- Consider async hashing to avoid blocking event loop

**Security**:
- Session token rotation missing — add on privilege escalation
- Consider adding 2FA support hooks

**Observability**:
- Add metrics for failed login attempts
- Log suspicious patterns (timing attacks)

I recommend:
[Edit auth.py to use async bcrypt]
[Add session rotation logic]
[Implement metrics collection]

Performance benchmarks show 40% latency improvement with async.
```

#### Round 3: Vibe (Synthesizer)

**Prompt sent to Vibe**:
```
You are the final synthesizer in a debate. Your role is to merge all perspectives into a coherent, optimal solution.

**Original Task**: Review and improve the authentication module in auth.py

**All Agent Contributions**:

## Claude
I've reviewed auth.py and identified 3 security issues:
[...]

## Codex
Strong improvements overall. However:
[...]

## Gemini
Excellent catch on the race condition. Additional considerations:
[...]

Synthesize the best elements from each contribution into a final answer.
```

**Vibe's response**:
```
# Final Synthesis: Authentication Module Improvements

## Implemented Changes

### Security (Claude + Codex)
✓ Replaced MD5 with bcrypt (work factor: 10 for balance)
✓ Fixed rate limiter race condition with Redis atomic ops
✓ Session tokens encrypted with AES-256 (key from env var)
✓ Added session rotation on privilege escalation (Gemini)

### Performance (Gemini)
✓ Async bcrypt hashing (40% latency reduction)
✓ Non-blocking login flow

### Testing (Codex)
✓ 95% test coverage on new code
✓ Integration tests for rate limiter
✓ Benchmarks for async hashing

### Observability (Gemini)
✓ Prometheus metrics for failed logins
✓ Structured logging for suspicious patterns

## Migration Plan
1. Deploy in staging with feature flag
2. Monitor bcrypt performance metrics
3. Gradual rollout to 10% → 50% → 100% production traffic
4. Add 2FA support in follow-up PR

All tests passing. Ready for deployment.
```

### Result

**File**: `output/<session>/debate-synthesis.json`

```json
{
  "pattern": "sequential",
  "rounds": 4,
  "summary": "Authentication module hardened with bcrypt, race condition fixed, async performance improvements, and comprehensive testing",
  "result": "All tests passing. Ready for deployment.",
  "tasks_completed": [
    "Claude: Identified security issues, implemented bcrypt and encryption",
    "Codex: Fixed race condition, added tests, hardcoded key fix",
    "Gemini: Added async performance improvements, session rotation, metrics",
    "Vibe: Synthesized all improvements into deployment-ready solution"
  ],
  "confidence": 0.95,
  "warnings": ["Monitor bcrypt work factor in production", "2FA support deferred to follow-up"]
}
```

---

## Example 2: Algorithm Optimization (Round-Robin Pattern)

### User Command

```bash
./council.sh "Optimize this bubble sort implementation for large datasets" \
  --debate \
  --pattern round_robin \
  --rounds 3
```

### Execution Flow

#### Round 0: Parallel Proposals

**All agents receive**:
```
You are participating in a debate. Your role is to propose a solution to the following task.

**Task**: Optimize this bubble sort implementation for large datasets

Provide your best solution.
```

**Responses** (simultaneous):

- **Claude**: "Replace with quicksort (O(n log n) average case)"
- **Codex**: "Use timsort (Python's default, optimal for partially sorted data)"
- **Gemini**: "Parallelize with divide-and-conquer merge sort"
- **Vibe**: "Keep bubble sort but add early termination flag"

#### Round 1: Peer Review

Each agent reviews the **next** agent's work:

- **Claude reviews Codex**: "Timsort is excellent for real-world data. Consider hybrid approach with insertion sort for small partitions."
- **Codex reviews Gemini**: "Parallelization adds overhead. Only beneficial for datasets > 100k elements."
- **Gemini reviews Vibe**: "Early termination helps but doesn't change O(n²) worst case. Not sufficient for large datasets."
- **Vibe reviews Claude**: "Quicksort worst-case is O(n²) on already-sorted data. Needs randomized pivot selection."

#### Round 2: Refinement

Agents update proposals based on feedback:

- **Claude**: "Quicksort with randomized pivot + insertion sort for partitions < 10 elements"
- **Codex**: "Timsort with tuned merge parameters for large datasets"
- **Gemini**: "Parallel merge sort, but only for datasets > 100k (Codex's feedback)"
- **Vibe**: "Hybrid: timsort for < 100k, parallel merge sort for >= 100k"

#### Convergence Check

```typescript
const round1Hash = hashAllWork([claude1, codex1, gemini1, vibe1]);
const round2Hash = hashAllWork([claude2, codex2, gemini2, vibe2]);

if (round1Hash === round2Hash) {
  console.log('Converged at round 2');
  break; // Skip round 3
}
```

**Result**: Not converged (proposals evolved), continue to synthesis.

#### Final Synthesis (Claude)

**Prompt**:
```
Synthesize the best elements from each contribution into a final answer.

## Claude
Quicksort with randomized pivot + insertion sort for partitions < 10 elements

## Codex
Timsort with tuned merge parameters for large datasets

## Gemini
Parallel merge sort, but only for datasets > 100k

## Vibe
Hybrid: timsort for < 100k, parallel merge sort for >= 100k
```

**Claude's synthesis**:
```python
def optimized_sort(arr):
    """
    Hybrid sorting algorithm optimized for real-world datasets.

    Strategy:
    - Small arrays (< 10): Insertion sort (low overhead)
    - Medium arrays (10-100k): Timsort (excellent for partially sorted)
    - Large arrays (>= 100k): Parallel merge sort (leverage multicore)
    """
    n = len(arr)

    if n < 10:
        return insertion_sort(arr)  # Vibe's insight
    elif n < 100_000:
        return timsort(arr)  # Codex's choice
    else:
        return parallel_merge_sort(arr)  # Gemini's optimization

# Benchmarks (1M elements):
# Bubble sort: 120s
# This implementation: 0.8s (150x faster)
```

### Result

**Consensus**: Hybrid approach combining all agents' insights
**Performance**: 150x improvement over original bubble sort

---

## Example 3: Architecture Decision (Tournament Pattern)

### User Command

```bash
./council.sh "Should we use PostgreSQL or MongoDB for our user analytics platform?" \
  --debate \
  --pattern tournament
```

### Execution Flow

#### Round 1: Bracket Matches

**Match 1: Claude (PostgreSQL) vs Codex (MongoDB)**

- **Claude**: "PostgreSQL provides ACID guarantees critical for billing analytics, strong JOIN support, mature ecosystem"
- **Codex**: "MongoDB offers flexible schema for rapidly evolving analytics dimensions, horizontal scaling built-in"

**Judge (Claude Opus via LLM)**:
```
Claude presents stronger argument. Billing analytics requires transactional consistency.
MongoDB's flexibility is valuable but not critical for structured analytics data.

Winner: Claude (PostgreSQL)
```

**Match 2: Gemini (PostgreSQL) vs Vibe (MongoDB)**

- **Gemini**: "PostgreSQL has native time-series support via TimescaleDB, perfect for analytics"
- **Vibe**: "MongoDB aggregation pipeline is more intuitive for complex analytics queries"

**Judge**:
```
Gemini's TimescaleDB argument is compelling for time-series analytics.

Winner: Gemini (PostgreSQL)
```

#### Round 2: Final Match

**Claude vs Gemini (both advocating PostgreSQL)**

- **Claude**: "Focus on ACID and JOINs"
- **Gemini**: "Focus on time-series optimization"

**Judge**:
```
Both arguments support PostgreSQL. Gemini's TimescaleDB recommendation
provides a more specific, actionable solution for the analytics use case.

Winner: Gemini
```

### Result

```json
{
  "pattern": "tournament",
  "decision": "PostgreSQL with TimescaleDB extension",
  "reasoning": "Time-series optimization critical for user analytics, ACID guarantees important for billing, mature ecosystem",
  "tournament_bracket": {
    "round_1": [
      {"match": "Claude vs Codex", "winner": "Claude"},
      {"match": "Gemini vs Vibe", "winner": "Gemini"}
    ],
    "round_2": [
      {"match": "Claude vs Gemini", "winner": "Gemini"}
    ]
  },
  "final_recommendation": {
    "database": "PostgreSQL 15",
    "extension": "TimescaleDB",
    "deployment": "Managed service (AWS RDS or Timescale Cloud)",
    "migration_path": "Start with single instance, partition by time, add read replicas as needed"
  }
}
```

---

## Message Flow Example (NDJSON Protocol)

### Real message sequence for "Write a factorial function" debate

```ndjson
// 1. Server sends initial prompt to all agents
{"type":"user","session_id":"session-abc123","parent_tool_use_id":null,"message":{"role":"user","content":"Write a factorial function"}}

// 2. Claude responds (streaming)
{"type":"stream_event","session_id":"session-abc123","uuid":"req-1","delta":{"type":"text","text":"I'll"}}
{"type":"stream_event","session_id":"session-abc123","uuid":"req-1","delta":{"type":"text","text":" write"}}
{"type":"stream_event","session_id":"session-abc123","uuid":"req-1","delta":{"type":"text","text":" a"}}
// ... more tokens ...

// 3. Claude encounters Write tool
{"type":"control_request","subtype":"can_use_tool","session_id":"session-abc123","uuid":"req-2","tool":"Write","input":{"file_path":"factorial.py","content":"def factorial(n):\n    return 1 if n <= 1 else n * factorial(n-1)"}}

// 4. Server approves
{"type":"control_response","subtype":"can_use_tool","session_id":"session-abc123","uuid":"req-2","behavior":"allow","updatedInput":null,"updatedPermissions":null}

// 5. Claude completes
{"type":"assistant","session_id":"session-abc123","uuid":"req-1","message":{"role":"assistant","content":[{"type":"text","text":"I've written a recursive factorial function in factorial.py"}]},"usage":{"input_tokens":45,"output_tokens":123}}

// 6. Server sends review prompt to Codex
{"type":"user","session_id":"session-abc123","parent_tool_use_id":null,"message":{"role":"user","content":"Review Claude's factorial implementation and suggest improvements:\n\ndef factorial(n):\n    return 1 if n <= 1 else n * factorial(n-1)"}}

// 7. Codex responds with improvements...
```

---

## Performance Metrics

### Sequential Pattern (4 rounds)

- **Total time**: 45 seconds
- **Per-round latency**: ~10 seconds average
- **Messages exchanged**: 78 (4 prompts + 74 streaming/tool events)
- **Token usage**: 2,340 input + 1,890 output
- **Cost**: ~$0.08 (Claude Sonnet 3.5)

### Round-Robin Pattern (3 rounds, 4 agents)

- **Total time**: 35 seconds
- **Parallel execution**: Yes (Round 0, Final synthesis)
- **Messages exchanged**: 312
- **Token usage**: 8,120 input + 4,560 output
- **Cost**: ~$0.28

### Tournament Pattern (3 matches)

- **Total time**: 28 seconds
- **Matches**: 3 (2 in round 1 parallel, 1 in round 2)
- **Judge invocations**: 3 (Claude Opus for decision)
- **Token usage**: 4,890 input + 2,340 output
- **Cost**: ~$0.42 (includes Opus judge calls)

---

## Integration Examples

### From council.sh

```bash
# Add to council.sh after argument parsing

if [ "$DEBATE_MODE" = true ]; then
    # Start SDK server
    bun run "${SCRIPT_DIR}/sdk-server/index.ts" &
    SDK_SERVER_PID=$!
    sleep 2  # Wait for server to be ready

    # Create session via HTTP API
    SESSION_JSON=$(curl -s -X POST http://localhost:8000/sessions \
        -H "Content-Type: application/json" \
        -d "{\"prompt\":\"$PROMPT\",\"agents\":[$ACTIVE_AGENTS]}")

    SESSION_ID=$(echo "$SESSION_JSON" | jq -r '.sessionId')

    # Launch agents with WebSocket URLs
    for agent in $ACTIVE_AGENTS; do
        port="${AGENT_PORTS[$agent]}"
        sdk_launch_agent "$agent" "$SESSION_ID" "$port" &
    done

    # Start debate via HTTP API
    curl -X POST "http://localhost:8000/sessions/${SESSION_ID}/debate" \
        -H "Content-Type: application/json" \
        -d "{\"pattern\":\"$COLLAB_PATTERN\",\"maxRounds\":$COLLAB_ROUNDS}"

    # Wait for completion (poll status endpoint)
    while true; do
        status=$(curl -s "http://localhost:8000/sessions/${SESSION_ID}" | jq -r '.status')
        if [ "$status" = "completed" ]; then
            break
        fi
        sleep 2
    done

    # Cleanup
    kill $SDK_SERVER_PID

    exit 0
fi
```

### From TUI (Go Bubbletea)

```go
// Connect to WebSocket for real-time streaming
func (m model) subscribeToSession(sessionID string) tea.Cmd {
    return func() tea.Msg {
        ws, _, err := websocket.DefaultDialer.Dial(
            fmt.Sprintf("ws://localhost:8765/events?session=%s", sessionID),
            nil,
        )
        if err != nil {
            return errMsg{err}
        }

        for {
            var event CouncilEvent
            err := ws.ReadJSON(&event)
            if err != nil {
                return errMsg{err}
            }

            switch event.Type {
            case "stream:token":
                // Update agent panel with new token
                return streamTokenMsg{
                    agent: event.Data.Agent,
                    text:  event.Data.Delta.Text,
                }

            case "agent:completed":
                // Mark agent as done, show final message
                return agentCompletedMsg{
                    agent: event.Data.Agent,
                    usage: event.Data.Usage,
                }
            }
        }
    }
}
```

---

## Expected Output Structure

### debate-synthesis.json

```json
{
  "version": "1.0",
  "session_id": "session-abc123",
  "pattern": "round_robin",
  "rounds_completed": 3,
  "converged": false,
  "summary": "Hybrid sorting algorithm combining timsort, parallel merge sort, and insertion sort",
  "result": "150x performance improvement over original implementation",

  "timeline": [
    {
      "round": 0,
      "type": "parallel_proposal",
      "agents": ["claude", "codex", "gemini", "vibe"],
      "outputs": {
        "claude": "Quicksort with randomized pivot",
        "codex": "Timsort for partially sorted data",
        "gemini": "Parallel merge sort for large datasets",
        "vibe": "Bubble sort with early termination"
      }
    },
    {
      "round": 1,
      "type": "peer_review",
      "assignments": {
        "claude": "reviews codex",
        "codex": "reviews gemini",
        "gemini": "reviews vibe",
        "vibe": "reviews claude"
      }
    },
    {
      "round": 2,
      "type": "refinement",
      "convergence_hash": "a3f5e8d2",
      "previous_hash": "b9c1f4a7"
    },
    {
      "round": 3,
      "type": "final_synthesis",
      "synthesizer": "claude",
      "result": "Hybrid algorithm with 3-way branching"
    }
  ],

  "metrics": {
    "total_time_seconds": 35,
    "total_tokens": {
      "input": 8120,
      "output": 4560
    },
    "cost_usd": 0.28,
    "messages_exchanged": 312
  },

  "confidence": 0.92,

  "warnings": [
    "Parallel merge sort overhead significant for datasets < 100k",
    "Timsort merge parameters may need tuning for specific data patterns"
  ]
}
```

---

## Troubleshooting Common Scenarios

### Scenario 1: Agent Disagrees with Review

**Round 1**: Claude proposes solution A
**Round 2**: Codex reviews, suggests solution B (complete rewrite)

**Resolution**: Codex's counter-proposal becomes the new candidate. Subsequent rounds review solution B, not A.

### Scenario 2: Convergence Detection False Positive

Two agents produce identical text but with different formatting (whitespace).

**Solution**: Normalize before hashing:

```typescript
private hashAllWork(work: string[]): string {
  const normalized = work.map(w =>
    w.trim().replace(/\s+/g, ' ')
  );
  return Bun.hash(normalized.join('::')).toString();
}
```

### Scenario 3: Agent Times Out Mid-Debate

**Round 1**: Claude completes
**Round 2**: Codex times out (5 min limit)

**Resolution**: Mark Codex's task as failed, continue with remaining agents. Final synthesis notes: "Warning: Codex did not complete review in round 2."

---

## Next: Production Deployment

See `WEBSOCKET_SDK_DESIGN.md` Section 10 for deployment guides (Docker, systemd, Nginx).

**Quick deploy**:

```bash
# Build
docker build -t council-sdk -f sdk-server/Dockerfile .

# Run
docker run -d \
  -p 8000:8000 \
  -p 8765-8768:8765-8768 \
  -e JWT_SECRET=$(openssl rand -hex 32) \
  --name council-sdk \
  council-sdk

# Verify
curl http://localhost:8000/health
```

---

**Status**: Complete working examples provided
**Use these**: As templates for implementing debate orchestration
