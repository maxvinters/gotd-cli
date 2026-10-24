package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/gotd/cli/internal/output"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		// Concise message by default; full chain + stack with TG_DEBUG for
		// troubleshooting. All diagnostics go to stderr.
		if os.Getenv("TG_DEBUG") != "" {
			fmt.Fprintln(os.Stderr, output.EscapeTerminalText(fmt.Sprintf("%+v", err)))
		} else {
			fmt.Fprintf(os.Stderr, "tg: %s\n", output.EscapeTerminalText(err.Error()))
		}
		os.Exit(1)
	}
}
