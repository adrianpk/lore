# Record and Retrieve Activity

Record one Git-backed event and one non-Git event, then retrieve both from the
local journal.

## Record a commit

In an example Git worktree, confirm the completed commit:

```sh
git show --no-patch --oneline HEAD
```

Record it explicitly:

```sh
lore record commit "Established the first reusable workspace policy"
```

Lore prints `recorded`, an event ID, the `commit` type, the repository name,
and an abbreviated object ID. Repeating the command for the same repository
and commit prints `already recorded` without appending a duplicate.

## Record non-Git work

Record a two-hour configuration task under a system scope:

```sh
lore record worklog \
  --duration 120 \
  --scope-type system \
  --scope-name "Build pipeline" \
  --scope-identity system:build-pipeline \
  "Configured release-stage credentials"
```

The duration is an explicit fact supplied to Lore. It is not inferred from the
commit or command time.

## Find the events

Search summaries and typed display fields:

```sh
lore search --query "workspace policy"
lore search --type worklog --scope system:build-pipeline
```

Each result includes its event ID. Retrieve one complete effective event:

```sh
lore show --id <event-id> --json
```

The JSON result includes complete type-specific properties, references,
provenance, and correction information when present.

## Review a bounded period

Use local calendar dates to group activity by occurrence date and scope:

```sh
lore report --from 2026-09-01 --to 2026-09-08 --timezone UTC
```

The `from` date is inclusive and the `to` date is exclusive. The report shows
event counts and sums only explicit worklog minutes.

Return to the [user guide](index.md), or use the [how-to
guides](../../how-to/index.md) for recurring tasks.
