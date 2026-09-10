# Lore Knowledge Model

Status: approved

Source quiz: `ops/default/quiz/00001-lore-knowledge-model/artifact.md`

Companion model: `ops/default/spec/00002-lore-knowledge-model/files/model.md`

## Purpose

Define the first implementable Lore contract for preserving and retrieving
curated operational know-how across human and agent sessions.

## Scope

Lore stores durable knowledge entries, complete append-only revisions, and
their structured applicability. It provides explicit capture, contextual
retrieval, lifecycle changes, review support, and explicit physical purge.

This specification does not activate Lore in global agent instructions, infer
knowledge from transcripts, synchronize data between machines, encrypt stored
entries, or provide a hosted service.

## Actors

- A human records, retrieves, reviews, revises, archives, invalidates, or
  explicitly purges knowledge.
- An agent performs the same operations when the active agent policy grants
  that authority.
- The Lore CLI validates requests and renders stable machine-readable output.
- The local store preserves canonical entries and recovers interrupted writes.

## Knowledge Entry

Every stored entry contains:

- a ULID `id`;
- one `kind`: `procedure`, `fact`, `warning`, or `reference`;
- one `status`: `active`, `archived`, or `invalidated`;
- a required short `title`;
- one required Markdown `body`;
- `recorded_at` in UTC;
- optional `verified_at` in UTC after a concrete check;
- zero or more structured `contexts`;
- zero or more sparse topic `labels`;
- zero or more supporting `references`;
- optional capture `provenance`;
- optional `supersedes` and `revision_reason` for a complete revision.

Kinds share this common shape. A procedure normally explains repeatable steps,
a fact records a durable truth, a warning describes a reusable hazard, and a
reference explains what a resource is and when it should be consulted. These
meanings guide capture and review; they do not introduce kind-specific storage
schemas.

## Bounds And Normalization

- `title` is trimmed, non-empty, single-line UTF-8 with at most 200 Unicode
  code points.
- `body` is trimmed, non-empty UTF-8 Markdown with at most 32 KiB.
- An entry has at most 16 contexts, 8 labels, and 32 references.
- A context `name`, label, reference `label`, and provenance identifier has at
  most 64 Unicode code points unless the companion model states a narrower
  canonical grammar.
- Duplicate contexts, labels, and references are rejected after
  canonicalization rather than silently stored twice.
- Credentials, access tokens, private keys, raw prompts, conversation
  transcripts, and hidden reasoning are invalid entry content.

Canonical names and labels use lowercase ASCII kebab form. Normalization trims
space, lowercases ASCII letters, replaces each run outside `a-z`, `0-9` with
one hyphen, and removes leading or trailing hyphens. An empty result is invalid.
Project and repository contexts default to the normalized project-root
directory name when a caller can determine it reliably.

## Contexts

Each context has a required `type` and canonical `name`. Supported initial
types are `project`, `repository`, `system`, `host`, `technology`, and
`client`.

A context may also carry a full `repository` identity such as
`forge.example/owner/name`. This value disambiguates matching short names but
does not replace the required canonical name. A context is uniquely identified
within an entry by `(type, name, repository)`.

Context filters use exact canonical matching. Multiple context filters are
combined with AND semantics; an entry must contain every requested context.

## Labels And References

Labels are optional topic markers. They use the same canonical grammar as
context names and must not encode project, repository, host, client, or other
structured applicability already represented by contexts.

A supporting reference contains:

- a required `type`: `url`, `file`, `repository`, or `commit`;
- a required trimmed `value` of at most 2,048 bytes;
- an optional short `label`.

Lore preserves the value but does not fetch or validate the referenced
resource during capture. URL user information and token-bearing values are
rejected.

## Verification And Provenance

`recorded_at` is assigned by Lore when it accepts an entry. A supplied
`verified_at` must not be later than `recorded_at`. Later verification is
recorded as a new revision.

Provenance identifies the capture origin as `human`, `agent`, or
`integration`. It may include short optional `harness`, `harness_version`, and
`model` strings. Provenance is descriptive evidence, not an authorization or
confidence score.

## Revision And Lifecycle

Lore never mutates a stored entry in place. Revising, archiving, invalidating,
or restoring an entry appends a complete new entry whose `supersedes` points
to the current effective revision and whose non-empty `revision_reason`
explains the change.

Revision chains must be linear and acyclic:

- the superseded entry must exist;
- it must be the current effective entry in its chain;
- no entry may supersede itself;
- the entry kind remains stable across a chain;
- a superseded revision cannot receive a second successor.

