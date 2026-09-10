# Correct and Verify the Journal

Append a correction when an activity record is inaccurate. Verify the journal
when storage integrity is in doubt or after restoring local data.

## Find the effective event ID

Search for the activity and copy its event ID:

```sh
lore search --query "alert routing"
```

Inspect the complete effective event before changing it:

```sh
lore show --id <event-id> --json
```

## Append a correction

Replace a summary:

```sh
lore correct \
  --target <event-id> \
  --reason "Clarify the affected behavior" \
  --summary "Configured alert routing for the production contact point"
```

Correct a worklog occurrence time and duration:

```sh
lore correct \
  --target <event-id> \
  --reason "Correct the timesheet facts" \
  --at 2026-09-03T08:00:00Z \
  --duration 180
```

Omit any replacement option whose current value should remain unchanged. The
target must be the current effective event. Lore appends a complete replacement
of the same type and does not rewrite the earlier line. `--duration` is valid
only for worklogs.

Use `lore show --id <old-event-id>` to resolve an earlier chain member to the
new effective event. Add `--raw` to retrieve the exact earlier event.

## Verify journal integrity

Verify the active monthly segment:

```sh
lore verify
```

Verify every canonical segment:

```sh
lore verify --full
```

Use `--json` when a client needs structured results. On corruption, Lore
reports the segment, line, byte offset, and reason. Verification is read-only
and does not perform general repair.
