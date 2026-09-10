# Lore Knowledge Model Reference

## Canonical Entry

```json
{
  "id": "01M26K00000000000000000000",
  "kind": "procedure",
  "status": "active",
  "title": "Recover the local preview service",
  "body": "1. Inspect the user unit.\n2. Verify the bound port.\n3. Restart only after the failure is understood.",
  "recorded_at": "2026-09-10T21:19:39Z",
  "verified_at": "2026-09-10T21:18:00Z",
  "contexts": [
    {
      "type": "project",
      "name": "example",
      "repository": "forge.example/adrian/example"
    },
    {
      "type": "host",
      "name": "zeus"
    }
  ],
  "labels": ["recovery", "service"],
  "references": [
    {
      "type": "file",
      "value": "docs/reference/recovery.md",
      "label": "recovery runbook"
    }
  ],
  "provenance": {
    "type": "agent",
    "harness": "codex",
    "harness_version": "1",
    "model": "gpt-5"
  }
}
```

Optional fields are omitted rather than serialized as empty strings, empty
objects, or null values. Empty context, label, and reference arrays may be
omitted from stored JSON and normalize to empty collections on read.

## Fields

| Field | Type | Required | Contract |
| --- | --- | --- | --- |
| `id` | ULID string | yes | Unique immutable revision identity |
| `kind` | enum | yes | `procedure`, `fact`, `warning`, `reference` |
| `status` | enum | yes | `active`, `archived`, `invalidated` |
| `title` | string | yes | Trimmed, single line, 1-200 code points |
| `body` | string | yes | Trimmed Markdown, 1-32 KiB |
| `recorded_at` | RFC 3339 UTC | yes | Assigned when accepted |
| `verified_at` | RFC 3339 UTC | no | Concrete verification, not after `recorded_at` |
| `contexts` | context array | no | At most 16 unique contexts |
| `labels` | string array | no | At most 8 unique canonical topics |
| `references` | reference array | no | At most 32 unique references |
| `provenance` | object | no | Descriptive capture origin |
| `supersedes` | ULID string | revision only | Current effective predecessor |
| `revision_reason` | string | revision only | Trimmed explanation, 1-500 code points |

## Context

```json
{
  "type": "repository",
  "name": "lore",
  "repository": "forge.adrianpk.com/adrianpk/lore"
}
```

| Field | Type | Required | Contract |
| --- | --- | --- | --- |
| `type` | enum | yes | `project`, `repository`, `system`, `host`, `technology`, `client` |
| `name` | string | yes | Canonical lowercase kebab name, at most 64 bytes |
| `repository` | string | no | Trimmed full identity, at most 512 bytes, no credentials |

Canonical name grammar is `[a-z0-9]+(?:-[a-z0-9]+)*`. A context identity is
the tuple `(type, name, repository)`.

## Label

A label is a canonical string using the context-name grammar and a maximum of
64 bytes. Labels describe topics only. Array order is preserved for display;
identity and duplicate checks use the canonical value.

## Reference

```json
{
  "type": "url",
  "value": "https://example.com/runbook",
  "label": "service runbook"
}
```

| Field | Type | Required | Contract |
| --- | --- | --- | --- |
| `type` | enum | yes | `url`, `file`, `repository`, `commit` |
| `value` | string | yes | Trimmed, 1-2,048 bytes, no credentials |
| `label` | string | no | Trimmed, single line, at most 64 code points |

A reference identity is `(type, value)`. Labels do not permit duplicate
references with different display text.

## Provenance

```json
{
  "type": "agent",
  "harness": "codex",
  "harness_version": "1",
  "model": "gpt-5"
}
```

`type` is required when provenance is present and is one of `human`, `agent`,
or `integration`. Other fields are optional, trimmed, single-line strings of at
most 64 code points.

## Complete Revision

```json
{
  "id": "01M26K10000000000000000000",
  "kind": "procedure",
  "status": "archived",
  "title": "Recover the local preview service",
  "body": "1. Inspect the user unit.\n2. Verify the bound port.\n3. Restart only after the failure is understood.",
  "recorded_at": "2026-09-11T08:00:00Z",
  "contexts": [
    {"type": "project", "name": "example"}
  ],
  "labels": ["recovery", "service"],
  "supersedes": "01M26K00000000000000000000",
  "revision_reason": "The preview service was retired."
}
```

Every revision repeats the full effective content. Consumers never merge JSON
objects across a chain. `verified_at` may be omitted to state that the revised
knowledge has not yet been concretely verified.

## Effective View

For each root entry, Lore follows the unique successor links to the terminal
revision. That terminal revision is effective. A missing predecessor, duplicate
successor, self-reference, cycle, or kind change invalidates the chain and must
produce an explicit error rather than a partial result.

## Storage Layout

```text
lore/
  entries/
    2026/
      09.jsonl
  pending/
    <entry-id>.json
```

The pending filename is derived from the entry ID. Recovery compares the exact
canonical pending line with the target month before appending or clearing it.

## Query Projection

Human list output contains `id`, `kind`, `status`, `title`, `recorded_at`, and
compact context names. JSON output returns complete effective entries unless a
documented projection flag is selected. Query limits apply before rendering
and never silently exceed 200 entries.
