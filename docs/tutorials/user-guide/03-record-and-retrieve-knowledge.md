# Record and Retrieve Knowledge

Record one procedure, find it by context, and inspect its complete content.

## Record a procedure

From the relevant project checkout, run:

```sh
lore record procedure \
  --title "Recover the preview service" \
  --body "Inspect the user service logs before restarting it. Restart only after the failure mode is identified." \
  --context project:lore \
  --context system:preview-service \
  --label recovery \
  --source human
```

Lore returns the new entry ID. A project context is inferred from the Git
worktree when none is supplied, but explicit contexts make the example
portable.

## Find the entry

Search by text and required context:

```sh
lore search --text "preview service" --context project:lore
```

Search filters are combined across filter classes. Repeated labels match any
listed label; repeated contexts are all required. Normal search returns active
entries only unless `--status` is supplied.

## Inspect the complete entry

Copy the entry ID from the search output:

```sh
lore show --id <entry-id> --json
```

The result includes the title, Markdown body, contexts, labels, references,
provenance, verification time, and lifecycle state when present.

## Revise the knowledge

Append a complete successor while preserving the earlier revision:

```sh
lore revise \
  --target <entry-id> \
  --reason "Add the bounded restart condition" \
  --body "Inspect the user service logs first. Restart once only after identifying a transient failure."
```

Inspect the chain from either revision ID:

```sh
lore show --id <entry-id> --history --json
```

Continue with [Review and prune knowledge](04-review-and-prune-knowledge.md).
