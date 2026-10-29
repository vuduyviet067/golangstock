package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL string
	HTTPPort    string
	SessionTTL  time.Duration
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
	sessionTTL := 24 * time.Hour

	if value := os.Getenv("SESSION_TTL_HOURS"); value != "" {
		hours, err := strconv.Atoi(value)
		if err != nil || hours <= 0 {
			return Config{}, fmt.Errorf(
				"SESSION_TTL_HOURS must be a positive integer",
			)
		}

		sessionTTL = time.Duration(hours) * time.Hour
	}
	return Config{
		DatabaseURL: databaseURL,
		HTTPPort:    port,
		SessionTTL:  sessionTTL,
	}, nil
}
