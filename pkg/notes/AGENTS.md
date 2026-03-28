# pkg/notes

The substrate. Creates notes with IDs, appends events and comments, folds current state, queries by kind/status/target/tag. Built on `pkg/git` and `pkg/guard`.

## Boundaries

- Knows about note format, edges, kinds, event folding, slots, branch-scope
- Does NOT know about tickets, reviews, or docs — those are just kinds
- Consumers see the Engine interface
