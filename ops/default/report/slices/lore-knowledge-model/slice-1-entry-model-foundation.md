# Slice 1: Entry Model Foundation

Status: ready for review

Delivery set: `lore-knowledge-model`

Plan: `ops/default/plan/00003-lore-knowledge-model-delivery/artifact.md`

Tracker: `ops/default/tracker/00004-lore-knowledge-model-delivery/artifact.md`

Branch: `feat/slice-1-entry-model-foundation`

PR: pending

## Purpose

Establish the canonical Lore knowledge-entry contracts and local validation
boundary before the revision-aware journal and user-facing command cutover.

## Delivered Behavior

- Lore represents `procedure`, `fact`, `warning`, and `reference` entries with
  one common title and Markdown body shape.
- Entries carry active, archived, or invalidated lifecycle state; structured
  project, repository, system, host, technology, and client contexts; sparse
  labels; supporting references; optional provenance; and complete revision
  metadata.
- Canonicalization produces lowercase kebab context names and labels while
  trimming caller-supplied content and metadata without mutating caller-owned
  slices or objects.
- Validation enforces content, collection, metadata, time, URL, credential,
  provenance, ULID, and local revision bounds with actionable errors.
- Complete revision construction preserves entry kind, requires an explicit
  reason, rejects self-reference and backdating, and leaves chain-wide
  effective-head validation to persistence.

## Explicit Boundary

This slice does not persist Lore entries or expose knowledge commands. The
inherited activity journal and CLI remain unchanged so the repository stays
operational until the planned Slice 3 cutover. New behavior must use the entry
model and cannot extend the transitional activity domain.

## Contracts Added Or Changed

- `Entry`, `EntryID`, `EntryKind`, `EntryStatus`, `Context`, `EntryReference`,
  `Provenance`, and `EntryProjection` define stable JSON field names.
- `NormalizeCanonicalName` implements the approved lowercase ASCII kebab
  normalization contract.
- `Entry.Canonicalize` returns an independently normalized and validated copy.
- `Entry.Validate`, context, reference, provenance, enum, and ID validators own
  local model invariants.
- `NewEntryRevision` constructs a complete successor without performing
  persistence-owned chain lookup.

## Files Of Interest

- `internal/lore/entry.go`
- `internal/lore/entry_validation.go`
- `internal/lore/entry_test.go`
- `internal/lore/README.md`
- `ops/default/spec/00002-lore-knowledge-model/files/model.md`

## Commits

- `8e16b2c` — `feat(model): define Lore entries`
- `e61cd13` — `feat(model): validate Lore entries`
- `4519940` — `fix(model): refine revision validation`
- `0a704af` — `test(model): verify Lore entry contracts`

## Validation

- `make check` (pass)
- `make test` (pass)
- `make docs-check` (pass)
- `make verify` (pass; lint reports `0 issues`)
- `go test ./internal/lore -run 'TestEntry|TestContext|TestReference|TestRevision'` (pass)
- `git diff --check` (pass)

The four Make targets ran with `GOTOOLCHAIN=go1.26.3`, matching `go.mod` and
the toolchain used to build the installed linter.

## Risks And Follow-Ups

- Effective-head ownership, forks, cycles, missing predecessors, and
  persistence recovery remain Slice 2 responsibilities.
- The inherited activity domain remains intentionally present until the
  atomic CLI cutover in Slice 3.
- Global agent integration remains inactive.
