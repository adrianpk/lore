# Lore Knowledge Model Delivery Plan

Status: approved

Delivery set: `lore-knowledge-model`

Slice strategy: layered

Reason: the copied activity domain crosses the model, append-only journal,
service, CLI, verification, and documentation boundaries. Introducing and
validating the Lore model before switching persistence and commands keeps every
slice buildable and makes deletion of the old domain explicit rather than
mixing two incompatible schemas in one oversized review.

Spec: `ops/default/spec/00002-lore-knowledge-model/artifact.md`

Tracker: `ops/default/tracker/00004-lore-knowledge-model-delivery/artifact.md`

Base branch: `dev`

Active slice: Slice 1

Execution gate: fulfilled by the user's explicit authorization on 2026-09-10

## Objective

Replace the inherited activity-journal implementation with the approved Lore
knowledge model, durable revision-aware storage, explicit capture and
retrieval commands, and opt-in review and purge workflows without activating
global agent integration.

## Planning Decisions

- `internal/lore` remains the owning package; the delivery changes its domain
  rather than introducing a second application package.
- The existing activity model remains operational only during the first two
  layered slices so each branch stays green. Slice 3 removes it when the Lore
  CLI becomes complete enough to replace it.
- Slice 1 establishes pure model contracts without persistence or command
  behavior. Slice 2 establishes append-only storage and revision resolution.
- Slice 3 performs the user-visible cutover to capture, lifecycle, show, and
  search commands and removes activity reporting and migration behavior.
- Slice 4 adds explicit review and confirmed purge, then aligns all durable
  documentation and acceptance coverage.
- Global `AGENTS.md` integration remains outside this delivery set until the
  completed CLI has been verified in real workflows and separately activated.
- Data migration from an activity journal is outside scope because Lore starts
  with a new knowledge store and no compatible semantic mapping exists.

## Slice Map

| Slice | Short Name | Branch | PR Title | Report | Prerequisites |
| --- | --- | --- | --- | --- | --- |
| Slice 1 | entry model foundation | `feat/slice-1-entry-model-foundation` | `feat(slice-1): establish Lore entry model` | `ops/default/report/slices/lore-knowledge-model/slice-1-entry-model-foundation.md` | Approved spec, plan, and tracker committed on `dev` |
| Slice 2 | revision-aware journal | `feat/slice-2-revision-aware-journal` | `feat(slice-2): persist revision-aware Lore entries` | `ops/default/report/slices/lore-knowledge-model/slice-2-revision-aware-journal.md` | Slice 1 merged into `dev` |
| Slice 3 | knowledge workflows | `feat/slice-3-knowledge-workflows` | `feat(slice-3): deliver Lore knowledge workflows` | `ops/default/report/slices/lore-knowledge-model/slice-3-knowledge-workflows.md` | Slice 2 merged into `dev` |
| Slice 4 | review and pruning | `feat/slice-4-review-pruning` | `feat(slice-4): deliver Lore review and pruning` | `ops/default/report/slices/lore-knowledge-model/slice-4-review-pruning.md` | Slice 3 merged into `dev` |

## Slice 1: Entry Model Foundation

### Delivered Increment

The Lore package has a canonical, validated knowledge-entry model matching the
approved specification. It can normalize and validate entries, contexts,
labels, references, provenance, lifecycle states, and complete revisions in
memory while leaving the current CLI and journal behavior unchanged.

### Tasks

| Task | Work | Expected Commit |
| --- | --- | --- |
| T1.1 | Add entry, kind, status, context, reference, provenance, and revision value contracts with stable JSON field names. | `feat(model): define Lore entries` |
| T1.2 | Add canonicalization and bounded validation for entry content, structured metadata, verification times, sensitive reference forms, and revision invariants. | `feat(model): validate Lore entries` |
| T1.3 | Add focused table-driven tests for valid canonical forms, every rejected boundary, and complete revision construction; align the package model reference. | `test(model): verify Lore entry contracts` |

### Expected Files

- `internal/lore/entry.go`
- `internal/lore/entry_validation.go`
- focused `internal/lore/*entry*_test.go`
- `internal/lore/README.md`
- `ops/default/spec/00002-lore-knowledge-model/files/model.md`
- Slice 1 tracker and report artifacts

### Validation

- `make check`
- `make test`
- `make docs-check`
- `make verify`
- `go test ./internal/lore -run 'TestEntry|TestContext|TestReference|TestRevision'`
- `git diff --check`

### Done When

- Every approved enum and JSON field has one canonical Go representation.
- Valid entries normalize deterministically and invalid entries return
  actionable validation errors.
- Context, label, reference, provenance, time, content, and revision bounds are
  covered by focused tests.
- Existing commands and tests remain green without persisting Lore entries yet.

## Slice 2: Revision-Aware Journal

### Delivered Increment

