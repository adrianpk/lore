package lore

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	maxActivityReferences = 32
	maxWorklogMinutes     = 24 * 60
)

// ActivityType identifies one peer activity variant.
type ActivityType string

const (
	// ActivityTypeCommit records one Git commit.
	ActivityTypeCommit ActivityType = "commit"
	// ActivityTypePullRequest records one pull-request lifecycle action.
	ActivityTypePullRequest ActivityType = "pull_request"
	// ActivityTypeWorklog records explicitly declared work duration.
	ActivityTypeWorklog ActivityType = "worklog"
	// ActivityTypeNote records user-directed information.
	ActivityTypeNote ActivityType = "note"
)

// ScopeType identifies the context in which activity occurred.
type ScopeType string

const (
	// ScopeTypeRepository identifies a source-code repository.
	ScopeTypeRepository ScopeType = "repository"
	// ScopeTypeProject identifies a project that is not necessarily a repository.
	ScopeTypeProject ScopeType = "project"
	// ScopeTypeSystem identifies an operated technical system.
	ScopeTypeSystem ScopeType = "system"
	// ScopeTypeClient identifies a client context.
	ScopeTypeClient ScopeType = "client"
	// ScopeTypeOther identifies a deliberately named custom context.
	ScopeTypeOther ScopeType = "other"
)

// PullRequestAction identifies a recordable pull-request lifecycle action.
type PullRequestAction string

const (
	// PullRequestActionOpened records pull-request creation.
	PullRequestActionOpened PullRequestAction = "opened"
	// PullRequestActionReviewed records a completed review.
	PullRequestActionReviewed PullRequestAction = "reviewed"
	// PullRequestActionMerged records integration.
	PullRequestActionMerged PullRequestAction = "merged"
	// PullRequestActionClosed records closure without integration.
	PullRequestActionClosed PullRequestAction = "closed"
)

// ReferenceType identifies an optional related object.
type ReferenceType string

const (
	// ReferenceTypeGitCommit identifies a related commit.
	ReferenceTypeGitCommit ReferenceType = "git_commit"
	// ReferenceTypePullRequest identifies a related pull request.
	ReferenceTypePullRequest ReferenceType = "pull_request"
	// ReferenceTypeURL identifies a generic related URL.
	ReferenceTypeURL ReferenceType = "url"
)

// ProvenanceType identifies the semantic source of a record.
type ProvenanceType string

const (
	// ProvenanceTypeHuman identifies human-supplied content.
	ProvenanceTypeHuman ProvenanceType = "human"
	// ProvenanceTypeAgent identifies agent-supplied content.
	ProvenanceTypeAgent ProvenanceType = "agent"
	// ProvenanceTypeIntegration identifies integration-supplied content.
	ProvenanceTypeIntegration ProvenanceType = "integration"
)

// Activity is one canonical typed journal event.
type Activity struct {
	ID               EventID             `json:"id"`
	Type             ActivityType        `json:"type"`
	OccurredAt       time.Time           `json:"occurred_at"`
	RecordedAt       time.Time           `json:"recorded_at"`
	Summary          string              `json:"summary"`
	Scope            *ActivityScope      `json:"scope,omitempty"`
	Properties       *ActivityProperties `json:"properties,omitempty"`
	References       []ActivityReference `json:"references,omitempty"`
	Provenance       *ActivityProvenance `json:"provenance,omitempty"`
	Supersedes       EventID             `json:"supersedes,omitempty"`
	CorrectionReason string              `json:"correction_reason,omitempty"`
}

// ActivityScope identifies one stable activity context.
type ActivityScope struct {
	Type     ScopeType `json:"type"`
	Name     string    `json:"name"`
	Identity string    `json:"identity"`
}

