# Command Line Reference

Lore provides `serve`, `migrate`, `record`, `search`, `show`, `report`,
`correct`, and `verify`. Client commands require the local service socket;
`migrate` is offline and does not use it.

## `lore serve`

```text
lore serve
```

Run the per-user service. It adopts the supplied systemd socket when socket
activation is present. Otherwise, it creates the Unix socket directly.

## `lore migrate`

```text
lore migrate --source <legacy-journal-root> --destination <canonical-journal-root> --manifest <path>
```

Transform one frozen legacy events directory into a new canonical events
directory. The command does not contact or use the service.

| Option | Meaning |
| --- | --- |
| `--source <path>` | Existing legacy events root containing only `YYYY/MM.jsonl` segments. |
| `--destination <path>` | New canonical events root to publish. The path must not exist. |
| `--manifest <path>` | New deterministic cutover manifest. The path must not exist. |

The three paths are resolved to absolute clean paths. They must be distinct;
source and destination must not contain each other, and the manifest must be
outside both. Destination and manifest parent directories must already exist.

Migration strictly decodes every legacy milestone and correction, validates
the complete source history, writes to a private sibling staging directory,
verifies the canonical result, and rechecks that the source did not change.
Only then does it publish the destination and manifest. Publication never
overwrites an existing target. A failed manifest publication removes the newly
published destination.

The manifest records deterministic source and destination journal SHA-256
digests, per-segment paths, line counts and SHA-256 digests, ordered event IDs,
effective-event count, and ordered correction chains. It contains no generation
time or machine-specific source and destination paths. Repeating migration from
the same frozen source produces byte-identical canonical segments and manifest.

## `lore record`

```text
lore record commit [common flags] [summary]
lore record pull-request --forge <url> --repository <owner/name> --number <n> --action <action> [common flags] <summary>
lore record worklog --duration <minutes> [common flags] <summary>
lore record note [common flags] <summary>
```

`record` requires an explicit activity type. Flags precede the positional
summary.

### Common record options

| Option | Meaning |
| --- | --- |
| `--at <time>` | Set occurrence time from RFC3339 input. A commit derives this time and rejects the option. |
| `--scope-type <type>` | Set `repository`, `project`, `system`, `client`, or `other` scope. |
| `--scope-name <name>` | Set the scope display name. |
| `--scope-identity <identity>` | Set the stable scope identity. |
| `--reference-url <url>` | Add a URL reference. Repeatable. |
| `--reference-commit <scope>@<object-id>` | Add a Git commit reference. Repeatable. |
| `--reference-pull-request <url>` | Add a pull-request reference. Repeatable. |
| `--source <source>` | Set `human`, `agent`, or `integration` provenance. |
| `--harness <name>` | Set the agent harness. Requires agent provenance. |
| `--harness-version <version>` | Set the harness version. Requires `--harness`. |
| `--model <id>` | Set the execution model. Requires `--harness`. |

Scope options are all-or-none. Commit and pull-request events derive their
repository scope and reject a custom scope. When `--at` is absent for a
non-Git event, occurrence time equals the service recording time.

### Type-specific behavior

- `commit` reads the current worktree `HEAD`, commit subject and timestamps,
  and a credential-free repository identity. Its summary defaults to the
  commit subject. Repository identity plus object ID is idempotent.
- `pull-request` requires `--forge`, `--repository`, `--number`, and an
  `--action` of `opened`, `reviewed`, `merged`, or `closed`. `--url` adds the
  optional pull-request URL. Forge, repository, number, and action are
  idempotent. Lore does not contact the forge.
- `worklog` requires `--duration` from 1 through 1,440 minutes.
- `note` has no type-specific properties.

Separately submitted worklogs and notes remain separate events even when their
content matches.

## `lore search`

```text
lore search [flags]
```

