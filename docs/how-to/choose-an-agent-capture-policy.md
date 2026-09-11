# Choose an Agent Capture Policy

Keep retrieval and capture as separate policy decisions. Retrieval is
read-only; capture changes the durable knowledge store.

## Start with explicit capture

Use this policy first:

```md
Query Lore when applicable stored knowledge could avoid a costly repeated inference.
Record knowledge only when the user explicitly asks. Curate the reusable result into one fact, procedure, warning, or reference.
Do not preserve the conversation, hidden reasoning, credentials, tokens, or personal data.
```

An explicit request already authorizes the record. Do not interrupt the active
workflow with another approval prompt.

## Select only useful context

Use `project:<name>` for the short canonical project identity. Prefer the
lowercase canonicalized directory name. Add `repository:<name>@<identity>`
when repository identity changes applicability. Use `system`, `host`,
`technology`, or `client` only when that dimension matters.

Labels are optional topical hints. Keep them sparse because contexts provide
the stable structural dimensions.

## Consider autonomous capture later

Autonomous capture should be a deliberate workspace opt-in. Its policy must
require high-value, reusable knowledge such as a non-obvious recovery
procedure, hard constraint, recurring failure mode, or expensive discovery.
It should exclude routine commands, transient state, obvious documentation,
and progress narration.

Review store growth and false positives before broadening the policy. The
current recommended configuration remains explicit capture.
