package lore

import (
	"context"
	"flag"
	"fmt"
	"io"
)

type verifyOptions struct {
	full      bool
	json      bool
	scheduled bool
}

func (app *App) runVerify(ctx context.Context, arguments []string) error {
	options, err := parseVerifyOptions(arguments, app.stderr)
	if err != nil {
		return err
	}

	full := options.full

	fullMonth := ""
	if options.scheduled {
		full, fullMonth, err = scheduledVerificationPlan(app.config.VerificationStatePath, app.now())
		if err != nil {
			return err
		}
	}

	var result VerifyResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "verify", VerifyParams{Full: full}, &result); err != nil {
		return err
	}

	if options.scheduled && full {
		if err := recordScheduledFullVerification(app.config.VerificationStatePath, fullMonth); err != nil {
			return err
		}
	}

	if options.json {
		return writeJSON(app.stdout, result)
	}

	_, err = fmt.Fprintf(
		app.stdout,
		"verified %s journal: %d segments, %d events\n",
		result.Scope,
		result.SegmentCount,
		result.EventCount,
	)

	return err
}

func parseVerifyOptions(arguments []string, errorOutput io.Writer) (verifyOptions, error) {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	flags.SetOutput(errorOutput)

	var options verifyOptions
	flags.BoolVar(&options.full, "full", false, "verify every canonical segment")
	flags.BoolVar(&options.json, "json", false, "write JSON output")
	flags.BoolVar(&options.scheduled, "scheduled", false, "select the scheduled daily or monthly scope")

	if err := flags.Parse(arguments); err != nil {
		return verifyOptions{}, err
	}

	if len(flags.Args()) != 0 || (options.full && options.scheduled) {
		return verifyOptions{}, fmt.Errorf("usage: lore verify [--full | --scheduled] [--json]")
	}

	return options, nil
}
