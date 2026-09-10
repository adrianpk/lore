# Local Journal Model

Lore uses one per-user local service as the sole canonical writer. Agents and
CLI processes send bounded operations through an owner-only Unix socket. This
serializes concurrent writes without requiring clients to coordinate file
access.

Every event is a complete canonical JSON object stored on one line in a monthly
UTC segment. The common envelope carries identity, type, occurrence and
recording times, and summary. Optional scope, references, and provenance are
independent from strict properties selected by `commit`, `pull_request`,
`worklog`, or `note`.

The service assigns the event ID and recording time. Occurrence time describes
when the activity happened and can be earlier. Segment placement follows the
recording time, while reports group by occurrence time in the requested local
timezone.

Corrections append a new self-contained event of the same type that supersedes
the current effective event. Published lines do not change. Search and reports
operate on effective events and return compact projections. Show returns a
complete effective or explicitly requested raw event.

Lore keeps journal and query work local. It does not contact a forge, infer
duration, or require a Git artifact. Git and other referenced systems remain
the detailed sources for their own artifacts; Lore stores typed activity and
optional routes back to those sources.
