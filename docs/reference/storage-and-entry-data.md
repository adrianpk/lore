# Storage and Entry Data Reference

Lore stores private per-user knowledge under the XDG data directory and serves
requests through an XDG runtime socket.

## Paths

| Path | Purpose |
| --- | --- |
| `${XDG_DATA_HOME:-$HOME/.local/share}/lore/entries/YYYY/MM.jsonl` | Canonical monthly UTC entry segments. |
| `${XDG_DATA_HOME:-$HOME/.local/share}/lore/pending/` | Recoverable append transactions. |
| `$XDG_RUNTIME_DIR/lore/lore.sock` | Per-user Unix service socket. |
| `$HOME/.config/systemd/user/lore*.{service,socket,timer}` | Optional installed user units. |

Directories and the socket are user-private. Published entry lines are never
edited in place. Indexes are rebuilt in memory from canonical segments.

## Canonical entry

Each JSONL line contains one complete revision:

| Field | Contract |
| --- | --- |
| `id` | Immutable ULID assigned by the service. |
| `kind` | `procedure`, `fact`, `warning`, or `reference`. |
| `status` | `active`, `archived`, or `invalidated`. |
| `title` | Required single-line title, at most 200 Unicode code points. |
| `body` | Required Markdown content, at most 32 KiB. |
| `recorded_at` | Service-assigned UTC acceptance time. |
| `verified_at` | Optional UTC content-verification time. |
| `contexts` | Up to 16 structured applicability contexts. |
| `labels` | Up to 8 canonical topical labels. |
| `references` | Up to 32 supporting resources. |
| `provenance` | Optional human, agent, or integration origin. |
| `supersedes` | Prior effective revision ID. |
| `revision_reason` | Required explanation for a successor revision. |

Unknown fields and invalid combinations are rejected. Context names and labels
are normalized to lowercase kebab form. Duplicate contexts, labels, and
references are rejected.

## Revision chains and lifecycle

A revision is a new complete entry that points to the prior effective entry.
The kind and provenance cannot change within a chain. Search, review, and
normal show operations use only each chain's terminal effective revision;
history returns every revision.

Archive, invalidate, and restore are lifecycle revisions. They retain the
chain. Confirmed purge is the only operation that physically removes entries,
and it removes selected chains in full.

## Durability and recovery

An append is staged in `pending`, synchronized, appended to its monthly
segment, and recovered on restart if interrupted. A purge writes retained
entries to a sibling staging directory before swapping directories. Startup
finishes or rolls back recognized interrupted states and rejects ambiguous
ones.

## Privacy boundary

Entries can contain user-supplied body text, paths, repository identities,
references, and provenance. Keep the data directory private. Do not store
credentials, token-bearing URLs, personal data, transcripts, prompts, hidden
reasoning, terminal history, diffs, or whole source files.

Lore does not contact remote repositories or documentation services. A
reference identifies supporting material but does not copy or verify it.
