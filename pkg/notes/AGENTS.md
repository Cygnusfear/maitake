# pkg/notes

Notes substrate for maitake.

## Purpose

- Parse and serialize the maitake note format
- Generate human-readable note IDs
- Define the core note, state, query, and engine types
- Later: create, append, fold, index, branch scope, and query notes on top of `pkg/git` and `pkg/guard`

## Boundaries

- Knows about note headers, edges, kinds, folding inputs, slots, and branch scope
- Does NOT know about ticket-specific or review-specific rules beyond generic note kinds
- Uses `pkg/git` for git object and notes-ref types
- Must not shell out to git directly

## Rules

- Keep files small; split before 500 lines
- Every exported function needs a test
- Parse errors must be strict and descriptive
- Serializer output must round-trip through the parser
