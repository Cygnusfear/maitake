---
name: mai-research
description: Use when conducting research, oracle investigations, or delphi consultations. Save findings as mai notes with proper tags and structure.
---

# Mai Research — Oracle and Delphi Investigations

## Single oracle investigation

```bash
# Coordinator creates the research ticket
mai ticket "Investigate token refresh approaches" -p 2 -l research \
  -d "Research mutex vs single-flight vs channel-based refresh.
Compare: performance, error handling, complexity."
# → res-1234

# Delegate to oracle
teams delegate [{
  "text": "Execute res-1234. Use mai-agent skill. Research and post findings.",
  "assignee": "oracle",
  "template": "oracle"
}]
```

### Oracle workflow

```bash
mai show res-1234                  # read the assignment
mai search "token refresh"         # find prior research and decisions on this topic
# ... investigate code, run experiments, read docs ...

mai add-note res-1234 "## Findings

### Mutex
- Simple, well-understood
- Serializes concurrent callers (N callers = N sequential refreshes)
- Error handling is per-caller

### Single-flight
- Coalesces N callers into 1 refresh
- Returns same result/error to all waiters
- Problem: transient errors propagate to all callers

### Channel-based
- Most flexible
- Most complex
- Overkill for this use case

## Recommendation
Use mutex. Single-flight's error propagation is dangerous for auth tokens."

mai close res-1234 -m "Research complete. Recommendation: mutex."
```

## Delphi consultation (parallel oracles)

```bash
# Coordinator creates the delphi epic
mai ticket "Delphi: auth refresh strategy" -p 1 -l research,delphi \
  -d "3 independent oracles investigate the same question."
# → del-5678

# Create sub-tickets for each oracle
mai ticket "Oracle 1: auth refresh" -l research,oracle
# → orc-1111
mai ticket "Oracle 2: auth refresh" -l research,oracle
# → orc-2222
mai ticket "Oracle 3: auth refresh" -l research,oracle
# → orc-3333

mai dep del-5678 orc-1111
mai dep del-5678 orc-2222
mai dep del-5678 orc-3333

# Delegate all three in parallel
teams delegate [
  {"text": "Execute orc-1111. Independent research on auth refresh strategy.", "assignee": "oracle-1", "template": "oracle"},
  {"text": "Execute orc-2222. Independent research on auth refresh strategy.", "assignee": "oracle-2", "template": "oracle"},
  {"text": "Execute orc-3333. Independent research on auth refresh strategy.", "assignee": "oracle-3", "template": "oracle"}
]
```

### After oracles complete

```bash
mai dep tree del-5678
# del-5678 [open] Delphi: auth refresh strategy
# ├── orc-1111 [closed] Oracle 1: auth refresh
# ├── orc-2222 [closed] Oracle 2: auth refresh
# └── orc-3333 [closed] Oracle 3: auth refresh

# Read each oracle's findings
mai show orc-1111
mai show orc-2222
mai show orc-3333

# Synthesize
mai add-note del-5678 "## Synthesis

All 3 oracles agree: mutex is the right approach.
- Oracle 1 flagged single-flight error propagation (confirmed by 2 and 3)
- Oracle 2 found a performance edge case with >100 concurrent callers (not our scale)
- Oracle 3 found existing mutex pattern in src/cache.ts we can reuse

Decision: use mutex, reuse the cache.ts pattern."

mai close del-5678 -m "Delphi complete. Consensus: mutex."
```

## Rules

1. **One ticket per oracle.** Each oracle works independently.
2. **Oracle tickets are children of the delphi epic.** Use `mai dep`.
3. **Findings go in `mai add-note`.** Not in external files.
4. **Synthesis goes on the parent epic.** Read all oracles, synthesize, close.
5. **Close oracle tickets when done.** Don't leave them open.
