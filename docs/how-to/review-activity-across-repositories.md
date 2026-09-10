# Review Activity Across Scopes

Use search for discovery, show for one complete event, and report for a bounded
calendar interval.

## Locate prior work

Search summaries, scopes, typed display fields, references, correction reasons,
and provenance identifiers:

```sh
lore search --query "redaction" --limit 20
```

Narrow the result when necessary:

```sh
lore search \
  --scope project:example \
  --type commit,pull_request,worklog \
  --occurred-from 2026-08-01T00:00:00Z \
  --occurred-to 2026-09-01T00:00:00Z \
  --order ascending \
  --json
```

Use `--commit <prefix>` for one commit identity,
`--pull-request <text>` for a pull-request identity, or duration and provenance
filters for worklog and source-specific views.

## Resolve the exact event

Copy the event ID from search output:

```sh
lore show --id <event-id> --json
```

`show` returns the complete effective record, including properties, references,
provenance, and correction information. Add `--raw` when the supplied ID must
return that exact stored chain member.

## Summarize a period

```sh
lore report \
  --from 2026-08-01 \
  --to 2026-08-08 \
  --timezone Europe/Paris \
  --json
```

Reports group effective events by local occurrence date and scope. They show
event counts and sum only explicit worklog minutes. Check the `truncated` field
before treating JSON output as complete.
