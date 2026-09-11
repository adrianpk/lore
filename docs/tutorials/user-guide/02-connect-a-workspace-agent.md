# Connect a Workspace Agent

Add a Lore rule to one Git workspace. The initial policy allows retrieval when
useful and capture only after an explicit user request.

Add this policy to the workspace `AGENTS.md`:

```md
When stored operational knowledge may materially reduce repeated inference, query Lore with `lore search` before reconstructing the workflow.
Record knowledge only when the user explicitly asks to preserve it. Curate the reusable fact, procedure, warning, or reference rather than storing the conversation.
Use `lore record <kind>` with the narrowest applicable contexts and only useful topic labels.
Prefer the canonical lowercase project directory name as the `project` context name. Include a repository identity only when it improves applicability.
Never store credentials, tokens, personal data, prompts, transcripts, hidden reasoning, or unnecessary source content.
Do not ask for separate confirmation after the user has explicitly requested capture.
```

When no project context is supplied, the CLI attempts to infer one from the
current Git worktree. Supplying the context explicitly keeps agent behavior
predictable.

Commit the workspace rule so later agent sessions receive the same capture
policy. This repository does not install or activate a global agent rule.

For policy variants and capture granularity, see [Choose an agent capture
policy](../../how-to/choose-an-agent-capture-policy.md).

Continue with [Record and retrieve
knowledge](03-record-and-retrieve-knowledge.md).
