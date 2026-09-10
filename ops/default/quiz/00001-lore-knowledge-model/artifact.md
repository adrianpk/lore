# Lore Knowledge Model Quiz

Status: Curated

## Purpose

Establish sufficient product decisions for a first Lore knowledge-model
specification without prematurely fixing the complete CLI or persistence
schema.

## Product Intent

Lore preserves curated operational know-how that humans and agents can reuse
across sessions. It supports explicit capture and selective autonomous capture
without interrupting the active workflow.

## Entry Model

- The initial entry kinds are `procedure`, `fact`, `warning`, and `reference`.
- Every kind uses the same minimal content shape rather than a specialized
  schema.
- Content consists of `title` plus one flexible Markdown body. There is no
  separate description alongside the body.
- A `reference` entry explains what a resource is, when to consult it, and why
  it is authoritative or useful.
- Common references remain metadata that any entry kind can use as evidence or
  a related route.

## Context And Labels

- Applicability is expressed through structured contexts rather than labels.
- An entry may have several contexts.
- Initial context types are `project`, `repository`, `system`, `host`,
  `technology`, and `client`.
- Every context has a required short canonical `name` used for normal
  retrieval.
- A project or repository name defaults to a normalized lowercase form of the
  project-root directory name when that value is available.
- A full repository identity remains optional metadata for disambiguation.
- Labels describe topics; they do not encode project, repository, host, or
  other applicability metadata.
- Labels are optional and should remain sparse.

## Verification And Lifecycle

- Every entry has `recorded_at`.
- `verified_at` is optional and is populated only after a concrete check.
- Lore does not initially model confidence scales or inferred/probable states.
- Effective entry states are `active`, `archived`, and `invalidated`.
- A revision is a complete new entry that identifies the prior effective entry
  through `supersedes`.
- Normal retrieval returns the effective revision while preserving append-only
  history.

## Capture Policy

- After each substantive turn, an agent applies a lightweight internal test for
  reusable knowledge.
- Autonomous capture requires a high threshold: the knowledge must be useful
  beyond the current step, costly to rediscover, sufficiently grounded, and
  safe to retain.
- Most turns produce no capture and require no Lore command.
- Authorized autonomous capture writes directly without asking for per-entry
  approval or interrupting steering.
- A user can always explicitly request capture of a concrete fact or a curated
  result from the preceding interaction.
- Capture stores curated knowledge, not a transcript of the interaction.
- Batch evaluation every fixed number of prompts is not required.

## Review, Pruning, And Purge

- Review and pruning run only when explicitly requested during the initial
  product stage.
- Review may recommend retaining, revising, merging, archiving, invalidating,
  or deleting entries.
- Physical purge is initially available only through explicit invocation.
- No agent may infer authorization for automatic physical deletion.

## Constraints

- Routine capture must not add approval prompts or metadata questions to the
  active workflow.
- Context and label inference should avoid extra user prompts when the agent
  has reliable information.
- Global agent integration remains inactive until the implementation and its
  commands are verified.
- Lore must not preserve prompts, conversations, hidden reasoning, credentials,
  tokens, or incidental sensitive data as knowledge.

## Open Questions

- Exact limits for title, body, labels, contexts, and references.
- Exact context-name normalization and collision handling.
- Exact reference variants and their required fields.
- Exact CLI names and flags.
- Search ranking and the interaction between text, contexts, kinds, and labels.
- Exact purge selectors, date semantics, dry-run behavior, and confirmation
  mechanism.

## Pre-Spec Readiness

The quiz reached sufficient clarity for a draft specification. The remaining
questions can be resolved while defining enforceable contracts and do not
require another broad discovery round.
