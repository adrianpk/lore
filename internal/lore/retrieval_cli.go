package lore

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type searchOptions struct {
	query Query
	json  bool
}

type showOptions struct {
	params ShowParams
	json   bool
}

type correctOptions struct {
	params CorrectParams
	json   bool
}

type searchFlagValues struct {
	eventTypes   string
	occurredFrom string
	occurredTo   string
	recordedFrom string
	recordedTo   string
}

type optionalString struct {
	target **string
}

type optionalInt struct {
	target **int
}

func (app *App) runSearch(ctx context.Context, arguments []string) error {
	options, err := parseSearchOptions(arguments, app.stderr)
	if err != nil {
		return err
	}

	var result QueryResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "search", options.query, &result); err != nil {
		return err
	}

	if options.json {
		return writeJSON(app.stdout, result)
	}

	for _, event := range result.Events {
		if err := writeProjectionLine(app.stdout, event); err != nil {
			return err
		}
	}

	if result.Truncated {
		_, err = fmt.Fprintf(app.stdout, "truncated after %d events\n", result.ReturnedCount)
	}

	return err
}

func (app *App) runShow(ctx context.Context, arguments []string) error {
	options, err := parseShowOptions(arguments, app.stderr)
	if err != nil {
		return err
	}

	var result ShowResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "show", options.params, &result); err != nil {
		return err
	}

	if options.json {
		return writeJSON(app.stdout, result)
	}

	return writeEventLine(app.stdout, result.Event)
}

func (app *App) runCorrect(ctx context.Context, arguments []string) error {
	options, err := parseCorrectOptions(arguments, app.stderr)
	if err != nil {
		return err
	}

	var result CorrectResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "correct", options.params, &result); err != nil {
		return err
	}

	if options.json {
		return writeJSON(app.stdout, result)
	}

	_, err = fmt.Fprintf(
		app.stdout,
		"corrected %s %s %s %s\n",
		result.Event.ID,
		result.Event.Type,
		activityScopeName(result.Event),
		activityIdentifier(result.Event),
	)

	return err
}

func parseSearchOptions(arguments []string, errorOutput io.Writer) (searchOptions, error) {
	flags := flag.NewFlagSet("search", flag.ContinueOnError)
	flags.SetOutput(errorOutput)

	var (
		options searchOptions
		values  searchFlagValues
	)

	bindSearchFlags(flags, &options, &values)

	if err := flags.Parse(arguments); err != nil {
		return searchOptions{}, err
	}

	if len(flags.Args()) != 0 {
		return searchOptions{}, fmt.Errorf("usage: lore search [flags]")
	}

	if err := applySearchFlagValues(&options.query, values); err != nil {
		return searchOptions{}, err
	}

	return options, nil
}

func bindSearchFlags(flags *flag.FlagSet, options *searchOptions, values *searchFlagValues) {
	flags.StringVar(&options.query.Text, "query", "", "text query")
	flags.StringVar(&options.query.Scope, "scope", "", "scope name or identity")
	flags.StringVar(&options.query.Commit, "commit", "", "full or abbreviated commit")
	flags.StringVar(&options.query.PullRequest, "pull-request", "", "pull-request identity")
	flags.StringVar((*string)(&options.query.Source), "source", "", "human, agent, or integration provenance")
	flags.StringVar(&options.query.Harness, "harness", "", "execution harness")
	flags.StringVar(&options.query.Model, "model", "", "execution model")
	flags.StringVar(&values.eventTypes, "type", "", "comma-separated event types")
	flags.StringVar(&values.occurredFrom, "occurred-from", "", "inclusive RFC3339 occurrence time")
	flags.StringVar(&values.occurredTo, "occurred-to", "", "exclusive RFC3339 occurrence time")
	flags.StringVar(&values.recordedFrom, "recorded-from", "", "inclusive RFC3339 recording time")
	flags.StringVar(&values.recordedTo, "recorded-to", "", "exclusive RFC3339 recording time")
	flags.IntVar(&options.query.DurationMin, "duration-min", 0, "minimum worklog duration")
	flags.IntVar(&options.query.DurationMax, "duration-max", 0, "maximum worklog duration")
	flags.IntVar(&options.query.Limit, "limit", 0, "maximum returned events")
	flags.Var(&options.query.Order, "order", "ascending or descending")
	flags.BoolVar(&options.json, "json", false, "write JSON output")
}

