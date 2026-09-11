package lore

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	// ErrPurgeConfirmation indicates missing or stale dry-run evidence.
	ErrPurgeConfirmation = errors.New("purge confirmation does not match the current store and selection")
	// ErrPurgeSelector indicates an unsafe or invalid destructive selector.
	ErrPurgeSelector = errors.New("purge selector is invalid")
)

const purgeTokenVersion = "lore/purge/v1"

// PurgeParams selects complete chains and chooses dry-run or confirmed mode.
type PurgeParams struct {
	IDs            []EntryID     `json:"ids,omitempty"`
	RecordedBefore *time.Time    `json:"recorded_before,omitempty"`
	Statuses       []EntryStatus `json:"statuses,omitempty"`
	All            bool          `json:"all,omitempty"`
	DryRun         bool          `json:"dry_run,omitempty"`
	Confirm        string        `json:"confirm,omitempty"`
}

// PurgeChain identifies one exact complete revision chain.
type PurgeChain struct {
	RootID   EntryID   `json:"root_id"`
	EntryIDs []EntryID `json:"entry_ids"`
}

// PurgeResult reports selected chains and dry-run confirmation evidence.
type PurgeResult struct {
	Chains            []PurgeChain `json:"chains"`
	ChainCount        int          `json:"chain_count"`
	RevisionCount     int          `json:"revision_count"`
	ConfirmationToken string       `json:"confirmation_token,omitempty"`
	Purged            bool         `json:"purged"`
}

type canonicalPurgeSelector struct {
	IDs            []EntryID     `json:"ids,omitempty"`
	RecordedBefore *time.Time    `json:"recorded_before,omitempty"`
	Statuses       []EntryStatus `json:"statuses,omitempty"`
	All            bool          `json:"all,omitempty"`
}

// Purge dry-runs or removes exact complete chains after confirmation.
func (journal *EntryJournal) Purge(ctx context.Context, params PurgeParams) (PurgeResult, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()

	if err := journal.refreshForPurge(ctx); err != nil {
		return PurgeResult{}, err
	}

	selector, err := canonicalizePurgeParams(params)
	if err != nil {
		return PurgeResult{}, err
	}

	chains, roots, err := selectPurgeChains(selector, journal.index)
	if err != nil {
		return PurgeResult{}, err
	}

	digest, err := entryStoreDigest(ctx, journal.config.EntriesRoot)
	if err != nil {
		return PurgeResult{}, err
	}

	token, err := purgeConfirmationToken(selector, chains, digest)
	if err != nil {
		return PurgeResult{}, err
	}

	result := newPurgeResult(chains)
	if params.DryRun {
		result.ConfirmationToken = token

		return result, nil
	}

	if !confirmationMatches(params.Confirm, token) {
		return PurgeResult{}, ErrPurgeConfirmation
	}

	if err := rewriteEntryStore(ctx, journal.config, journal.index, roots); err != nil {
		journal.loaded = false

		return PurgeResult{}, err
	}

	index, err := rebuildEntryIndex(ctx, journal.config.EntriesRoot)
	if err != nil {
		journal.loaded = false

		return PurgeResult{}, err
	}

	journal.index = index
	result.Purged = true

	return result, nil
}

func (journal *EntryJournal) refreshForPurge(ctx context.Context) error {
	if !journal.loaded {
		return ErrEntryJournalNotLoaded
	}

	if err := journal.recoverForAppend(ctx); err != nil {
		return err
	}

	index, err := rebuildEntryIndex(ctx, journal.config.EntriesRoot)
	if err != nil {
		journal.loaded = false

		return err
	}

	journal.index = index

	return nil
}

