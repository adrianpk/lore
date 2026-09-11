# Slice 3: Knowledge Workflows

Status: ready for merge

Delivery set: `lore-knowledge-model`

Plan: `ops/default/plan/00003-lore-knowledge-model-delivery/artifact.md`

Tracker: `ops/default/tracker/00004-lore-knowledge-model-delivery/artifact.md`

Branch: `feat/slice-3-knowledge-workflows`

PR: `#3`

## Purpose

Expose Lore capture, revision, lifecycle, and retrieval workflows while
removing the inherited activity journal in one user-visible cutover.

## Delivered Behavior

- `lore record <kind>` records active procedure, fact, warning, or reference
  entries with a required title and exactly one inline or file-backed Markdown
  body.
- Repeatable context, label, and reference flags preserve structured metadata;
  JSON input supports agent capture without multiline shell quoting.
- Recording in a Git worktree silently infers a canonical project context and
  includes a credential-free origin identity when available.
- `lore revise` inherits omitted fields and replaces explicitly supplied
  complete values. Verification and provenance can be explicitly cleared.
- `lore archive`, `lore invalidate`, and `lore restore` append complete
  status revisions with required reasons and reject stale targets.
- `lore show` returns the effective revision or complete history.
  `lore search` applies bounded text, kind, status, context, label, time,
  order, and limit filters to effective entries.
- Search defaults to active entries, newest-first order, and a limit of 20.
  Repeated filters use OR semantics except contexts, which use AND semantics.
- The local service remains the sole writer for capture and lifecycle
  mutations.
- Activity, worklog, report, correction, schedule, and migration models,
  commands, indexes, tests, and obsolete assets are absent.

## Explicit Boundary

This slice does not add review candidate selection or physical purge. It does
not rewrite the public documentation set or activate global agent integration;
those remain assigned to Slice 4.

## Contracts Added Or Changed

- `RecordEntryParams`, `ReviseEntryParams`, and `LifecycleEntryParams`
  separate caller input from Lore-assigned identity and recording time.
- `EntryWorkflows` appends root entries and complete successor revisions
  through `EntryJournal`.
- `SearchEntriesParams` and `ShowEntryParams` provide deterministic
  effective and historical retrieval.
- `Service` dispatches record, revise, archive, invalidate, restore, show,
  search, and verify operations over the local Unix socket.
- `Config` owns only the XDG knowledge store, configuration, runtime, and
  socket paths.

## Files Of Interest

- `internal/lore/entry_cli.go`
- `internal/lore/entry_workflows.go`
- `internal/lore/entry_lifecycle.go`
- `internal/lore/entry_retrieval.go`
- `internal/lore/entry_retrieval_cli.go`
- `internal/lore/project_context.go`
- `internal/lore/service.go`
- `internal/lore/app.go`

## Commits

- `0b21c90` — `feat(cli): deliver Lore knowledge workflows`

The command cutover and inherited-domain removal share one commit because
neither side is independently buildable without exposing incompatible product
contracts.

## Validation

- `make check` (pass)
- `make test` (pass)
- `make docs-check` (pass)
- `make verify` (pass; lint reports `0 issues`)
- `go test ./internal/lore -run 'TestRecordEntry|TestReviseEntry|TestSearchEntries|TestShowEntry'` (pass)
- `go test -race ./internal/lore -run 'TestRecordEntry|TestReviseEntry|TestSearchEntries|TestShowEntry'` (pass)
- Isolated service validation recorded, revised, archived, showed, searched,
  and verified a three-revision chain.
- `git diff --check` (pass)

The Make targets ran with `GOTOOLCHAIN=go1.26.3`, matching `go.mod` and the
toolchain used to build the installed linter.

## Risks And Follow-Ups

- Public tutorials, how-to guides, reference pages, and explanations remain
  the explicit Slice 4 documentation task.
- Review selection and confirmed chain purge remain unavailable until Slice 4.
- Global agent integration remains inactive.
