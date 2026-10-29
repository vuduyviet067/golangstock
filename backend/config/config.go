package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL string
	HTTPPort    string
}

func Load() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf(
			"DATABASE_URL environment variable is required",
		)
	}

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	return Config{
		DatabaseURL: databaseURL,
		HTTPPort:    port,
	}, nil
}
