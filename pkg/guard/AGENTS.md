# pkg/guard

Thin hook runner for write-time protection.

## Purpose

- Run local maitake hooks from `.maitake/hooks/`
- Feed note content on stdin
- Pass through hook-specific environment variables
- Reject writes when a blocking hook exits non-zero

## Boundaries

- NO git logic
- NO note parsing or ticket semantics
- NO policy logic beyond shipping the default pre-write hook content
- Missing hooks are allowed
- Non-executable hooks are skipped with a warning

## Testing

- Every exported function has a test
- Use temp directories and real hook scripts
- Keep the package thin and stdlib-only
