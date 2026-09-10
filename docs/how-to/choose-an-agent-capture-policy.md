# Choose an Agent Capture Policy

Define the capture policy in each workspace `AGENTS.md`. Different workspaces
can use different commit granularity. Direct user instructions can separately
authorize non-Git activity records.

## Use a selective commit policy

Use this policy when the journal should contain only important completed Git
outcomes:

```md
After creating a significant commit, immediately run `lore record commit [flags] [summary]` while that commit is still `HEAD`.
Record completed delivery outcomes, reusable decisions, accepted specifications, and non-obvious fixes.
Use the summary when the commit subject does not explain why the outcome matters.
Supply provenance only when its values are known reliably.
Do not record routine, mechanical, generated, formatting-only, punctuation-only, or work-in-progress commits.
Never include credentials, tokens, personal data, or hidden reasoning in the summary.
```

This policy makes commit records sparse. A missing commit event does not mean
that no work occurred.

## Use a broad commit policy

Use this policy when commit recall is more important than a small journal:

```md
After every completed intentional commit, immediately run `lore record commit [flags] [summary]` while that commit is still `HEAD`.
Include features, fixes, refactors, documentation, tests, maintenance, and small deliberate changes.
Use the summary only when the commit subject does not explain the purpose or likely retrieval terms.
Supply provenance only when its values are known reliably.
Do not record generated-only, temporary, work-in-progress, or abandoned commits.
Never include credentials, tokens, personal data, or hidden reasoning in the summary.
```

This policy produces denser commit counts. Compare counts only when capture
policies are sufficiently similar.

## Define user-directed non-Git capture

Use a separate rule for activity that is not represented by a commit:

```md
When the user explicitly asks to preserve completed non-Git work, use `lore record worklog` when they provide a duration or `lore record note` for fallback information.
Record only the requested fact and context. This instruction does not authorize automatic conversation capture.
Never infer duration, scope, references, or provenance that the user did not supply or approve.
```

A `worklog` is suitable for configuration, investigation, coordination,
research, maintenance, or another task with explicit minutes. A `note` is the
fallback for user-directed information that has no more specific event type.
Neither requires a repository.

## Supply provenance carefully

Use `--source human` when an agent transports content explicitly supplied by
the user. Use `--source agent --harness <name>` for agent-authored records only
when the provenance is reliable. Harness version and model require a harness.

Do not guess provenance or place tool transcripts, prompts, hidden reasoning,
credentials, tokens, or personal data in summaries or references.
