# Local Knowledge Model

Lore keeps operational know-how on the user's machine because the content may
span private repositories, infrastructure, hosts, and clients. A per-user
service serializes mutations while a Unix socket gives humans and agents the
same bounded interface.

Each stored line is one complete knowledge revision. Complete revisions make
recovery and inspection simple: an entry never depends on an in-place patch or
hidden database state. Monthly UTC segments bound file size, while an in-memory
index resolves the effective end of every revision chain.

The four kinds describe why knowledge exists. A `procedure` explains repeatable
steps, a `fact` states a durable operational truth, a `warning` preserves a
hazard, and a `reference` says what resource matters and when to consult it.
Contexts describe applicability; sparse labels add topical retrieval hints.

Revision and lifecycle operations preserve history. Archive removes an entry
from normal retrieval, invalidate marks it incorrect, and restore makes a new
active revision. Physical purge is deliberately separate: it requires a dry
run and a matching store-bound token, then removes complete chains so no
dangling revision remains.

Lore stores curated knowledge, not the evidence source itself. References can
route an agent to documentation, a file, repository, or commit, but the caller
remains responsible for checking whether that source is current.
