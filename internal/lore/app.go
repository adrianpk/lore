package lore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
)

// App owns process-level command dispatch and dependency assembly.
type App struct {
	config           Config
	stdin            io.Reader
	stdout           io.Writer
	stderr           io.Writer
	workingDirectory func() (string, error)
}

// NewApp constructs the executable application without performing IO.
func NewApp(config Config, stdout, stderr io.Writer) *App {
	return &App{
		config:           config,
		stdin:            os.Stdin,
		stdout:           stdout,
		stderr:           stderr,
		workingDirectory: os.Getwd,
	}
}

// Run executes one CLI command.
func (app *App) Run(ctx context.Context, arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("usage: lore <archive|invalidate|record|restore|revise|search|serve|show|verify>")
	}

	switch arguments[0] {
	case "archive", "invalidate", "restore":
		return app.runEntryLifecycle(ctx, arguments[0], arguments[1:])
	case "record":
		return app.runEntryRecord(ctx, arguments[1:])
	case "revise":
		return app.runEntryRevise(ctx, arguments[1:])
	case "search":
		return app.runEntrySearch(ctx, arguments[1:])
	case "serve":
		return app.runServe(ctx, arguments[1:])
	case "show":
		return app.runEntryShow(ctx, arguments[1:])
	case "verify":
		return app.runEntryVerify(ctx, arguments[1:])
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func (app *App) runServe(ctx context.Context, arguments []string) error {
	if len(arguments) != 0 {
		return fmt.Errorf("usage: lore serve")
	}

	journal := NewEntryJournal(app.config)
	if err := journal.Load(ctx); err != nil {
		return fmt.Errorf("load Lore journal: %w", err)
	}

	listener, err := ListenServiceSocket(app.config.SocketPath)
	if err != nil {
		return err
	}

	server := NewServer(NewService(journal))
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
