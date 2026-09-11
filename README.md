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

The initial capture policy is explicit: a human asks an agent to preserve a
fact, procedure, warning, or reference. The agent curates the reusable result
and records it without preserving the surrounding conversation. Autonomous
capture can be authorized later by workspace policy, but it is not the
recommended starting mode.

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

Lore integrates through repository and global agent policies. A workspace can
allow retrieval when stored know-how may reduce repeated inference while
keeping capture limited to explicit user requests. Autonomous capture remains
a separate, opt-in policy decision.

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

Lore implements its initial local Go CLI and per-user service. It supports
typed capture, contextual search, complete revision chains, lifecycle changes,
objective review, confirmed whole-chain purge, and storage verification over
bounded monthly JSONL segments.

Repository documentation describes optional workspace integration, but no
global agent policy is installed or activated by this repository.