// ActivityProperties contains the strict fields selected by Activity.Type.
type ActivityProperties struct {
	ObjectID        string            `json:"object_id,omitempty"`
	Subject         string            `json:"subject,omitempty"`
	AuthoredAt      *time.Time        `json:"authored_at,omitempty"`
	Forge           string            `json:"forge,omitempty"`
	Repository      string            `json:"repository,omitempty"`
	Number          int               `json:"number,omitempty"`
	Action          PullRequestAction `json:"action,omitempty"`
	URL             string            `json:"url,omitempty"`
	DurationMinutes int               `json:"duration_minutes,omitempty"`
}

// ActivityReference identifies one optional related object.
type ActivityReference struct {
	Type          ReferenceType `json:"type"`
	ScopeIdentity string        `json:"scope_identity,omitempty"`
	ObjectID      string        `json:"object_id,omitempty"`
	URL           string        `json:"url,omitempty"`
}

// ActivityProvenance describes the semantic source of an activity record.
type ActivityProvenance struct {
	Type           ProvenanceType `json:"type"`
	Harness        string         `json:"harness,omitempty"`
	HarnessVersion string         `json:"harness_version,omitempty"`
	Model          string         `json:"model,omitempty"`
}

// ActivityProjection is the compact search and report representation.
type ActivityProjection struct {
	ID              EventID      `json:"id"`
	Type            ActivityType `json:"type"`
	OccurredAt      time.Time    `json:"occurred_at"`
	RecordedAt      time.Time    `json:"recorded_at"`
	ScopeName       string       `json:"scope_name,omitempty"`
	Summary         string       `json:"summary"`
	DurationMinutes int          `json:"duration_minutes,omitempty"`
	Identifier      string       `json:"identifier,omitempty"`
}

// Validate checks the complete canonical activity contract.
func (activity Activity) Validate() error {
	if err := activity.validateCommon(); err != nil {
		return err
	}

	if err := activity.validateProperties(); err != nil {
		return err
	}

	if err := validateActivityReferences(activity.References); err != nil {
		return err
	}

	if activity.Provenance != nil {
		if err := activity.Provenance.Validate(); err != nil {
			return err
		}
	}

	return activity.validateCorrection()
}

func (activity Activity) validateCommon() error {
	if err := activity.ID.Validate(); err != nil {
		return fmt.Errorf("validate activity ID: %w", err)
	}

	idMilliseconds, err := activity.ID.timestampMilliseconds()
	if err != nil {
		return fmt.Errorf("validate activity ID timestamp: %w", err)
	}

	if err := validateUTCTime("occurred_at", activity.OccurredAt); err != nil {
		return err
	}

	if err := validateUTCTime("recorded_at", activity.RecordedAt); err != nil {
		return err
	}

	if activity.RecordedAt.UnixMilli() != idMilliseconds {
		return fmt.Errorf("validate activity: ID timestamp must equal recorded_at at millisecond precision")
	}

	if activity.OccurredAt.After(activity.RecordedAt) {
		return fmt.Errorf("validate activity: occurred_at must not follow recorded_at")
	}

	if err := validateSignificanceText("summary", activity.Summary, true); err != nil {
		return err
	}

	if activity.Scope != nil {
		return activity.Scope.Validate()
	}

	return nil
}

func (activity Activity) validateProperties() error {
	switch activity.Type {
	case ActivityTypeCommit:
		return activity.validateCommit()
	case ActivityTypePullRequest:
		return activity.validatePullRequest()
	case ActivityTypeWorklog:
		return activity.validateWorklog()
	case ActivityTypeNote:
		if activity.Properties != nil {
			return fmt.Errorf("validate note: properties must be absent")
		}

		return nil
	default:
		return fmt.Errorf("validate activity: unsupported type %q", activity.Type)
	}
}

