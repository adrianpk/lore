# Lore Core

This package owns Lore's knowledge-entry model, validation, repository
identity normalization, monthly entry storage, and local recovery behavior.

`entry.go` defines the canonical entry, context, reference, provenance, and
lifecycle value contracts. `entry_validation.go` owns normalization and local
entry invariants. `entry_journal.go` and `entry_index.go` own monthly JSONL
persistence, raw lookup, effective revisions, and history. `entry_pending.go`
owns recoverable append publication. The entry workflow, service, and CLI
files own explicit capture, complete revisions, lifecycle transitions, and
bounded retrieval.

The root `main` package assembles dependencies. Product logic remains in this
package.
