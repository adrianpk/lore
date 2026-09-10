# Raw Questionnaire Capture

## Topic

Define the minimum Lore knowledge model and its capture, retrieval, lifecycle,
and pruning boundaries before writing a specification.

## Questionnaire State

Questions asked: 8.

## Question 1: Initial Entry Kinds

Should Lore use one generic entry or several strict kinds?

Answer: use four initial kinds: `procedure`, `fact`, `warning`, and
`reference`.

Status: answered

## Question 2: Content Structure

Should each kind have specialized fields or share flexible content?

Answer: avoid a complicated schema before real usage demonstrates a need.
Use a short title plus one description or body field and labels. Do not store
both description and body. Markdown can express steps, explanations, and
avoidance guidance inside the single body.

Status: answered; `body` is the working field name

## Question 3: Contexts And Labels

Should applicability depend only on labels or use structured contexts?

Answer: labels would become noisy if used for arbitrary metadata. Use
structured contexts for project and repository semantics and retain labels for
topics. Allow several context types, including project, repository, system,
host, technology, and client.

Clarification: every context should have a short canonical name. Prefer a
lowercase normalized name inferred from the project-root directory. Normal
interaction should not require a long repository identity, although the full
repository can remain an optional field.

Status: answered

## Question 4: Verification

Should Lore accept only verified knowledge?

Answer: do not make verification elaborate. Entries may exist without a
verification timestamp. Record `verified_at` only when a concrete check took
place, and do not add confidence scales or extra prompts.

Status: answered

## Question 5: States And Revisions

Are `active`, `archived`, and `invalidated` sufficient, with complete revisions
linked through `supersedes`?

Answer: yes.

Status: answered

## Question 6: Autonomous Capture

Should an authorized agent write directly or request approval for every
candidate?

Answer: write directly without interrupting the user. Per-entry approval would
make steering noisy and frustrating. Prefer later pruning over continuous
capture confirmations. Explicit user-directed capture must also remain
available.

Status: answered

## Question 7: Capture Evaluation And Pruning

Should capture and pruning run periodically or on another trigger?

Answer: apply a quick capture-value test after every substantive turn, but use
a high importance threshold so the store does not fill with low-value details.
Do not batch capture every fixed number of prompts. Review and pruning run only
when explicitly requested during the initial product stage.

Status: answered

## Question 8: Physical Purge

How should physical deletion be invoked initially?

Answer: only through explicit invocation. It must not run autonomously.

Status: answered; exact selectors and confirmation mechanics remain open
