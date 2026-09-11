# Slice 2: Revision-Aware Journal

Status: delivered

Delivery set: `lore-knowledge-model`

Plan: `ops/default/plan/00003-lore-knowledge-model-delivery/artifact.md`

Tracker: `ops/default/tracker/00004-lore-knowledge-model-delivery/artifact.md`

Branch: `feat/slice-2-revision-aware-journal`

PR: `#2`

## Purpose

Persist canonical Lore entries durably and resolve complete append-only
revision chains before exposing knowledge commands.

## Delivered Behavior

- Lore resolves its new knowledge data root through `XDG_DATA_HOME`, falling
  back to `$HOME/.local/share/lore`, without changing the inherited activity
  paths.
- Canonical entries append to `entries/YYYY/MM.jsonl` with private directory
  and file permissions, bounded reads, strict JSON fields, canonical encoding,
  and deterministic monthly placement.
- One entry-named pending operation stages every append. Startup and pre-append
  recovery complete a missing or exact partial append, clear a completed
  append, and preserve mismatched, malformed, ambiguous, or unexpected state.
- A complete final JSON object without an LF remains readable; the next append
  restores JSONL framing with one separator.
- Raw lookup, terminal effective revisions, oldest-to-newest history, and
  effective-entry enumeration rebuild from the append-only store after
  restart.
- Persistence rejects duplicate IDs, missing predecessors, stale targets and
  forks, cycles, kind changes, backdated revisions, wrong monthly placement,
  unknown fields, and noncanonical JSON.
- Returned entries are deep copies, preventing callers from mutating the
  in-memory index outside append validation.

## Explicit Boundary

This slice does not expose entry capture or retrieval through the CLI or
service. The inherited activity journal remains operational and separate. The
planned Slice 3 command cutover will assemble this store and remove the
activity domain atomically.

## Contracts Added Or Changed

- `Config` exposes `DataRoot`, `EntriesRoot`, and `EntryPendingRoot` using the
  approved XDG layout.
- `EntryJournal.Load` prepares private storage, recovers one operation, and
  rebuilds all indexes before becoming available.
- `EntryJournal.Append` canonicalizes, validates, stages, publishes, syncs, and
  indexes one complete entry revision.
- `Entry`, `Effective`, `History`, `EffectiveEntries`, and `Count` expose
  read-only copies of disposable index state.
- `ErrEntryJournalNotLoaded`, `ErrEntryJournalCorrupt`,
  `ErrEntryRevisionTargetMissing`, and `ErrEntryRevisionTargetStale` provide
  stable failure classification.

## Files Of Interest

- `internal/lore/config.go`
- `internal/lore/entry_journal.go`
- `internal/lore/entry_index.go`
- `internal/lore/entry_pending.go`
- `internal/lore/entry_journal_test.go`
- `internal/lore/entry_pending_test.go`

## Commits

- `ca2c318` — `feat(store): persist Lore entries`
- `f5b73de` — `feat(store): resolve Lore entry revisions`
- `30b2409` — `test(store): verify Lore journal recovery`
- `8f2353e` — `fix(store): isolate returned Lore entries`
- `0af227e` — `fix(store): satisfy entry loader convention`
- `565e96b` — `fix(test): satisfy pending helper convention`

## Validation

- `make check` (pass)
- `make test` (pass)
- `make docs-check` (pass)
- `make verify` (pass; lint reports `0 issues`)
- `go test ./internal/lore -run 'TestEntryJournal|TestEntryHistory|TestEntryRecovery'` (pass)
- `go test -race ./internal/lore -run 'TestEntryJournal|TestEntryHistory|TestEntryRecovery'` (pass)
- `git diff --check` (pass)

The Make targets ran with `GOTOOLCHAIN=go1.26.3`, matching `go.mod` and the
toolchain used to build the installed linter.

## Risks And Follow-Ups

- The store is intentionally unreachable from the product CLI until Slice 3
  can replace the inherited activity surface without a partial cutover.
- Pending publication serializes one in-process writer. The service remains
  the single intended write authority.
- Semantic duplicate detection and search ranking remain retrieval concerns,
  not persistence invariants.
- Global agent integration remains inactive.
