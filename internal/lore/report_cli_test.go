package lore

import (
	"bytes"
	"testing"
)

func TestParseReportOptions(t *testing.T) {
	t.Parallel()

	options, err := parseReportOptions([]string{
		"--from", "2026-08-01",
		"--to", "2026-08-08",
		"--timezone", "Europe/Warsaw",
		"--type", "worklog,note",
		"--limit", "100",
		"--json",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseReportOptions() error = %v", err)
	}

	valid := options.params.From == "2026-08-01" &&
		options.params.To == "2026-08-08" &&
		options.params.Timezone == "Europe/Warsaw" &&
		len(options.params.Types) == 2 &&
		options.params.Types[0] == ActivityTypeWorklog &&
		options.params.Types[1] == ActivityTypeNote &&
		options.params.Limit == 100 &&
		options.json
	if !valid {
		t.Fatalf("parseReportOptions() = %#v", options)
	}
}

func TestParseReportOptionsRequiresExplicitInterval(t *testing.T) {
	t.Parallel()

	if _, err := parseReportOptions([]string{"--from", "2026-08-01"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected missing to date error")
	}
}
