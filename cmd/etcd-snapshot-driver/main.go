package main

import (
	"context"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	if err := newRootCommand().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
