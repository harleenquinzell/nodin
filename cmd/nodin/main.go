package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/harleenquinzell/nodin/internal/cli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	root := cli.NewRootCmd(version)
	if err := root.ExecuteContext(ctx); err != nil {
		if errors.Is(err, cli.ErrConflicts) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
}
