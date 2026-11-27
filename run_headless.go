//go:build !tray

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// run waits for SIGINT/SIGTERM. Build with -tags tray for the tray icon.
func run(*app) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
