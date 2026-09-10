# Lore Core

This package owns Lore's knowledge-entry model, validation, repository
identity normalization, monthly journal storage, and local recovery behavior.

`entry.go` defines the canonical entry, context, reference, provenance, and
lifecycle value contracts. `entry_validation.go` owns normalization and local
entry invariants. Persistence and command integration remain separate concerns.

The inherited activity model remains temporarily available while the layered
delivery replaces persistence and the CLI. New product behavior must use the
entry model and must not extend that transitional activity surface.

The root `main` package assembles dependencies. Product logic remains in this
package.
