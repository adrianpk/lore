// Command lore runs the local chronicle client and service.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"forge.adrianpk.com/adrianpk/lore/internal/lore"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := ""
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	config, err := lore.DefaultConfigForCommand(command)
	if err == nil {
		err = lore.NewApp(config, os.Stdout, os.Stderr).Run(ctx, os.Args[1:])
	}

	if err != nil {
		log.New(os.Stderr, "", 0).Println(err)
		os.Exit(1)
	}
}
