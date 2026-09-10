package lore

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const recordUsage = "usage: lore record <commit|pull-request|worklog|note> [flags] [summary]"

type recordOptions struct {
	activityType   ActivityType
	summary        string
	occurredAt     time.Time
	scope          *ActivityScope
	references     []ActivityReference
	provenance     *ActivityProvenance
	forge          string
	repository     string
	number         int
	action         PullRequestAction
	pullRequestURL string
	duration       int
}

type commonRecordFlags struct {
	at                    string
	scopeType             string
	scopeName             string
	scopeIdentity         string
	referenceURLs         stringListFlag
	referenceCommits      stringListFlag
	referencePullRequests stringListFlag
	source                string
	harness               string
	harnessVersion        string
	model                 string
}

type stringListFlag []string

func (app *App) runRecord(ctx context.Context, arguments []string) error {
	options, err := parseRecordOptions(arguments, app.stderr)
	if err != nil {
		return err
	}

	params, err := app.recordParams(ctx, options)
	if err != nil {
		return err
	}

	var result RecordResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "record", params, &result); err != nil {
		return err
	}

	return writeRecordReceipt(app.stdout, result)
}

func (app *App) recordParams(ctx context.Context, options recordOptions) (RecordParams, error) {
	if options.activityType != ActivityTypeCommit {
		return options.recordParams(), nil
	}

	directory, err := app.workingDirectory()
	if err != nil {
		return RecordParams{}, fmt.Errorf("resolve current directory: %w", err)
	}

	evidence, err := app.inspector.Inspect(ctx, directory)
	if err != nil {
		return RecordParams{}, err
	}

	return commitRecordParams(evidence, options), nil
}

func parseRecordOptions(arguments []string, errorOutput io.Writer) (recordOptions, error) {
	if len(arguments) == 0 {
		return recordOptions{}, errors.New(recordUsage)
	}

	activityType, err := parseRecordType(arguments[0])
	if err != nil {
		return recordOptions{}, err
	}

	flags := flag.NewFlagSet("record "+arguments[0], flag.ContinueOnError)
	flags.SetOutput(errorOutput)

	options := recordOptions{activityType: activityType}
	common := commonRecordFlags{}
	bindCommonRecordFlags(flags, &common)
	bindTypedRecordFlags(flags, &options)

	if err := flags.Parse(arguments[1:]); err != nil {
		return recordOptions{}, err
	}

	if err := applyRecordSummary(flags.Args(), &options); err != nil {
		return recordOptions{}, err
	}

	if err := applyCommonRecordFlags(common, &options); err != nil {
		return recordOptions{}, err
	}

	if err := normalizeTypedRecordOptions(&options); err != nil {
		return recordOptions{}, err
	}

	return options, nil
}

func parseRecordType(value string) (ActivityType, error) {
	switch value {
	case "commit":
		return ActivityTypeCommit, nil
	case "pull-request":
		return ActivityTypePullRequest, nil
	case "worklog":
		return ActivityTypeWorklog, nil
	case "note":
		return ActivityTypeNote, nil
	default:
		return "", fmt.Errorf("%s: unsupported type %q", recordUsage, value)
	}
}

func bindCommonRecordFlags(flags *flag.FlagSet, values *commonRecordFlags) {
	flags.StringVar(&values.at, "at", "", "RFC3339 occurrence time")
	flags.StringVar(&values.scopeType, "scope-type", "", "scope type")
	flags.StringVar(&values.scopeName, "scope-name", "", "scope display name")
	flags.StringVar(&values.scopeIdentity, "scope-identity", "", "stable scope identity")
	flags.Var(&values.referenceURLs, "reference-url", "related URL")
	flags.Var(&values.referenceCommits, "reference-commit", "related scope identity and commit")
	flags.Var(&values.referencePullRequests, "reference-pull-request", "related pull-request URL")
	flags.StringVar(&values.source, "source", "", "human, agent, or integration")
	flags.StringVar(&values.harness, "harness", "", "agent harness name")
	flags.StringVar(&values.harnessVersion, "harness-version", "", "agent harness version")
	flags.StringVar(&values.model, "model", "", "provider-qualified model identifier")
}

func bindTypedRecordFlags(flags *flag.FlagSet, options *recordOptions) {
	switch options.activityType {
	case ActivityTypePullRequest:
		flags.StringVar(&options.forge, "forge", "", "forge base URL")
		flags.StringVar(&options.repository, "repository", "", "owner/name repository")
		flags.IntVar(&options.number, "number", 0, "pull-request number")
		flags.Var(&options.action, "action", "opened, reviewed, merged, or closed")
		flags.StringVar(&options.pullRequestURL, "url", "", "pull-request URL")
	case ActivityTypeWorklog:
		flags.IntVar(&options.duration, "duration", 0, "explicit duration in minutes")
	}
}

