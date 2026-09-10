package lore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// App owns process-level command dispatch and dependency assembly.
type App struct {
	config           Config
	stdout           io.Writer
	stderr           io.Writer
	workingDirectory func() (string, error)
	inspector        *GitInspector
	now              func() time.Time
}

// NewApp constructs the executable application without performing IO.
func NewApp(config Config, stdout, stderr io.Writer) *App {
	return &App{
		config:           config,
		stdout:           stdout,
		stderr:           stderr,
		workingDirectory: os.Getwd,
		inspector:        NewGitInspector(),
		now:              time.Now,
	}
}

// Run executes one CLI command.
func (app *App) Run(ctx context.Context, arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("usage: lore <correct|migrate|record|report|search|serve|show|verify>")
	}

	if arguments[0] == "migrate" {
		return app.runMigrate(ctx, arguments[1:])
	}

	return app.runServiceCommand(ctx, arguments)
}

func (app *App) runServiceCommand(ctx context.Context, arguments []string) error {
	switch arguments[0] {
	case "correct":
		return app.runCorrect(ctx, arguments[1:])
	case "record":
		return app.runRecord(ctx, arguments[1:])
	case "report":
		return app.runReport(ctx, arguments[1:])
	case "search":
		return app.runSearch(ctx, arguments[1:])
	case "serve":
		return app.runServe(ctx, arguments[1:])
	case "show":
		return app.runShow(ctx, arguments[1:])
	case "verify":
		return app.runVerify(ctx, arguments[1:])
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func (app *App) runServe(ctx context.Context, arguments []string) error {
	if len(arguments) != 0 {
		return fmt.Errorf("usage: lore serve")
	}

	journal := NewJournal(app.config)
	if err := journal.Load(ctx); err != nil && !errors.Is(err, ErrJournalCorrupt) {
		return fmt.Errorf("load Lore journal: %w", err)
	}

	listener, err := ListenServiceSocket(app.config.SocketPath)
	if err != nil {
		return err
	}

	server := NewServer(NewService(NewRecorder(journal)))
	serveErr := server.Serve(ctx, listener)

	closeErr := listener.Close()
	if errors.Is(closeErr, net.ErrClosed) {
		closeErr = nil
	}

	if err := errors.Join(serveErr, closeErr); err != nil {
		return fmt.Errorf("serve Lore requests: %w", err)
	}

	return nil
}