Lore entries append durably to monthly `entries/YYYY/MM.jsonl` files, recover
interrupted writes, and resolve complete revision chains into deterministic
effective and historical views without exposing a new CLI yet.

### Tasks

| Task | Work | Expected Commit |
| --- | --- | --- |
| T2.1 | Add the knowledge journal layout, canonical JSONL append path, bounded reads, pending-write staging, and interrupted-write recovery. | `feat(store): persist Lore entries` |
| T2.2 | Add indexed entry lookup, linear revision validation, effective-view resolution, historical chains, and corruption diagnostics. | `feat(store): resolve Lore entry revisions` |
| T2.3 | Cover restart, missing final newline, pending recovery, month boundaries, forks, cycles, missing predecessors, and kind changes. | `test(store): verify Lore journal recovery` |

### Validation

- `make check`
- `make test`
- `make docs-check`
- `make verify`
- `go test ./internal/lore -run 'TestEntryJournal|TestEntryHistory|TestEntryRecovery'`
- `git diff --check`

## Slice 3: Knowledge Workflows

### Delivered Increment

Humans and agents can explicitly record, revise, archive, invalidate, restore,
show, and search Lore knowledge through stable human and JSON interfaces. The
activity-journal domain, reporting, correction, and migration surface is
removed in the same cutover.

### Tasks

| Task | Work | Expected Commit |
| --- | --- | --- |
| T3.1 | Add record parsing, JSON input, body-file support, inferred project context, canonical rendering, and service orchestration. | `feat(cli): record Lore knowledge` |
| T3.2 | Add revise, archive, invalidate, and restore commands that append complete successors with explicit reasons. | `feat(cli): manage Lore entry lifecycle` |
| T3.3 | Add show, history, and bounded search with text, kind, status, context, label, time, order, and limit behavior. | `feat(cli): retrieve Lore knowledge` |
| T3.4 | Remove the inherited activity, worklog, report, correction, and migration contracts and their obsolete tests and assets. | `refactor(core): remove activity journal model` |

### Validation

- `make check`
- `make test`
- `make docs-check`
- `make verify`
- `go test ./internal/lore -run 'TestRecordEntry|TestReviseEntry|TestSearchEntries|TestShowEntry'`
- `git diff --check`

## Slice 4: Review And Pruning

### Delivered Increment

Explicit review returns bounded objective candidate sets without mutation;
physical purge requires matching dry-run evidence and preserves store
integrity. The user guide, how-to guides, reference, explanation, service
assets, and acceptance tests describe Lore rather than an activity journal.

### Tasks

| Task | Work | Expected Commit |
| --- | --- | --- |
| T4.1 | Add explicit review filters and duplicate-title candidate grouping without semantic or lifecycle mutation. | `feat(review): select pruning candidates` |
| T4.2 | Add chain-scoped purge dry runs, store-bound confirmation tokens, atomic rewrite, and post-purge verification. | `feat(store): purge confirmed entry chains` |
| T4.3 | Rewrite durable documentation and service-facing examples for human and agent Lore workflows while keeping global activation deferred. | `docs(lore): document knowledge workflows` |
| T4.4 | Add end-to-end capture, revision, retrieval, review, purge, restart, and system-service acceptance coverage. | `test(acceptance): verify Lore knowledge workflows` |

### Validation

- `make check`
- `make test`
- `make docs-check`
- `make verify`
- `go test ./internal/lore -run 'TestReview|TestPurge|TestKnowledgeWorkflow'`
- `git diff --check`

## Cross-Slice Invariants

- Every slice starts from the latest merged `dev` and leaves all repository
  gates green.
- New persisted Lore data follows the approved entry model exactly.
- Stored entries are append-only except through confirmed physical purge.
- Revisions remain complete, linear, acyclic, and kind-stable.
- Search and review never mutate state.
- No command stores credentials, raw prompts, transcripts, or hidden reasoning.
- No slice activates global agent integration.
- The inherited activity domain is transitional and is fully removed in
  Slice 3 rather than retained as a second ledger.

## Delivery Rules

- The approved specification, this plan, and the tracker are committed and
  pushed on `dev` before Slice 1 branches.
- Execute slices in order from the latest merged `dev`.
- Use the exact branch, PR title, task IDs, and report path in this plan.
- Use one Conventional Commit per tracker task when practical.
- Every slice PR targets `dev`; no slice merges directly to `main`.
- Update the tracker and slice report before opening each PR.
- Wait for maintainer-controlled merge before starting the next slice.

## Final Completion

The delivery set is complete when all four slice PRs are merged into `dev`,
their reports are delivered, inherited activity behavior is absent, all
knowledge workflows and recovery paths pass, global activation remains
deferred, and a cycle report exists at:

```text
ops/default/report/cycles/lore-knowledge-model.md
```