Any lifecycle status may transition to any other status through an explicit
revision. Normal retrieval returns only the effective revision of each chain
and defaults to `active` entries. Historical retrieval returns all revisions
without changing them.

## Persistence

The local store follows XDG conventions and writes canonical JSON Lines under:

```text
${XDG_DATA_HOME:-$HOME/.local/share}/lore/entries/YYYY/MM.jsonl
```

Each line is one complete entry. Monthly placement derives from
`recorded_at`. Writes use the repository's existing pending-write and recovery
discipline: validate and durably stage before append, tolerate a missing final
newline, repair only one valid interrupted pending write, and reject ambiguous
or corrupt recovery state. Stored lines remain bounded by the existing journal
line limit.

## Command Contract

The initial CLI surface is:

- `lore record <kind>` creates an active root entry. It requires `--title` and
  exactly one of `--body` or `--body-file`; repeatable `--context`, `--label`,
  and `--reference` flags add metadata.
- `lore revise --target <id> --reason <text>` appends a complete successor.
  Omitted content and metadata fields inherit from the target; explicitly
  supplied fields replace the corresponding complete value.
- `lore archive`, `lore invalidate`, and `lore restore` append lifecycle
  revisions and require `--target` plus `--reason`.
- `lore show --id <id>` returns the effective revision by default. `--history`
  returns its complete oldest-to-newest chain.
- `lore search` combines optional text, kind, status, context, label, and time
  filters. It searches effective active entries by default.
- `lore review` returns bounded candidates selected by explicit objective
  filters for human or agent evaluation; it never changes stored state.
- `lore purge` physically removes selected chains only through the explicit
  two-step contract defined below.

Repeatable contexts use `type:name` or `type:name@repository`. References use
`type:value` with an optional separate label in structured JSON input. All
commands support JSON output. Capture commands accept JSON input so agents do
not need fragile shell quoting for multiline bodies.

Text search is case-insensitive substring matching over title and body.
Filters combine with AND semantics; repeated values for one filter use OR
semantics except repeated contexts, which retain the all-context rule. Default
ordering is newest `recorded_at` first with `id` as a deterministic tie-breaker.
The default limit is 20 and the maximum is 200.

## Review And Purge

Review is explicitly invoked. Lore supplies objective candidate sets such as
entries recorded or verified before a timestamp, lifecycle states, duplicate
canonical titles, or requested contexts. A human or agent interprets value and
may recommend retaining, revising, merging, archiving, invalidating, or
deleting entries. Lore does not claim semantic quality from age alone.

Purge operates on complete revision chains so it cannot leave dangling
`supersedes` references. A caller first runs `lore purge --dry-run` with an
explicit selector: repeated `--id`, `--recorded-before`, a lifecycle status, or
`--all`. Lore returns the exact chain IDs, count, and a confirmation token
derived from that selection. The destructive invocation must repeat the same
selector and provide `--confirm <token>`. A changed store or selection
invalidates the token. No implicit, scheduled, or autonomous purge exists.

## Agent Capture Policy

Global activation is deferred. When a future agent policy explicitly enables
Lore, the agent applies a lightweight capture-value test after substantive
turns. It records knowledge silently only when the result is reusable beyond
the current step, expensive to rediscover, sufficiently grounded, and safe to
retain. Most turns produce no entry. An explicit user capture request always
causes the agent to curate the requested knowledge rather than save a
transcript.

## Invariants

- The append-only store is the source of truth until an explicit purge.
- Effective revision selection is deterministic and never hides a fork or
  corrupt chain.
- Capture never requires interactive metadata questions when context can be
  inferred reliably.
- Search and review are read-only.
- Archive and invalidation remain recoverable through revisions.
- Physical deletion requires explicit selection, dry-run evidence, and a
  matching confirmation token.
- No command stores secrets or agent reasoning as knowledge.
- Global agent integration remains inactive until the CLI and persistence
  contracts are implemented and verified.

## Acceptance Criteria

- All four entry kinds round-trip through canonical JSON with validation of
  required fields and bounds.
- Context normalization, labels, references, verification time, provenance,
  and duplicate detection have focused tests.
- Complete revisions preserve history and deterministically expose one
  effective entry without permitting forks or cycles.
- Entries survive process restart and interrupted-write recovery in the
  monthly JSONL store.
- Explicit capture, lifecycle, show, search, and historical retrieval work in
  both human-readable and JSON modes.
- Review produces bounded candidates without mutation.
- Purge cannot delete data without a matching dry-run confirmation token and
  leaves no broken revision chain.
- Repository quality gates and focused acceptance tests pass.
- Documentation explains how humans and agents integrate Lore while keeping
  global activation explicitly deferred.
