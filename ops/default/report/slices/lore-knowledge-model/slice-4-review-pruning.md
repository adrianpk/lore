# Slice 4: Review And Pruning

Status: ready for merge

Delivery set: `lore-knowledge-model`

Plan: `ops/default/plan/00003-lore-knowledge-model-delivery/artifact.md`

Tracker: `ops/default/tracker/00004-lore-knowledge-model-delivery/artifact.md`

Branch: `feat/slice-4-review-pruning`

PR: `#4`

## Purpose

Complete the initial Lore knowledge model with objective review, confirmed
whole-chain purge, durable product documentation, and end-to-end acceptance
coverage while leaving global agent integration inactive.

## Delivered Behavior

- `lore review` selects bounded effective entries using recording time,
  verification time, status, context, and duplicate canonical title filters.
- Review filters combine across supplied classes, explain every selection, and
  never mutate entry or lifecycle state.
- `lore purge --dry-run` returns exact sorted chains, revision counts, and a
  confirmation token bound to the canonical selector and stored bytes.
- Confirmed purge rejects stale or mismatched evidence, rewrites retained
  revisions through synchronized sibling directories, and removes complete
  chains without dangling successors.
- Startup recovers recognized interrupted directory swaps and rejects
  ambiguous purge state.
- The user guide, how-to guides, reference, explanation, README, changelog, and
  package description now document Lore knowledge workflows rather than the
  inherited baseline.
- Workspace agent guidance starts with explicit capture, allows useful
  read-only retrieval, keeps labels sparse, and leaves autonomous capture and
  global activation as separate future policy decisions.
- Unix-service acceptance tests cover all four entry kinds, capture, revision,
  lifecycle, retrieval, review, purge, restart, and supplied systemd assets.

## Explicit Boundary

This slice does not install or modify global `AGENTS.md`, activate the Lore
service on the development host, migrate unrelated data, or merge its own pull
request. A cycle report remains a post-merge delivery-set closure artifact.

## Contracts Added Or Changed

- `ReviewParams` and `ReviewResult` define objective, bounded, read-only
  pruning candidate selection and duplicate-title groups.
- `PurgeParams` and `PurgeResult` separate exact dry-run evidence from a
  destructive confirmation request.
- Purge selectors use union semantics except for exclusive `all`; review
  selectors use intersection semantics across supplied classes.
- A purge ID resolves any chain member to its root and always selects the full
  linear revision chain.
- The current store digest and canonical selection form a deterministic SHA-256
  confirmation token.

## Files Of Interest

- `internal/lore/review.go`
- `internal/lore/purge.go`
- `internal/lore/entry_purge_store.go`
- `internal/lore/acceptance_test.go`
- `docs/tutorials/user-guide/`
- `docs/how-to/`
- `docs/reference/`
- `docs/explanation/`

## Commits

- `e7bd347` — `feat(review): select pruning candidates`
- `2c138bc` — `feat(store): purge confirmed entry chains`
- `2dd32ac` — `docs(lore): document knowledge workflows`
- `1cab38a` — `test(acceptance): verify Lore knowledge workflows`

## Validation

- `make check` (pass)
- `make test` (pass)
- `make docs-check` (pass)
- `make verify` (pass; lint reports `0 issues`)
- `go test ./internal/lore -run 'TestReview|TestPurge|TestKnowledgeWorkflow'` (pass)
- `go test -race ./internal/lore -run 'TestReview|TestPurge|TestKnowledgeWorkflow'` (pass)
- All local Markdown links resolve to existing files.
- No inherited activity command, worklog, timesheet, report, correction, or
  migration terminology remains in the product documentation.
- `git diff --check` (pass)

The Make targets ran with `GOTOOLCHAIN=go1.26.3`, matching `go.mod` and the
toolchain used to build the installed linter.

## Risks And Follow-Ups

- Global agent integration remains inactive until separately authorized after
  real workflow evaluation.
- Physical purge is intentionally irreversible after matching confirmation;
  archive and invalidate remain the preferred reversible choices.
- Delivery-set closure and the canonical cycle report remain pending until the
  maintainer merges Slice 4 into `dev`.
