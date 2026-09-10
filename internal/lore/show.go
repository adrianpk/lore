package lore

import (
	"context"
	"errors"
	"fmt"
)

// ErrEventNotFound identifies a retrieval with no effective event.
var ErrEventNotFound = errors.New("event was not found")

// ShowParams selects one effective event by ID.
type ShowParams struct {
	ID  EventID `json:"id"`
	Raw bool    `json:"raw,omitempty"`
}

// ShowResult contains one effective event.
type ShowResult struct {
	Event Event `json:"event"`
}

// Show returns one event by ID, resolving its correction chain by default.
func (journal *Journal) Show(ctx context.Context, params ShowParams) (ShowResult, error) {
	if err := ctx.Err(); err != nil {
		return ShowResult{}, err
	}

	if err := params.ID.Validate(); err != nil {
		return ShowResult{}, fmt.Errorf("%w: invalid event ID: %w", ErrQueryInvalid, err)
	}

	var (
		event Event
		ok    bool
	)
	if params.Raw {
		event, ok = journal.Event(params.ID)
	} else {
		event, ok = journal.Effective(params.ID)
	}

	if !ok {
		return ShowResult{}, fmt.Errorf("%w: event %q", ErrEventNotFound, params.ID)
	}

	return ShowResult{Event: event}, nil
}
