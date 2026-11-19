package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	// Embedded IANA time zone database, used only when the host has no
	// zoneinfo of its own: the dashboard's tz parameter (Asia/Ho_Chi_Minh from
	// the frontend) must load on any deployment host. Guarded by main_test.go.
	_ "time/tzdata"

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
