package lore

import (
	"context"
	"errors"
	"testing"
)

func TestJournalShowResolvesEffectiveAndRawEvents(t *testing.T) {
	t.Parallel()

	fixture := newQueryFixture(t)

	effective, err := fixture.journal.Show(context.Background(), ShowParams{ID: fixture.milestoneA.ID})
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}

	if effective.Event.ID != fixture.correction.ID {
		t.Fatalf("effective event = %#v", effective.Event)
	}

	raw, err := fixture.journal.Show(context.Background(), ShowParams{ID: fixture.milestoneA.ID, Raw: true})
	if err != nil {
		t.Fatalf("raw Show() error = %v", err)
	}

	if raw.Event.ID != fixture.milestoneA.ID {
		t.Fatalf("raw event = %#v", raw.Event)
	}
}

func TestJournalShowRejectsMissingAndInvalidIDs(t *testing.T) {
	t.Parallel()

	fixture := newQueryFixture(t)
	tests := []struct {
		params ShowParams
		want   error
	}{
		{params: ShowParams{ID: "01K1Z8V7BMMN6W7Y6R8QJHA9ZZ"}, want: ErrEventNotFound},
		{params: ShowParams{}, want: ErrQueryInvalid},
		{params: ShowParams{ID: "invalid"}, want: ErrQueryInvalid},
	}

	for _, test := range tests {
		if _, err := fixture.journal.Show(context.Background(), test.params); !errors.Is(err, test.want) {
			t.Fatalf("Show(%#v) error = %v, want %v", test.params, err, test.want)
		}
	}
}
