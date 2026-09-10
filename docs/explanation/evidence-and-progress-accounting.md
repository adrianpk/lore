# Activity Evidence and Progress Accounting

Professional work produces several kinds of evidence. A commit can anchor an
implementation in Git. A pull-request action can identify integration or
review. Configuration, investigation, coordination, and operational work may
produce neither artifact. Lore records each as a peer activity type instead of
using commits as the parent model.

## Two consumers

For an agent, Lore is a retrieval index. A bounded search can locate a relevant
event and its optional references. A commit can route the agent to an immutable
Git object; a pull request or URL can route it to another source system; a
worklog or note can preserve a meaningful fact without inventing an artifact.

For a human, Lore is a local account of recorded activity across scopes.
Reports can review a week, group work by project or system, and total explicit
worklog minutes for timesheet preparation.

## Evidence levels

Lore supports distinct levels of interpretation:

1. Stored facts include event type, occurrence and recording times, summary,
   optional scope, type-specific properties, references, and provenance.
2. Transparent derivations include event counts, occurrence-date groups, scope
   distributions, and the sum of explicit worklog minutes.
3. Contextual inferences require evidence outside Lore. For example, an agent
   can inspect a referenced commit or pull request to explain a delivery.
4. Unavailable facts include unrecorded activity, inferred effort, difficulty,
   causality, productivity, and hidden reasoning.

An event sequence does not prove causal dependence. A worklog duration states
what its source supplied; it is not verified or derived by Lore.

## Capture policy affects interpretation

A selective workspace records a sparse set of important commits. A broad
workspace records most completed intentional commits. User-directed worklogs
and notes cover different facts and can have their own capture policy. Event
counts across unlike policies are not comparable performance measures.

Lore makes missing evidence visible but cannot reconstruct activity that no one
recorded.