func confirmationMatches(provided, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func canonicalizePurgeParams(params PurgeParams) (canonicalPurgeSelector, error) {
	if params.DryRun == (strings.TrimSpace(params.Confirm) != "") {
		return canonicalPurgeSelector{}, fmt.Errorf("%w: choose exactly one of dry_run or confirm", ErrPurgeSelector)
	}

	selector := canonicalPurgeSelector{
		IDs:            append([]EntryID(nil), params.IDs...),
		RecordedBefore: cloneTime(params.RecordedBefore),
		Statuses:       append([]EntryStatus(nil), params.Statuses...),
		All:            params.All,
	}

	if !hasPurgeSelector(selector) {
		return canonicalPurgeSelector{}, fmt.Errorf("%w: at least one selector is required", ErrPurgeSelector)
	}

	if selector.All && (len(selector.IDs) > 0 || selector.RecordedBefore != nil || len(selector.Statuses) > 0) {
		return canonicalPurgeSelector{}, fmt.Errorf("%w: all cannot be combined with another selector", ErrPurgeSelector)
	}

	if err := canonicalizePurgeSelector(&selector); err != nil {
		return canonicalPurgeSelector{}, err
	}

	return selector, nil
}

func canonicalizePurgeSelector(selector *canonicalPurgeSelector) error {
	for _, id := range selector.IDs {
		if err := id.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrPurgeSelector, err)
		}
	}

	for _, status := range selector.Statuses {
		if !status.Valid() {
			return fmt.Errorf("%w: unsupported status %q", ErrPurgeSelector, status)
		}
	}

	if selector.RecordedBefore != nil {
		if err := validateUTCTime("purge recorded_before", *selector.RecordedBefore); err != nil {
			return fmt.Errorf("%w: %w", ErrPurgeSelector, err)
		}
	}

	sort.Slice(selector.IDs, func(left, right int) bool { return selector.IDs[left] < selector.IDs[right] })

	selector.IDs = compactComparable(selector.IDs)
	sort.Slice(selector.Statuses, func(left, right int) bool { return selector.Statuses[left] < selector.Statuses[right] })

	selector.Statuses = compactComparable(selector.Statuses)

	return nil
}

func hasPurgeSelector(selector canonicalPurgeSelector) bool {
	return selector.All || len(selector.IDs) > 0 || selector.RecordedBefore != nil || len(selector.Statuses) > 0
}

func selectPurgeChains(selector canonicalPurgeSelector, index *entryIndex) ([]PurgeChain, map[EntryID]struct{}, error) {
	roots := make(map[EntryID]struct{})

	for _, id := range selector.IDs {
		root, exists := index.roots[id]
		if !exists {
			return nil, nil, fmt.Errorf("%w: entry %q does not exist", ErrPurgeSelector, id)
		}

		roots[root] = struct{}{}
	}

	for root, effectiveID := range index.effective {
		entry := index.entries[effectiveID]
		if selector.All || matchesPurgeEffectiveEntry(entry, selector) {
			roots[root] = struct{}{}
		}
	}

	chains := make([]PurgeChain, 0, len(roots))
	for root := range roots {
		chains = append(chains, PurgeChain{RootID: root, EntryIDs: entryChainIDs(root, index)})
	}

	sort.Slice(chains, func(left, right int) bool { return chains[left].RootID < chains[right].RootID })

	return chains, roots, nil
}

func matchesPurgeEffectiveEntry(entry Entry, selector canonicalPurgeSelector) bool {
	return selector.RecordedBefore != nil && entry.RecordedAt.Before(*selector.RecordedBefore) ||
		len(selector.Statuses) > 0 && matchesStatus(entry.Status, selector.Statuses)
}

func entryChainIDs(root EntryID, index *entryIndex) []EntryID {
	var ids []EntryID
	for current := root; current != ""; current = index.successors[current] {
		ids = append(ids, current)
	}

	return ids
}

func purgeConfirmationToken(selector canonicalPurgeSelector, chains []PurgeChain, digest [sha256.Size]byte) (string, error) {
	selection, err := json.Marshal(struct {
		Version  string                 `json:"version"`
		Selector canonicalPurgeSelector `json:"selector"`
		Chains   []PurgeChain           `json:"chains"`
		Digest   string                 `json:"digest"`
	}{
		Version:  purgeTokenVersion,
		Selector: selector,
		Chains:   chains,
		Digest:   hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return "", fmt.Errorf("encode purge confirmation: %w", err)
	}

	token := sha256.Sum256(selection)

	return hex.EncodeToString(token[:]), nil
}

func newPurgeResult(chains []PurgeChain) PurgeResult {
	result := PurgeResult{Chains: chains, ChainCount: len(chains)}
	for _, chain := range chains {
		result.RevisionCount += len(chain.EntryIDs)
	}

	return result
}

func compactComparable[T comparable](values []T) []T {
	if len(values) < 2 {
		return values
	}

	compacted := values[:1]
	for _, value := range values[1:] {
		if value != compacted[len(compacted)-1] {
			compacted = append(compacted, value)
		}
	}

	return compacted
}
