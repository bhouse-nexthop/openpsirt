// Package config loads runtime settings.
//
// Settings come from the environment. Every one has a working default, so an
// operator can start the binary with nothing set and get something sensible.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is everything the process needs to start.
type Config struct {
	// Addr is the host:port the HTTP server listens on.
	Addr string
	// LogLevel controls verbosity.
	LogLevel slog.Level
	// LogFormat is "text" or "json".
	LogFormat string
	// ShutdownGrace is how long in-flight requests get to finish.
	ShutdownGrace time.Duration
	// DatabaseURL says which database to use and how to reach it.
	DatabaseURL string
	// ReadTimeout and WriteTimeout bound a single request.
	//
	// Generous by web-server standards on purpose: scan files are large and
	// arrive over links we do not control. Too tight and a legitimate upload
	// is cut off mid-transfer.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	// Database pool settings. The one that matters is IdleTimeout: a
	// connection has to be closed by us before anything in the path closes it
	// behind our back.
	DBMaxOpen     int
	DBMaxIdle     int
	DBIdleTimeout time.Duration
	DBLifetime    time.Duration
	// AutoMigrate applies outstanding schema changes at startup.
	//
	// On by default: a self-hosted operator should not need a separate step,
	// and deploying the binary is then the whole upgrade. Turning it off suits
	// someone who would rather run migrations themselves, under different
	// credentials, at a time they choose.
	AutoMigrate bool
}

const envPrefix = "OPENPSIRT_"

// Load reads configuration from the environment.
func Load() (Config, error) {
	c := Config{
		Addr:          env("ADDR", ":8080"),
		LogFormat:     env("LOG_FORMAT", "text"),
		ShutdownGrace: duration("SHUTDOWN_GRACE", 15*time.Second),
		DatabaseURL:   env("DATABASE_URL", ""),
		AutoMigrate:   env("AUTO_MIGRATE", "true") != "false",
		ReadTimeout:   5 * time.Minute,
		WriteTimeout:  5 * time.Minute,
		DBMaxOpen:     number("DB_MAX_OPEN", 25),
		DBMaxIdle:     number("DB_MAX_IDLE", 25),
		DBIdleTimeout: duration("DB_IDLE_TIMEOUT", time.Minute),
		DBLifetime:    duration("DB_CONN_LIFETIME", 30*time.Minute),
	}

	if err := c.LogLevel.UnmarshalText([]byte(env("LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("%sLOG_LEVEL: %w", envPrefix, err)
	}
	switch c.LogFormat {
	case "text", "json":
	default:
		return Config{}, fmt.Errorf("%sLOG_FORMAT: want \"text\" or \"json\", got %q", envPrefix, c.LogFormat)
	}
	if strings.TrimSpace(c.Addr) == "" {
		return Config{}, fmt.Errorf("%sADDR: must not be empty", envPrefix)
	}
	return c, nil
}

// duration reads a duration setting, falling back when unset or unreadable.
func duration(key string, fallback time.Duration) time.Duration {
	raw, ok := os.LookupEnv(envPrefix + key)
	if !ok {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

// number reads a positive integer setting, falling back when unset or unusable.
func number(key string, fallback int) int {
	raw, ok := os.LookupEnv(envPrefix + key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(envPrefix + key); ok {
		return v
	}
	return fallback
}
