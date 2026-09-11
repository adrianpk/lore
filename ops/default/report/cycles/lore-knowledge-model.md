# Lore Knowledge Model Delivery

Status: completed

Delivery set: `lore-knowledge-model`

Completed: 2026-09-11

Spec: `ops/default/spec/00002-lore-knowledge-model/artifact.md`

Plan: `ops/default/plan/00003-lore-knowledge-model-delivery/artifact.md`

Tracker: `ops/default/tracker/00004-lore-knowledge-model-delivery/artifact.md`

## Outcome

Lore is a functional local knowledge manager for reusable operational know-how.
Humans and agents can record curated entries, retrieve applicable knowledge,
preserve complete revision history, change lifecycle state, review objective
pruning candidates, and physically remove confirmed complete chains through a
single per-user service.

The inherited activity domain is absent. Lore starts with its own knowledge
store and does not attempt a semantic migration from unrelated data.

## Delivered Slices

| Slice | Outcome | PR | Integrated Commit |
| --- | --- | --- | --- |
| Slice 1 | Canonical typed entry model and bounded validation | `#1` | `4d22d87` |
| Slice 2 | Durable monthly JSONL storage and linear revision resolution | `#2` | `bd0f2ce` |
| Slice 3 | Capture, lifecycle, show, search, and inherited-domain removal | `#3` | `9a24e75` |
| Slice 4 | Objective review, confirmed purge, product documentation, and acceptance coverage | `#4` | `6e9888b` |

## Final Product Contract

- Entries are `procedure`, `fact`, `warning`, or `reference` records with a
  required title and Markdown body.
- Structured project, repository, system, host, technology, and client
  contexts define applicability; sparse labels provide optional topical hints.
- Search is bounded, deterministic, revision-aware, and active-only by default.
- Revision, archive, invalidate, and restore append complete immutable
  successors.
- Review is objective, bounded, and read-only.
- Purge selects complete chains, requires exact dry-run evidence, and rejects a
  token after any selector or store change.
- Canonical data stays local in protected XDG paths and the per-user service is
  the sole writer.
- Public documentation follows a tutorial, how-to, reference, and explanation
  structure with an explicit-first agent capture policy.

## Validation

The integrated `dev` branch passed:

- `make check`
- `make test`
- `make docs-check`
- `make verify`
- `go test ./internal/lore -run 'TestReview|TestPurge|TestKnowledgeWorkflow'`
- `git diff --check`

`make verify` reported zero lint issues. Acceptance coverage exercises all four
entry kinds, local Unix transport, revision lifecycle, retrieval, review,
confirmed purge, process restart, and supplied systemd assets.

## Deferred Boundary

Global agent integration remains inactive. The repository documents an
explicit-first workspace policy, but installing or activating a global Lore
rule requires separate authorization after real workflow evaluation.
