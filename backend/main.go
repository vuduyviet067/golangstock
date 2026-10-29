package main

import (
	"errors"
	"log"
	"net/http"

	"gostock/config"
	"gostock/internal/app"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if err := app.Run(cfg); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
