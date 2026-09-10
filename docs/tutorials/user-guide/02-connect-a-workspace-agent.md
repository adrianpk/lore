# Connect a Workspace Agent

Add a Lore capture rule to one Git workspace. The rule tells the agent when to
record a completed commit and what context it may supply.

Add this selective policy to the workspace `AGENTS.md`:

```md
After creating a significant commit, immediately run `lore record commit [flags] [summary]` while that commit is still `HEAD`.
Record completed delivery outcomes, reusable decisions, accepted specifications, and non-obvious fixes.
Use the summary to state why the commit matters when the commit subject does not make that clear.
Supply provenance only when its values are known reliably.
Do not record routine, generated, formatting-only, punctuation-only, or work-in-progress commits.
Never include credentials, tokens, personal data, or hidden reasoning in the summary.
When the user explicitly asks to preserve non-Git activity, record the requested `worklog` or `note`; do not treat this as permission to capture other conversation content.
```

Lore derives commit properties and repository scope from the current
worktree. The agent should not duplicate those values in the summary.

Commit the workspace rule so later agent sessions receive the same capture
policy.

For a broader policy and guidance about choosing granularity, see [Choose an
agent capture policy](../../how-to/choose-an-agent-capture-policy.md).

Continue with [Record and retrieve
activity](03-record-and-retrieve-activity.md).
