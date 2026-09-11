# Revise and Verify Knowledge

Append a successor when an entry is incomplete or outdated. The prior revision
remains immutable and the successor becomes effective.

## Find and inspect the entry

```sh
lore search --text "preview service" --context project:lore
lore show --id <entry-id> --json
```

## Append a revision

Supply the target, a reason, and only the replacement fields that changed:

```sh
lore revise \
  --target <entry-id> \
  --reason "The service now uses socket activation" \
  --body-file ./revised-procedure.md \
  --verified-at 2026-09-11T10:00:00Z
```

Use `--clear-verified-at`, `--clear-contexts`, `--clear-labels`, or
`--clear-references` to remove an optional value explicitly. An entry kind
cannot change within a revision chain.

## Inspect history

```sh
lore show --id <entry-id> --history --json
```

Any chain member resolves to the effective entry unless `--history` is used.

## Verify the store

```sh
lore verify --json
```

Service startup validates and indexes the canonical segments. This request
reports the indexed revision and effective-entry counts. The installed timer
invokes `lore verify --scheduled` daily.
