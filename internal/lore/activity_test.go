package lore

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

type activityValidationCase struct {
	name    string
	change  func(*Activity)
	wantErr bool
}

func TestActivityValidatePeerTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		activity Activity
	}{
		{name: "commit", activity: validCommitActivity()},
		{name: "pull request", activity: validPullRequestActivity()},
		{name: "worklog", activity: validWorklogActivity()},
		{name: "note", activity: validNoteActivity()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := test.activity.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestActivityValidateCommonFields(t *testing.T) {
	t.Parallel()

	tests := []activityValidationCase{
		{name: "valid"},
		{name: "ID", change: func(activity *Activity) { activity.ID = "invalid" }, wantErr: true},
		{name: "occurrence time", change: func(activity *Activity) { activity.OccurredAt = time.Time{} }, wantErr: true},
		{
			name: "future occurrence",
			change: func(activity *Activity) {
				activity.OccurredAt = activity.RecordedAt.Add(time.Second)
			},
			wantErr: true,
		},
		{name: "summary", change: func(activity *Activity) { activity.Summary = "" }, wantErr: true},
		{
			name:    "summary length",
			change:  func(activity *Activity) { activity.Summary = strings.Repeat("x", maxNoteLength+1) },
			wantErr: true,
		},
		{
			name:    "scope",
			change:  func(activity *Activity) { activity.Scope.Type = "workspace" },
			wantErr: true,
		},
	}

	assertActivityValidationCases(t, validWorklogActivity(), tests)
}

func TestActivityValidateTypeProperties(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		activity Activity
		change   func(*Activity)
	}{
		{
			name:     "commit requires repository scope",
			activity: validCommitActivity(),
			change:   func(activity *Activity) { activity.Scope = nil },
		},
		{
			name:     "commit rejects pull request fields",
			activity: validCommitActivity(),
			change:   func(activity *Activity) { activity.Properties.Number = 1 },
		},
		{
			name:     "pull request requires positive number",
			activity: validPullRequestActivity(),
			change:   func(activity *Activity) { activity.Properties.Number = 0 },
		},
		{
			name:     "pull request requires action",
			activity: validPullRequestActivity(),
			change:   func(activity *Activity) { activity.Properties.Action = "updated" },
		},
		{
			name:     "pull request scope is derived from properties",
			activity: validPullRequestActivity(),
			change:   func(activity *Activity) { activity.Scope.Identity = "https://forge.example.test/team/other" },
		},
		{
			name:     "worklog requires duration",
			activity: validWorklogActivity(),
			change:   func(activity *Activity) { activity.Properties.DurationMinutes = 0 },
		},
		{
			name:     "worklog duration is bounded",
			activity: validWorklogActivity(),
			change:   func(activity *Activity) { activity.Properties.DurationMinutes = maxWorklogMinutes + 1 },
		},
		{
			name:     "note rejects properties",
			activity: validNoteActivity(),
			change:   func(activity *Activity) { activity.Properties = &ActivityProperties{} },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.change(&test.activity)

			if err := test.activity.Validate(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestActivityValidateReferences(t *testing.T) {
	t.Parallel()

	tests := []activityValidationCase{
		{name: "valid"},
		{
			name: "duplicate",
			change: func(activity *Activity) {
				activity.References = append(activity.References, activity.References[0])
			},
			wantErr: true,
		},
		{
			name: "too many",
			change: func(activity *Activity) {
				activity.References = make([]ActivityReference, maxActivityReferences+1)
			},
			wantErr: true,
		},
		{
			name: "credentials",
			change: func(activity *Activity) {
				activity.References[0].URL = "https://user:secret@example.test/dashboard"
			},
			wantErr: true,
		},
	}

	activity := validNoteActivity()
	activity.References = []ActivityReference{{Type: ReferenceTypeURL, URL: "https://example.test/dashboard"}}
	assertActivityValidationCases(t, activity, tests)
}

func TestActivityValidateProvenance(t *testing.T) {
	t.Parallel()

	tests := []activityValidationCase{
		{name: "valid"},
		{
			name:    "agent requires harness",
			change:  func(activity *Activity) { activity.Provenance.Harness = "" },
			wantErr: true,
		},
		{
			name: "human rejects agent fields",
			change: func(activity *Activity) {
				activity.Provenance.Type = ProvenanceTypeHuman
			},
			wantErr: true,
		},
		{
			name:    "control character",
			change:  func(activity *Activity) { activity.Provenance.Model = "provider/model\nsecret" },
			wantErr: true,
		},
	}

	activity := validNoteActivity()
	activity.Provenance = &ActivityProvenance{
		Type:           ProvenanceTypeAgent,
		Harness:        "codex",
		HarnessVersion: "1.0.0",
		Model:          "provider/model",
	}
	assertActivityValidationCases(t, activity, tests)
}

func TestActivityValidateCorrection(t *testing.T) {
	t.Parallel()

	tests := []activityValidationCase{
		{name: "valid"},
		{
			name:    "reason without target",
			change:  func(activity *Activity) { activity.Supersedes = "" },
			wantErr: true,
		},
		{
			name:    "missing reason",
			change:  func(activity *Activity) { activity.CorrectionReason = "" },
			wantErr: true,
		},
		{
			name:    "self target",
			change:  func(activity *Activity) { activity.Supersedes = activity.ID },
			wantErr: true,
		},
	}

	activity := validNoteActivity()
	activity.Supersedes = "01K1Z8V7BMMN6W7Y6R8QJHA9ST"
	activity.CorrectionReason = "Clarify the operating boundary"
	assertActivityValidationCases(t, activity, tests)
}

func assertActivityValidationCases(t *testing.T, valid Activity, tests []activityValidationCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			activity := copyActivity(valid)
			if test.change != nil {
				test.change(&activity)
			}

			err := activity.Validate()
			if test.wantErr && err == nil {
				t.Fatal("expected an error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func copyActivity(activity Activity) Activity {
	if activity.Scope != nil {
		scope := *activity.Scope
		activity.Scope = &scope
	}

	if activity.Properties != nil {
		properties := *activity.Properties
		activity.Properties = &properties
	}

	if activity.Provenance != nil {
		provenance := *activity.Provenance
		activity.Provenance = &provenance
	}

	activity.References = append([]ActivityReference(nil), activity.References...)

	return activity
}

func validCommitActivity() Activity {
	activity := baseActivity(ActivityTypeCommit, "Record the canonical typed activity model")
	activity.Scope = &ActivityScope{
		Type:     ScopeTypeRepository,
		Name:     "example",
		Identity: "https://example.test/team/example.git",
	}
	activity.Properties = &ActivityProperties{
		ObjectID: "0123456789abcdef0123456789abcdef01234567",
		Subject:  "feat: add canonical typed activity model",
	}

	return activity
}

func validPullRequestActivity() Activity {
	activity := baseActivity(ActivityTypePullRequest, "Merge the typed activity pull request")
	activity.Scope = &ActivityScope{
		Type:     ScopeTypeRepository,
		Name:     "team/example",
		Identity: "https://forge.example.test/team/example",
	}
	activity.Properties = &ActivityProperties{
		Forge:      "https://forge.example.test",
		Repository: "team/example",
		Number:     42,
		Action:     PullRequestActionMerged,
		URL:        "https://forge.example.test/team/example/pulls/42",
	}

	return activity
}

func validWorklogActivity() Activity {
	activity := baseActivity(ActivityTypeWorklog, "Configured Grafana alert routing")
	activity.Scope = &ActivityScope{
		Type:     ScopeTypeSystem,
		Name:     "Production observability",
		Identity: "system:production-observability",
	}
	activity.Properties = &ActivityProperties{DurationMinutes: 240}

	return activity
}

func validNoteActivity() Activity {
	return baseActivity(ActivityTypeNote, "Preserve the agreed operating boundary")
}

func baseActivity(activityType ActivityType, summary string) Activity {
	recordedAt := time.Date(2026, 8, 5, 14, 22, 31, 418000000, time.UTC)

	return Activity{
		ID:         testEventID(recordedAt, 1),
		Type:       activityType,
		OccurredAt: time.Date(2026, 8, 5, 14, 20, 47, 0, time.UTC),
		RecordedAt: recordedAt,
		Summary:    summary,
	}
}

func testEventID(recordedAt time.Time, marker byte) EventID {
	id, err := NewEventID(recordedAt, bytes.NewReader(bytes.Repeat([]byte{marker}, 10)))
	if err != nil {
		panic(err)
	}

	return id
}
