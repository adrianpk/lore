package lore

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxNoteLength = 500

// Event is the canonical typed activity event.
type Event = Activity

// EventType is the canonical activity discriminator.
type EventType = ActivityType

// ExecutionContext contains legacy Git-capture provenance before it is mapped
// to canonical activity provenance.
type ExecutionContext struct {
	Harness        string `json:"harness"`
	HarnessVersion string `json:"harness_version,omitempty"`
	Model          string `json:"model,omitempty"`
}

// RepositoryReference contains read-only Git repository capture facts.
type RepositoryReference struct {
	Name     string `json:"name"`
	Root     string `json:"root"`
	Identity string `json:"identity"`
	Remote   string `json:"remote,omitempty"`
}

// GitReference contains read-only Git commit capture facts.
type GitReference struct {
	Commit        string    `json:"commit"`
	Branch        string    `json:"branch,omitempty"`
	Subject       string    `json:"subject"`
	AuthoredAt    time.Time `json:"authored_at"`
	CommittedAt   time.Time `json:"committed_at"`
	WorktreeClean bool      `json:"worktree_clean"`
}

// Validate checks legacy capture provenance before canonical mapping.
func (context ExecutionContext) Validate() error {
	if err := validateIdentifier("execution harness", context.Harness, true); err != nil {
		return err
	}

	if err := validateIdentifier("execution harness version", context.HarnessVersion, false); err != nil {
		return err
	}

	return validateIdentifier("execution model", context.Model, false)
}

// Validate checks read-only repository capture facts.
func (reference RepositoryReference) Validate() error {
	if err := validateIdentifier("repository name", reference.Name, true); err != nil {
		return err
	}

	if !filepath.IsAbs(reference.Root) {
		return fmt.Errorf("validate repository: root must be absolute")
	}

	if filepath.Clean(reference.Root) != reference.Root {
		return fmt.Errorf("validate repository: root must be clean")
	}

	if err := validateIdentifier("repository identity", reference.Identity, true); err != nil {
		return err
	}

	if err := validateIdentifier("repository remote", reference.Remote, false); err != nil {
		return err
	}

	return reference.validateCanonicalIdentity()
}

// Validate checks read-only Git commit capture facts.
func (reference GitReference) Validate() error {
	if !validObjectID(reference.Commit) {
		return fmt.Errorf("validate git reference: commit must be a full lowercase object ID")
	}

	if err := validateIdentifier("git branch", reference.Branch, false); err != nil {
		return err
	}

	if containsControl(reference.Subject) {
		return fmt.Errorf("validate git reference: subject contains a control character")
	}

	if err := validateUTCTime("git authored_at", reference.AuthoredAt); err != nil {
		return err
	}

	return validateUTCTime("git committed_at", reference.CommittedAt)
}

func validateUTCTime(name string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("validate event: %s is required", name)
	}

	_, offset := value.Zone()
	if offset != 0 {
		return fmt.Errorf("validate event: %s must use UTC", name)
	}

	return nil
}

func validateIdentifier(name, value string, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("validate event: %s is required", name)
		}

		return nil
	}

	if strings.TrimSpace(value) != value {
		return fmt.Errorf("validate event: %s must be trimmed", name)
	}

	if containsControl(value) {
		return fmt.Errorf("validate event: %s contains a control character", name)
	}

	return nil
}

func validateSignificanceText(name, value string, required bool) error {
	if err := validateIdentifier(name, value, required); err != nil {
		return err
	}

	if utf8.RuneCountInString(value) > maxNoteLength {
		return fmt.Errorf("validate event: %s exceeds %d Unicode code points", name, maxNoteLength)
	}

	return nil
}

func validObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}

	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}

	return true
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}

	return false
}
