# Lore

Lore is a local, cross-session knowledge manager for durable operational
know-how shared by humans and agents.

Complicated workflows are often expensive to understand and easy to forget.
Repository conventions, infrastructure procedures, documentation routes,
failure patterns, and environment-specific constraints should not need to be
reconstructed from scratch in every agent session. Lore retains that knowledge
in a curated, searchable form.

## What Lore makes possible

Lore helps humans and agents:

- recover applicable know-how before repeating a costly investigation;
- preserve an explicit request to remember something useful;
- capture reusable knowledge autonomously when workspace policy allows it;
- distinguish repository, system, host, technology, and other contexts;
- retain labels, references, and provenance without storing conversations;
- revise, supersede, archive, or invalidate knowledge as conditions change;
- review older entries and suggest pruning without deleting them silently.

## Capture knowledge

Knowledge can enter Lore through two complementary paths.

An agent may curate a reusable result after discovering something costly or
non-obvious when the applicable `AGENTS.md` policy authorizes autonomous
capture. A human may also explicitly ask an agent to preserve a fact,
procedure, warning, or other useful result.

Capture retains the knowledge itself rather than the conversation that
produced it. Entries can include a concise title, curated content, applicable
contexts, labels, references, provenance, and verification information.

## Retrieve applicable knowledge

Humans can query Lore directly. Agents can consult it before reconstructing a
non-trivial workflow or making assumptions about a repository, system, host,
or environment.

Retrieval combines text search with structured context and optional labels.
Labels improve filtering but do not replace full-text or contextual retrieval,
and an entry remains valid without them.

## Review and prune

Operational knowledge changes over time. Lore uses complete append-only
revisions so the effective entry can evolve without erasing its history.

Review can identify entries that may be stale, duplicated, contradicted,
overly specific, or no longer useful. Agents can recommend retaining,
revising, merging, archiving, invalidating, or deleting an entry. Physical
deletion remains an explicit administrative action.

## Agent integration

Lore integrates through repository and global agent policies. A policy can
require retrieval before repeated inference, authorize bounded autonomous
capture, and define when human approval is required.

Global integration remains inactive until Lore's knowledge contracts and
commands are implemented and verified. Workspace policy must not refer to
commands the installed application cannot execute.

## Boundaries

Lore is not an activity journal, transcript store, monitoring system, or
measure of productivity. It does not automatically preserve prompts,
conversations, hidden reasoning, terminal history, diffs, file contents,
credentials, tokens, or personal data.

Autonomous capture is selective rather than exhaustive. Retrieval frequency
alone does not determine value: rarely needed knowledge can still be expensive
to reconstruct. Review and pruning recommendations do not authorize silent
destruction.

## Development status

Lore is in early development. Its intended shape is a local Go CLI and
per-user service backed by bounded, append-only JSONL storage. The knowledge
schema, command surface, retrieval behavior, and pruning workflow are being
defined before global agent activation.

The current implementation is a technical foundation under conversion. Its
existing command and data contracts are not yet the public Lore interface.
