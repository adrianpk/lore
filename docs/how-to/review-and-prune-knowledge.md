# Review and Prune Knowledge

Use review for read-only candidate selection. Prefer archive or invalidate when
history remains useful; reserve purge for knowledge that should be physically
removed.

## Select objective candidates

At least one selector is required:

```sh
lore review \
  --recorded-before 2026-01-01T00:00:00Z \
  --context project:lore \
  --duplicate-titles \
  --json
```

Supplied selectors combine with AND semantics. Repeated statuses match any
listed status, while repeated contexts are all required. `--verified-before`
matches only entries with an older verification timestamp.

## Retain history without normal retrieval

```sh
lore archive --target <entry-id> --reason "Retained for historical context"
```

Normal search returns active entries by default. Invalidated and archived
entries remain available through explicit status filters and history.

## Preview a purge

Purge selectors combine as a union and always select whole revision chains:

```sh
lore purge --status invalidated --recorded-before 2025-01-01T00:00:00Z --dry-run --json
```

Inspect `chains`, `chain_count`, and `revision_count` before continuing.

## Confirm the exact purge

Repeat the selectors and provide the returned token:

```sh
lore purge --status invalidated --recorded-before 2025-01-01T00:00:00Z --confirm <token> --json
```

Any intervening store change invalidates the token. `--all` cannot be combined
with another selector and still requires dry-run confirmation.
