package lore

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"
)

type reportOptions struct {
	params ReportParams
	json   bool
	types  string
}

func (app *App) runReport(ctx context.Context, arguments []string) error {
	options, err := parseReportOptions(arguments, app.stderr)
	if err != nil {
		return err
	}

	var result ReportResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "report", options.params, &result); err != nil {
		return err
	}

	if options.json {
		return writeJSON(app.stdout, result)
	}

	return writeHumanReport(app.stdout, result)
}

func parseReportOptions(arguments []string, errorOutput io.Writer) (reportOptions, error) {
	flags := flag.NewFlagSet("report", flag.ContinueOnError)
	flags.SetOutput(errorOutput)

	var options reportOptions
	flags.StringVar(&options.params.From, "from", "", "inclusive local date")
	flags.StringVar(&options.params.To, "to", "", "exclusive local date")
	flags.StringVar(&options.params.Timezone, "timezone", "", "IANA timezone name")
	flags.StringVar(&options.types, "type", "", "comma-separated activity types")
	flags.IntVar(&options.params.Limit, "limit", 0, "maximum returned events")
	flags.BoolVar(&options.json, "json", false, "write JSON output")

	if err := flags.Parse(arguments); err != nil {
		return reportOptions{}, err
	}

	if len(flags.Args()) != 0 || options.params.From == "" || options.params.To == "" {
		return reportOptions{}, fmt.Errorf(
			"usage: lore report --from <date> --to <date> [--timezone <name>] [--type <types>] [--limit <count>] [--json]",
		)
	}

	options.params.Types = parseEventTypes(options.types)

	return options, nil
}

func writeHumanReport(writer io.Writer, result ReportResult) error {
	location, err := reportLocation(result.Interval.Timezone)
	if err != nil {
		return err
	}

	for _, day := range result.Days {
		if err := writeReportDay(writer, day, location); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(writer, "total: %d events, %d minutes\n", result.EventCount, result.WorklogMinutes); err != nil {
		return err
	}

	if result.Truncated {
		_, err = fmt.Fprintf(writer, "truncated after %d events\n", result.ReturnedCount)
	}

	return err
}

func writeReportDay(writer io.Writer, day ReportDay, location *time.Location) error {
	if _, err := fmt.Fprintln(writer, day.Date); err != nil {
		return err
	}

	for _, scope := range day.Scopes {
		if err := writeReportScope(writer, scope, location); err != nil {
			return err
		}
	}

	return nil
}

func writeReportScope(writer io.Writer, scope ReportScope, location *time.Location) error {
	if _, err := fmt.Fprintf(writer, "  %s (%d events, %d minutes)\n", scope.Name, scope.EventCount, scope.WorklogMinutes); err != nil {
		return err
	}

	for _, event := range scope.Events {
		if err := writeReportEvent(writer, event, location); err != nil {
			return err
		}
	}

	return nil
}

func writeReportEvent(writer io.Writer, event ActivityProjection, location *time.Location) error {
	_, err := fmt.Fprintf(
		writer,
		"    %s %s %s %s %q\n",
		event.OccurredAt.In(location).Format("15:04:05"),
		event.ID,
		event.Type,
		event.Identifier,
		event.Summary,
	)

	return err
}
