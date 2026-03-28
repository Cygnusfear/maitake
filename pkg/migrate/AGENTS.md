# pkg/migrate

Migrates tickets from tk's `.tickets/` shadow branch (YAML frontmatter + markdown) to maitake's git notes substrate (JSON event streams).

## Boundaries

- Reads `.tickets/*.md` files, writes to notes engine
- Preserves IDs, timestamps, all metadata
- Splits `## Notes` sections into separate comment notes
- Maps deps/links/parent to edges
- Supports dry-run mode