func (activity Activity) validateCommit() error {
	if activity.Scope == nil || activity.Scope.Type != ScopeTypeRepository {
		return fmt.Errorf("validate commit: repository scope is required")
	}

	properties := activity.Properties
	if properties == nil {
		return fmt.Errorf("validate commit: properties are required")
	}

	if !validObjectID(properties.ObjectID) {
		return fmt.Errorf("validate commit: object_id must be a full lowercase object ID")
	}

	if err := validateIdentifier("commit subject", properties.Subject, true); err != nil {
		return err
	}

	if properties.AuthoredAt != nil {
		if err := validateUTCTime("commit authored_at", *properties.AuthoredAt); err != nil {
			return err
		}
	}

	if properties.hasPullRequestFields() || properties.DurationMinutes != 0 {
		return fmt.Errorf("validate commit: properties contain fields for another activity type")
	}

	return nil
}

func (activity Activity) validatePullRequest() error {
	if activity.Scope == nil || activity.Scope.Type != ScopeTypeRepository {
		return fmt.Errorf("validate pull request: repository scope is required")
	}

	properties := activity.Properties
	if properties == nil {
		return fmt.Errorf("validate pull request: properties are required")
	}

	if err := validatePullRequestProperties(*properties); err != nil {
		return err
	}

	wantIdentity := properties.Forge + "/" + properties.Repository
	if activity.Scope.Name != properties.Repository || activity.Scope.Identity != wantIdentity {
		return fmt.Errorf("validate pull request: repository scope does not match properties")
	}

	if properties.hasCommitFields() || properties.DurationMinutes != 0 {
		return fmt.Errorf("validate pull request: properties contain fields for another activity type")
	}

	return nil
}

func validatePullRequestProperties(properties ActivityProperties) error {
	if err := validateHTTPURL("pull request forge", properties.Forge, false); err != nil {
		return err
	}

	if err := validateRepositoryName(properties.Repository); err != nil {
		return err
	}

	if properties.Number < 1 {
		return fmt.Errorf("validate pull request: number must be positive")
	}

	if !properties.Action.Valid() {
		return fmt.Errorf("validate pull request: unsupported action %q", properties.Action)
	}

	if properties.URL == "" {
		return nil
	}

	return validateHTTPURL("pull request URL", properties.URL, true)
}

func (activity Activity) validateWorklog() error {
	properties := activity.Properties
	if properties == nil {
		return fmt.Errorf("validate worklog: properties are required")
	}

	if properties.DurationMinutes < 1 || properties.DurationMinutes > maxWorklogMinutes {
		return fmt.Errorf("validate worklog: duration_minutes must be between 1 and %d", maxWorklogMinutes)
	}

	if properties.hasCommitFields() || properties.hasPullRequestFields() {
		return fmt.Errorf("validate worklog: properties contain fields for another activity type")
	}

	return nil
}

func (activity Activity) validateCorrection() error {
	if activity.Supersedes == "" {
		if activity.CorrectionReason != "" {
			return fmt.Errorf("validate activity: correction_reason requires supersedes")
		}

		return nil
	}

	if err := activity.Supersedes.Validate(); err != nil {
		return fmt.Errorf("validate supersedes: %w", err)
	}

	if activity.Supersedes == activity.ID {
		return fmt.Errorf("validate activity: event cannot supersede itself")
	}

	return validateSignificanceText("correction_reason", activity.CorrectionReason, true)
}

// Validate checks one stable activity scope.
func (scope ActivityScope) Validate() error {
	if !scope.Type.Valid() {
		return fmt.Errorf("validate scope: unsupported type %q", scope.Type)
	}

	if err := validateIdentifier("scope name", scope.Name, true); err != nil {
		return err
	}

	return validateIdentifier("scope identity", scope.Identity, true)
}

// Validate checks one optional activity reference.
func (reference ActivityReference) Validate() error {
	switch reference.Type {
	case ReferenceTypeGitCommit:
		return reference.validateGitCommit()
	case ReferenceTypePullRequest, ReferenceTypeURL:
		if reference.ScopeIdentity != "" || reference.ObjectID != "" {
			return fmt.Errorf("validate reference: URL variant contains Git fields")
		}

		return validateHTTPURL("reference URL", reference.URL, true)
	default:
		return fmt.Errorf("validate reference: unsupported type %q", reference.Type)
	}
}