func applySearchFlagValues(query *Query, values searchFlagValues) error {
	query.Types = parseEventTypes(values.eventTypes)

	times := []struct {
		name   string
		value  string
		target **time.Time
	}{
		{name: "recorded-from", value: values.recordedFrom, target: &query.RecordedFrom},
		{name: "recorded-to", value: values.recordedTo, target: &query.RecordedTo},
		{name: "occurred-from", value: values.occurredFrom, target: &query.OccurredFrom},
		{name: "occurred-to", value: values.occurredTo, target: &query.OccurredTo},
	}

	for _, input := range times {
		parsed, err := parseOptionalTime(input.name, input.value)
		if err != nil {
			return err
		}

		*input.target = parsed
	}

	return nil
}

func parseShowOptions(arguments []string, errorOutput io.Writer) (showOptions, error) {
	flags := flag.NewFlagSet("show", flag.ContinueOnError)
	flags.SetOutput(errorOutput)

	var options showOptions
	flags.StringVar((*string)(&options.params.ID), "id", "", "event ID")
	flags.BoolVar(&options.params.Raw, "raw", false, "return the exact stored event")
	flags.BoolVar(&options.json, "json", false, "write JSON output")

	if err := flags.Parse(arguments); err != nil {
		return showOptions{}, err
	}

	if len(flags.Args()) != 0 || options.params.ID == "" {
		return showOptions{}, fmt.Errorf("usage: lore show --id <event-id> [--raw] [--json]")
	}

	return options, nil
}

func parseCorrectOptions(arguments []string, errorOutput io.Writer) (correctOptions, error) {
	flags := flag.NewFlagSet("correct", flag.ContinueOnError)
	flags.SetOutput(errorOutput)

	var options correctOptions

	var occurredAt string

	summary := optionalString{target: &options.params.ReplacementSummary}
	duration := optionalInt{target: &options.params.ReplacementDurationMinutes}

	flags.StringVar(&occurredAt, "at", "", "replacement RFC3339 occurrence time")
	flags.Var(&duration, "duration", "replacement worklog duration in minutes")
	flags.Var(&summary, "summary", "replacement summary")
	flags.StringVar((*string)(&options.params.TargetID), "target", "", "current effective event ID")
	flags.StringVar(&options.params.Reason, "reason", "", "correction reason")
	flags.BoolVar(&options.json, "json", false, "write JSON output")

	if err := flags.Parse(arguments); err != nil {
		return correctOptions{}, err
	}

	parsedOccurredAt, err := parseOptionalTime("at", occurredAt)
	if err != nil {
		return correctOptions{}, err
	}

	options.params.ReplacementOccurredAt = parsedOccurredAt

	if len(flags.Args()) != 0 || options.params.TargetID == "" || strings.TrimSpace(options.params.Reason) == "" {
		return correctOptions{}, fmt.Errorf(
			"usage: lore correct --target <event-id> --reason <reason> [--at <time>] [--summary <summary>] [--duration <minutes>] [--json]",
		)
	}

	return options, nil
}

func (value *optionalString) String() string {
	if value == nil || value.target == nil || *value.target == nil {
		return ""
	}

	return **value.target
}

func (value *optionalString) Set(input string) error {
	*value.target = &input

	return nil
}

func (value *optionalInt) String() string {
	if value == nil || value.target == nil || *value.target == nil {
		return ""
	}

	return strconv.Itoa(**value.target)
}

func (value *optionalInt) Set(input string) error {
	parsed, err := strconv.Atoi(input)
	if err != nil {
		return fmt.Errorf("parse integer: %w", err)
	}

	*value.target = &parsed

	return nil
}

// Set parses a CLI order value. Query validation rejects unsupported values.
func (order *QueryOrder) Set(value string) error {
	*order = QueryOrder(value)

	return nil
}

func (order *QueryOrder) String() string {
	return string(*order)
}

func parseEventTypes(value string) []EventType {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	eventTypes := make([]EventType, 0, len(parts))

	for _, part := range parts {
		eventTypes = append(eventTypes, EventType(strings.TrimSpace(part)))
	}

	return eventTypes
}

func parseOptionalTime(name, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("parse --%s: %w", name, err)
	}

	return &parsed, nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)

	return encoder.Encode(value)
}

func writeEventLine(writer io.Writer, event Event) error {
	_, err := fmt.Fprintf(
		writer,
		"%s %s %s %s %s %q\n",
		event.OccurredAt.Format(time.RFC3339),
		event.ID,
		event.Type,
		activityScopeName(event),
		activityIdentifier(event),
		event.Summary,
	)

	return err
}

func writeProjectionLine(writer io.Writer, event ActivityProjection) error {
	parts := []string{event.OccurredAt.Format(time.RFC3339), string(event.ID), string(event.Type)}
	if event.ScopeName != "" {
		parts = append(parts, event.ScopeName)
	}

	if event.Identifier != "" {
		parts = append(parts, event.Identifier)
	}

	_, err := fmt.Fprintf(writer, "%s %q\n", strings.Join(parts, " "), event.Summary)

	return err
}
