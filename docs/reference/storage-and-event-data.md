# Storage and Event Data Reference

Lore stores per-user data locally and uses fixed paths in the current release.
There is no user-editable configuration file.

## Paths

| Path | Purpose |
| --- | --- |
| `$HOME/.lore/events/YYYY/MM.jsonl` | Canonical monthly UTC journal segment. |
| `$HOME/.lore/pending.json` | Recovery record for one interrupted append. |
| `$HOME/.lore/verification.json` | Month of the last successful scheduled full verification. |
| `$XDG_RUNTIME_DIR/lore/lore.sock` | Per-user Unix service socket. |

Lore creates data directories with mode `0700` and owned files with mode
`0600`. The supplied systemd units use `UMask=0077`. The service is the sole
canonical journal writer.

An offline migration writes a new events root and a separately selected
manifest path. Neither is a fixed live path until an operator performs the
approved cutover.

## Canonical event envelope

Each JSONL line contains one complete canonical event. There is no
`schema_version` field.

| Field | Meaning |
| --- | --- |
| `id` | Time-sortable ULID assigned by the service. Its timestamp equals `recorded_at` at millisecond precision. |
| `type` | `commit`, `pull_request`, `worklog`, or `note`. |
| `occurred_at` | UTC time when the activity occurred. |
| `recorded_at` | UTC time when the service accepted the event. |
| `summary` | Required single-line text of at most 500 Unicode code points. |
| `scope` | Optional repository, project, system, client, or other context. |
| `properties` | Strict type-specific properties. |
| `references` | Up to 32 optional related commits, pull requests, or URLs. |
| `provenance` | Optional human, agent, or integration source. |
| `supersedes` | Prior effective event ID for a correction. |
| `correction_reason` | Required reason when `supersedes` is present. |

Optional fields are omitted rather than stored as `null`. Canonical decoding
rejects unknown fields, legacy shapes, and properties owned by another event
type.

## Type-specific properties

- A `commit` requires repository scope plus full lowercase `object_id` and
  `subject`; `authored_at` is optional. Its occurrence time is the Git
  committer time.
- A `pull_request` requires repository scope plus normalized `forge`,
  `repository`, positive `number`, and `action`; `url` is optional.
- A `worklog` requires `duration_minutes` from 1 through 1,440.
- A `note` has no properties object.

Commit repository identity is a normalized credential-free remote or
`git-root:<root-object-id>` when no remote exists. Pull-request scope identity
is `<forge>/<owner>/<repository>`. Absolute checkout paths, branches, and
worktree state are not canonical event data.

## Scope, references, and provenance

A scope contains required `type`, display `name`, and stable `identity` fields.
Worklogs and notes can be unscoped.

A `git_commit` reference contains a scope identity and full object ID. A
`pull_request` or `url` reference contains an absolute HTTP or HTTPS URL.
References do not change the event type or its natural identity.

Provenance contains a `type` of `human`, `agent`, or `integration`. Agent
provenance requires a harness and can include harness version and model. It
describes the semantic source of the record; it is not authorization or
verified identity.

## Corrections and effective events

A correction is a complete event of the same type that points to the current
effective event through `supersedes`. It preserves append-only history.
Default search, report, and show behavior uses the final effective event in a
chain. Raw show and full verification retain access to every stored event.

## Privacy

The journal contains user-supplied summaries, context identities, references,
and optional provenance. Keep it private. Do not commit journal files or attach
them to an issue without review and redaction.

Lore does not automatically collect prompts, conversations, hidden reasoning,
diffs, patches, file contents, environment variables, terminal history,
duration, effort, or productivity. A user or agent can still supply sensitive
text, so capture policies must prohibit secrets and personal data.

## Bounds and rotation

- Journal segments rotate by UTC `recorded_at` month.
- One encoded event line can be at most 8 MiB.
- One local service request can be at most 64 KiB.
- One local service response can be at most 8 MiB.
- Search and report return at most 1,000 effective events per request.
- A summary or correction reason can contain at most 500 Unicode code points.

## Migration manifest

The deterministic `lore/migration-manifest/v1` document has top-level
`source` and `destination` summaries. Each summary contains:

- a journal SHA-256 digest derived from ordered segment metadata;
- segment and event counts;
- effective-event count;
- ordered event IDs;
- ordered correction chains with root, effective, and member IDs;
- ordered relative segment paths with line counts and file SHA-256 digests.

Source and destination digests differ because their encoded event shapes
differ. Counts, IDs, segment placement, append order, and correction-chain
semantics must match.