func applyRecordSummary(arguments []string, options *recordOptions) error {
	if len(arguments) > 1 || (options.activityType != ActivityTypeCommit && len(arguments) != 1) {
		return errors.New(recordUsage)
	}

	if len(arguments) == 1 {
		options.summary = arguments[0]
	}

	return nil
}

func applyCommonRecordFlags(values commonRecordFlags, options *recordOptions) error {
	occurredAt, err := parseRecordTime(values.at)
	if err != nil {
		return err
	}

	scope, err := parseRecordScope(values)
	if err != nil {
		return err
	}

	references, err := parseRecordReferences(values)
	if err != nil {
		return err
	}

	provenance, err := parseRecordProvenance(values)
	if err != nil {
		return err
	}

	options.occurredAt = occurredAt
	options.scope = scope
	options.references = references
	options.provenance = provenance

	return nil
}

func parseRecordTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse --at: %w", err)
	}

	return parsed.UTC(), nil
}

func parseRecordScope(values commonRecordFlags) (*ActivityScope, error) {
	present := 0

	for _, value := range []string{values.scopeType, values.scopeName, values.scopeIdentity} {
		if value != "" {
			present++
		}
	}

	if present == 0 {
		return nil, nil
	}

	if present != 3 {
		return nil, fmt.Errorf("scope type, name, and identity must be supplied together")
	}

	scope := &ActivityScope{Type: ScopeType(values.scopeType), Name: values.scopeName, Identity: values.scopeIdentity}
	if err := scope.Validate(); err != nil {
		return nil, err
	}

	return scope, nil
}

func parseRecordReferences(values commonRecordFlags) ([]ActivityReference, error) {
	count := len(values.referenceURLs) + len(values.referenceCommits) + len(values.referencePullRequests)
	if count == 0 {
		return nil, nil
	}

	references := make([]ActivityReference, 0, count)
	for _, value := range values.referenceURLs {
		references = append(references, ActivityReference{Type: ReferenceTypeURL, URL: value})
	}

	for _, value := range values.referenceCommits {
		reference, err := parseCommitReference(value)
		if err != nil {
			return nil, err
		}

		references = append(references, reference)
	}

	for _, value := range values.referencePullRequests {
		references = append(references, ActivityReference{Type: ReferenceTypePullRequest, URL: value})
	}

	if err := validateActivityReferences(references); err != nil {
		return nil, err
	}

	return references, nil
}

func parseCommitReference(value string) (ActivityReference, error) {
	separator := strings.LastIndexByte(value, '@')
	if separator <= 0 || separator == len(value)-1 {
		return ActivityReference{}, fmt.Errorf("parse --reference-commit: expected <scope-identity>@<object-id>")
	}

	reference := ActivityReference{
		Type:          ReferenceTypeGitCommit,
		ScopeIdentity: value[:separator],
		ObjectID:      value[separator+1:],
	}
	if err := reference.Validate(); err != nil {
		return ActivityReference{}, err
	}

	return reference, nil
}

func parseRecordProvenance(values commonRecordFlags) (*ActivityProvenance, error) {
	if values.source == "" && values.harness == "" && values.harnessVersion == "" && values.model == "" {
		return nil, nil
	}

	source := ProvenanceType(values.source)
	if source == "" && values.harness != "" {
		source = ProvenanceTypeAgent
	}

	provenance := &ActivityProvenance{
		Type:           source,
		Harness:        values.harness,
		HarnessVersion: values.harnessVersion,
		Model:          values.model,
	}
	if err := provenance.Validate(); err != nil {
		return nil, err
	}

	return provenance, nil
}

func normalizeTypedRecordOptions(options *recordOptions) error {
	if options.summary != "" {
		if err := validateSignificanceText("summary", options.summary, true); err != nil {
			return err
		}
	}

	switch options.activityType {
	case ActivityTypeCommit:
		return normalizeCommitRecordOptions(*options)
	case ActivityTypePullRequest:
		return normalizePullRequestRecordOptions(options)
	case ActivityTypeWorklog:
		return normalizeWorklogRecordOptions(*options)
	}

	return nil
}

