package lore

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewEventID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		timestamp time.Time
		entropy   string
		want      EventID
		wantErr   bool
	}{
		{
			name:      "zero value",
			timestamp: time.UnixMilli(0).UTC(),
			entropy:   strings.Repeat("\x00", 10),
			want:      "00000000000000000000000000",
		},
		{
			name:      "before epoch",
			timestamp: time.UnixMilli(-1).UTC(),
			entropy:   strings.Repeat("\x00", 10),
			wantErr:   true,
		},
		{
			name:      "short entropy",
			timestamp: time.UnixMilli(1).UTC(),
			entropy:   "short",
			wantErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertNewEventID(t, test.timestamp, test.entropy, test.want, test.wantErr)
		})
	}
}

func assertNewEventID(t *testing.T, timestamp time.Time, entropy string, want EventID, wantErr bool) {
	t.Helper()

	got, err := NewEventID(timestamp, strings.NewReader(entropy))
	if wantErr {
		if err == nil {
			t.Fatal("expected an error")
		}

		return
	}

	if err != nil {
		t.Fatalf("NewEventID() error = %v", err)
	}

	if got != want {
		t.Fatalf("NewEventID() = %q, want %q", got, want)
	}

	if err := got.Validate(); err != nil {
		t.Fatalf("generated ID is invalid: %v", err)
	}
}

func TestNewEventIDSortsByTime(t *testing.T) {
	t.Parallel()

	entropy := strings.Repeat("\x00", 10)

	first, err := NewEventID(time.UnixMilli(1).UTC(), strings.NewReader(entropy))
	if err != nil {
		t.Fatalf("generate first ID: %v", err)
	}

	second, err := NewEventID(time.UnixMilli(2).UTC(), strings.NewReader(entropy))
	if err != nil {
		t.Fatalf("generate second ID: %v", err)
	}

	if first >= second {
		t.Fatalf("IDs are not time sortable: %q >= %q", first, second)
	}
}

func TestEventIDValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      EventID
		wantErr bool
	}{
		{name: "valid", id: "01K1Z8V7BMMN6W7Y6R8QJHA9SS"},
		{name: "lowercase", id: "01k1z8v7bmmn6w7y6r8qjha9ss", wantErr: true},
		{name: "invalid alphabet", id: "01K1Z8V7BMMN6W7Y6R8QJHA9SI", wantErr: true},
		{name: "overflow", id: "81K1Z8V7BMMN6W7Y6R8QJHA9SS", wantErr: true},
		{name: "length", id: "01K1Z8V7", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.id.Validate()
			if test.wantErr && err == nil {
				t.Fatal("expected an error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestNewEventIDEntropyError(t *testing.T) {
	t.Parallel()

	if _, err := NewEventID(time.UnixMilli(1).UTC(), failingReader{}); err == nil {
		t.Fatal("expected an error")
	}
}
