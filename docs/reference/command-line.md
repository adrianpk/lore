# Command-line Reference

Lore uses a local per-user service for every command except `serve`. Commands
accept repeated `--context`, `--label`, `--reference`, `--kind`, `--status`, or
`--id` options where shown.

## `lore serve`

```sh
lore serve
```

Load the canonical entry store and serve requests through the configured Unix
socket. The supplied systemd socket starts this command on demand.

## `lore record`

```sh
lore record <procedure|fact|warning|reference> \
  --title <title> (--body <markdown> | --body-file <path>) [options]
```

Create one active entry. Options are:

| Option | Meaning |
| --- | --- |
| `--verified-at <time>` | RFC3339 UTC time when the content was last verified. |
| `--context <value>` | `type:name` or `type:name@repository`; repeat up to 16 times. |
| `--label <name>` | Canonical topical label; repeat up to 8 times. |
| `--reference <value>` | `type:value`; repeat up to 32 times. |
| `--source <type>` | `human`, `agent`, or `integration`. |
| `--harness <name>` | Agent harness name; requires agent provenance. |
| `--harness-version <version>` | Agent harness version. |
| `--model <identifier>` | Model identifier. |
| `--json-input` | Read entry fields from standard input as JSON. |
| `--json` | Write the mutation result as JSON. |

Context types are `project`, `repository`, `system`, `host`, `technology`, and
`client`. Reference types are `url`, `file`, `repository`, and `commit`. When a
project context is omitted, the CLI attempts to infer it from the current Git
worktree.

## `lore search`

```sh
lore search [options]
```

Return bounded effective entries. The default is the newest 20 active entries;
the limit range is 1 through 200.

| Option | Meaning |
| --- | --- |
| `--text <text>` | Case-insensitive substring match over title and body. |
| `--kind <kind>` | Match one of the four entry kinds. |
| `--status <status>` | Match `active`, `archived`, or `invalidated`. |
| `--context <value>` | Require one exact canonical context. |
| `--label <name>` | Match any supplied label. |
| `--recorded-from <time>` | Inclusive RFC3339 UTC lower bound. |
| `--recorded-to <time>` | Exclusive RFC3339 UTC upper bound. |
| `--order <order>` | `ascending` or `descending`. |
| `--limit <count>` | Return 1 through 200 entries. |
| `--json` | Write complete entries and truncation state as JSON. |

Different filter classes combine with AND. Repeated kinds and statuses match
any supplied value; repeated contexts are all required.

## `lore show`

```sh
lore show --id <entry-id> [--history] [--json]
```

Resolve any chain member to its current effective entry. `--history` returns
the complete chain in oldest-to-newest order.

## `lore revise`

```sh
lore revise --target <entry-id> --reason <reason> [replacement options]
```

Append a successor to the current effective entry. Replacement options are
`--title`, `--body`, `--body-file`, `--verified-at`, repeated `--context`,
`--label`, and `--reference`, plus `--clear-verified-at`, `--clear-contexts`,
`--clear-labels`, and `--clear-references`. `--json-input` reads the revision
request from standard input and `--json` selects JSON output. Unspecified fields
are inherited; kind and provenance remain unchanged.

## Lifecycle commands

```sh
lore archive --target <entry-id> --reason <reason> [--json]
lore invalidate --target <entry-id> --reason <reason> [--json]
lore restore --target <entry-id> --reason <reason> [--json]
```

Each command appends a complete successor with status `archived`,
`invalidated`, or `active`. The target must resolve to the current effective
entry.

## `lore review`

```sh
lore review <selector> [options]
```

Return read-only pruning candidates. At least one objective selector is
required. The default limit is 20 and the range is 1 through 200.

| Option | Meaning |
| --- | --- |
| `--recorded-before <time>` | Entries recorded before an RFC3339 UTC time. |
| `--verified-before <time>` | Entries with an older verification time. |
| `--status <status>` | Entries in any supplied lifecycle state. |
| `--context <value>` | Entries containing every supplied context. |
| `--duplicate-titles` | Entries sharing a lowercase, whitespace-folded title. |
| `--limit <count>` | Maximum candidate count. |
| `--json` | Include candidates, reasons, groups, and truncation state as JSON. |

Every supplied selector class must match.

## `lore purge`

```sh
lore purge (--dry-run | --confirm <token>) <selector> [--json]
```

Physically remove complete revision chains. Select with repeated `--id`,
`--recorded-before`, repeated `--status`, or `--all`. Non-`--all` selectors form
a union. `--all` is exclusive. Exactly one of `--dry-run` and `--confirm` is
required.

A dry run returns exact chains, counts, and a confirmation token bound to the
selector and current stored bytes. Confirmation must repeat the selectors and
is rejected after any store change.

## `lore verify`

```sh
lore verify [--scheduled] [--json]
```

Report stored revision and effective-entry counts after service startup has
validated and indexed the store. `--scheduled` identifies timer-triggered
verification.
