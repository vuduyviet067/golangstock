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
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	application, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			log.Printf("close application error: %v", closeErr)
		}
	}()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if err := application.Run(ctx); err != nil {
		return fmt.Errorf("run application: %w", err)
	}

	return nil
}