func normalizeCommitRecordOptions(options recordOptions) error {
	if !options.occurredAt.IsZero() {
		return fmt.Errorf("record commit does not accept --at")
	}

	if options.scope != nil {
		return fmt.Errorf("record commit derives its repository scope")
	}

	return nil
}

func normalizePullRequestRecordOptions(options *recordOptions) error {
	if options.scope != nil {
		return fmt.Errorf("record pull-request derives its repository scope")
	}

	forge, err := normalizeForgeURL(options.forge)
	if err != nil {
		return err
	}

	options.forge = forge
	if err := validateRepositoryName(options.repository); err != nil {
		return err
	}

	if options.number < 1 {
		return fmt.Errorf("record pull-request requires a positive --number")
	}

	if !options.action.Valid() {
		return fmt.Errorf("record pull-request requires --action")
	}

	if options.pullRequestURL == "" {
		return nil
	}

	return validateHTTPURL("pull request URL", options.pullRequestURL, true)
}

func normalizeWorklogRecordOptions(options recordOptions) error {
	if options.duration < 1 || options.duration > maxWorklogMinutes {
		return fmt.Errorf("record worklog requires --duration between 1 and %d", maxWorklogMinutes)
	}

	return nil
}

func normalizeForgeURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("normalize forge URL: %w", err)
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Path == "/" {
		parsed.Path = ""
	}

	if err := validateHTTPURL("pull request forge", parsed.String(), false); err != nil {
		return "", err
	}

	parsed.Host = canonicalHost(parsed.Hostname(), parsed.Port())

	return parsed.String(), nil
}

func (options recordOptions) recordParams() RecordParams {
	params := RecordParams{
		Type:       options.activityType,
		OccurredAt: options.occurredAt,
		Summary:    options.summary,
		Scope:      options.scope,
		References: options.references,
		Provenance: options.provenance,
	}

	switch options.activityType {
	case ActivityTypePullRequest:
		params.Scope = &ActivityScope{
			Type:     ScopeTypeRepository,
			Name:     options.repository,
			Identity: options.forge + "/" + options.repository,
		}
		params.Properties = &ActivityProperties{
			Forge:      options.forge,
			Repository: options.repository,
			Number:     options.number,
			Action:     options.action,
			URL:        options.pullRequestURL,
		}
	case ActivityTypeWorklog:
		params.Properties = &ActivityProperties{DurationMinutes: options.duration}
	}

	return params
}

func commitRecordParams(evidence RepositoryEvidence, options recordOptions) RecordParams {
	summary := options.summary
	if summary == "" {
		summary = evidence.Git.Subject
	}

	properties := &ActivityProperties{ObjectID: evidence.Git.Commit, Subject: evidence.Git.Subject}
	if !evidence.Git.AuthoredAt.Equal(evidence.Git.CommittedAt) {
		authoredAt := evidence.Git.AuthoredAt
		properties.AuthoredAt = &authoredAt
	}

	return RecordParams{
		Type:       ActivityTypeCommit,
		OccurredAt: evidence.Git.CommittedAt,
		Summary:    summary,
		Scope: &ActivityScope{
			Type:     ScopeTypeRepository,
			Name:     evidence.Repository.Name,
			Identity: evidence.Repository.Identity,
		},
		Properties: properties,
		References: options.references,
		Provenance: options.provenance,
	}
}

func writeRecordReceipt(writer io.Writer, result RecordResult) error {
	status := "recorded"
	if !result.Recorded {
		status = "already recorded"
	}

	parts := []string{status, string(result.ID), string(result.Type)}
	if result.ScopeName != "" {
		parts = append(parts, result.ScopeName)
	}

	if result.Identifier != "" {
		parts = append(parts, result.Identifier)
	}

	_, err := fmt.Fprintln(writer, strings.Join(parts, " "))

	return err
}

func (value *stringListFlag) Set(input string) error {
	*value = append(*value, input)

	return nil
}

func (value *stringListFlag) String() string {
	if value == nil {
		return ""
	}

	return strings.Join(*value, ",")
}

// Set parses a pull-request action flag.
func (action *PullRequestAction) Set(value string) error {
	*action = PullRequestAction(value)
	if !action.Valid() {
		return fmt.Errorf("unsupported pull-request action %q", value)
	}

	return nil
}

// String formats a pull-request action flag.
func (action *PullRequestAction) String() string {
	return string(*action)
}

func abbreviatedCommit(commit string) string {
	const abbreviationLength = 12

	if len(commit) <= abbreviationLength {
		return commit
	}

	return commit[:abbreviationLength]
}
