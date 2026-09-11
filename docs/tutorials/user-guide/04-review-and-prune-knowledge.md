# Review and Prune Knowledge

Review objective candidates, archive knowledge that should leave normal
retrieval, and physically remove a complete chain only after a dry run.

## Review candidates

Find archived entries that also share duplicate canonical titles:

```sh
lore review --status archived --duplicate-titles --json
```

Every supplied review filter must match. The response states why each entry
was selected and groups duplicate titles without changing stored knowledge.

## Prefer lifecycle changes

Archive a current entry when it remains useful as history:

```sh
lore archive --target <entry-id> --reason "No longer part of the normal workflow"
```

Use `invalidate` when the knowledge is incorrect and `restore` when an archived
or invalidated chain should become active again.

## Dry-run a physical purge

Select the complete chain containing one entry:

```sh
lore purge --id <entry-id> --dry-run --json
```

Inspect the exact chain and revision counts, then copy the confirmation token.

## Confirm the same purge

Run the same selector with the token:

```sh
lore purge --id <entry-id> --confirm <confirmation-token> --json
```

Lore rejects the confirmation if the selector or store changed after the dry
run. A confirmed purge removes every revision in each selected chain.