func (reference ActivityReference) validateGitCommit() error {
	if err := validateIdentifier("reference scope identity", reference.ScopeIdentity, true); err != nil {
		return err
	}

	if !validObjectID(reference.ObjectID) {
		return fmt.Errorf("validate reference: object_id must be a full lowercase object ID")
	}

	if reference.URL != "" {
		return fmt.Errorf("validate reference: Git variant contains URL fields")
	}

	return nil
}

// Validate checks optional activity provenance.
func (provenance ActivityProvenance) Validate() error {
	if !provenance.Type.Valid() {
		return fmt.Errorf("validate provenance: unsupported type %q", provenance.Type)
	}

	if provenance.Type == ProvenanceTypeAgent {
		if err := validateIdentifier("provenance harness", provenance.Harness, true); err != nil {
			return err
		}
	} else if provenance.Harness != "" || provenance.HarnessVersion != "" || provenance.Model != "" {
		return fmt.Errorf("validate provenance: agent fields require agent type")
	}

	if err := validateIdentifier("provenance harness version", provenance.HarnessVersion, false); err != nil {
		return err
	}

	return validateIdentifier("provenance model", provenance.Model, false)
}

// Valid reports whether the scope type is canonical.
func (scopeType ScopeType) Valid() bool {
	switch scopeType {
	case ScopeTypeRepository, ScopeTypeProject, ScopeTypeSystem, ScopeTypeClient, ScopeTypeOther:
		return true
	default:
		return false
	}
}

// Valid reports whether the action is recordable.
func (action PullRequestAction) Valid() bool {
	switch action {
	case PullRequestActionOpened, PullRequestActionReviewed, PullRequestActionMerged, PullRequestActionClosed:
		return true
	default:
		return false
	}
}

// Valid reports whether the provenance type is canonical.
func (provenanceType ProvenanceType) Valid() bool {
	switch provenanceType {
	case ProvenanceTypeHuman, ProvenanceTypeAgent, ProvenanceTypeIntegration:
		return true
	default:
		return false
	}
}

func validateActivityReferences(references []ActivityReference) error {
	if len(references) > maxActivityReferences {
		return fmt.Errorf("validate activity: references exceed %d items", maxActivityReferences)
	}

	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if err := reference.Validate(); err != nil {
			return err
		}

		key := fmt.Sprintf("%s\x00%s\x00%s\x00%s", reference.Type, reference.ScopeIdentity, reference.ObjectID, reference.URL)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("validate activity: duplicate reference")
		}

		seen[key] = struct{}{}
	}

	return nil
}

func (properties ActivityProperties) hasCommitFields() bool {
	return properties.ObjectID != "" || properties.Subject != "" || properties.AuthoredAt != nil
}

func (properties ActivityProperties) hasPullRequestFields() bool {
	return properties.Forge != "" || properties.Repository != "" || properties.Number != 0 || properties.Action != "" || properties.URL != ""
}

func validateRepositoryName(value string) error {
	if err := validateIdentifier("pull request repository", value, true); err != nil {
		return err
	}

	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("validate pull request: repository must use owner/name")
	}

	return nil
}

func validateHTTPURL(name, value string, allowPath bool) error {
	if err := validateIdentifier(name, value, true); err != nil {
		return err
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("validate activity: parse %s: %w", name, err)
	}

	if err := validateHTTPURLAuthority(name, parsed); err != nil {
		return err
	}

	if !allowPath && urlHasResourceParts(parsed) {
		return fmt.Errorf("validate activity: %s must be a base URL", name)
	}

	return nil
}

func validateHTTPURLAuthority(name string, parsed *url.URL) error {
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("validate activity: %s must use HTTP or HTTPS", name)
	}

	if parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("validate activity: %s must have a host and no user information", name)
	}

	return nil
}

func urlHasResourceParts(parsed *url.URL) bool {
	return parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != ""
}
