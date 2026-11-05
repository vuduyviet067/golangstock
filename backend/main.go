package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gostock/config"
	"gostock/internal/app"
)

func main() {
	if err := run(); err != nil {
		log.Printf("application error: %v", err)
		os.Exit(1)
	}
}

func run() error {

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	return app.Start(ctx, cfg)
}