| Option | Meaning |
| --- | --- |
| `--query <text>` | Match normalized text across summaries, scopes, typed display fields, references, correction reasons, and provenance identifiers. |
| `--type <list>` | Match comma-separated `commit`, `pull_request`, `worklog`, or `note` types. |
| `--scope <value>` | Match an exact scope name or identity without case sensitivity. |
| `--commit <prefix>` | Match a lowercase hexadecimal full or abbreviated commit object ID. |
| `--pull-request <value>` | Match normalized pull-request identity text. |
| `--occurred-from <time>` | Include events occurring at or after an RFC3339 time. |
| `--occurred-to <time>` | Exclude events occurring at or after an RFC3339 time. |
| `--recorded-from <time>` | Include events recorded at or after an RFC3339 time. |
| `--recorded-to <time>` | Exclude events recorded at or after an RFC3339 time. |
| `--duration-min <minutes>` | Match worklogs with at least this explicit duration. |
| `--duration-max <minutes>` | Match worklogs with at most this explicit duration. |
| `--source <source>` | Match `human`, `agent`, or `integration` provenance. |
| `--harness <name>` | Match a normalized harness name. |
| `--model <id>` | Match a normalized model identifier. |
| `--limit <count>` | Return 1 through 1,000 events. The default is 20. |
| `--order <value>` | Use `ascending` or `descending`. The default is `descending`. |
| `--json` | Write the structured compact result. |

Ranges are inclusive at `from` and exclusive at `to`. Search returns only the
effective event in each correction chain. Its projection contains event ID,
type, occurrence and recording times, summary, optional scope name, optional
worklog duration, and optional type-specific identifier. It excludes complete
properties, references, provenance, and correction data.

Results may be truncated by the count limit or the 8 MiB response bound. JSON
clients must check `returned_count` and `truncated`.

## `lore show`

```text
lore show --id <event-id> [--raw] [--json]
```

Return one complete canonical event. By default, an ID anywhere in a
correction chain resolves to the current effective event. `--raw` returns the
exact stored event with that ID. `--json` exposes all canonical fields.

## `lore report`

```text
lore report --from <date> --to <date> [--timezone <name>] [--type <types>] [--limit <count>] [--json]
```

| Option | Meaning |
| --- | --- |
| `--from <date>` | Inclusive occurrence date in `YYYY-MM-DD` form. |
| `--to <date>` | Exclusive occurrence date in `YYYY-MM-DD` form. |
| `--timezone <name>` | IANA timezone name. The process local timezone is the default. |
| `--type <list>` | Match comma-separated activity types. |
| `--limit <count>` | Return 1 through 1,000 events. The default is 1,000. |
| `--json` | Write the structured grouped result. |

Reports group compact effective events by local occurrence date and scope.
Unscoped events appear under `Unscoped`. Every day, scope, and report has an
event count and explicit worklog-minute total. No other event type contributes
duration.

JSON clients must check `returned_count` and `truncated`.

## `lore correct`

```text
lore correct --target <event-id> --reason <reason> [--at <time>] [--summary <summary>] [--duration <minutes>] [--json]
```

Append a complete type-preserving replacement for one current effective event.

| Option | Meaning |
| --- | --- |
| `--target <event-id>` | Select the current effective event by ULID. |
| `--reason <reason>` | Explain the correction in at most 500 Unicode code points. |
| `--at <time>` | Replace occurrence time from RFC3339 input. |
| `--summary <summary>` | Replace the summary. Omit it to retain the current value. |
| `--duration <minutes>` | Replace explicit duration on a worklog. |
| `--json` | Write the complete appended replacement. |

The target cannot be missing or stale. An omitted replacement field is copied
from the target. An empty summary is invalid because every event requires a
summary. Duration is valid only for a worklog and must remain between 1 and
1,440 minutes. The original event remains unchanged.

## `lore verify`

```text
lore verify [--full | --scheduled] [--json]
```

| Option | Meaning |
| --- | --- |
| `--full` | Verify every canonical monthly segment. |
| `--scheduled` | Verify the active segment daily and the complete journal on the first scheduled run of each local month. |
| `--json` | Write the structured result. |

Without a scope option, Lore verifies the active monthly segment. `--full` and
`--scheduled` are mutually exclusive. A corruption diagnostic identifies the
segment, one-based line, zero-based byte offset, and reason.
