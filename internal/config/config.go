// Package config loads runtime settings.
//
// Settings come from the environment. Every one has a working default, so an
// operator can start the binary with nothing set and get something sensible.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
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
	// ScannerPath is where the vulnerability scanner lives. Empty means
	// whatever the environment resolves.
	//
	// The scanner is a requirement of a deployment rather than an option:
	// without it there is nothing to triage, because the vulnerability data is
	// produced here rather than sent to us.
	ScannerPath string
	// BootstrapAdmins are granted administrator at every startup, not only the
	// first. Applying it every time makes it the way back in for an operator
	// who has locked themselves out: add yourself, restart. For software
	// somebody else runs, a way back in matters more than a tidy one-shot.
	//
	// It is a pre-authorization and not a bypass: being named grants the role,
	// it does not admit anybody who has not authenticated.
	BootstrapAdmins []string
	// TrustedHeader is the header a reverse proxy sets to say who somebody is,
	// and TrustedSources are the addresses it is honored from. Both are needed
	// for either to do anything.
	TrustedHeader  string
	TrustedSources []net.IPNet
	// BaseURL is the address people arrive on. Behind a proxy that is not what
	// this process thinks it is called, and a provider compares the callback
	// against what it was registered with, so it has to be stated.
	BaseURL string
	// PlainHTTP serves without TLS, which is what running this locally looks
	// like. It only loosens cookies, and it is named for what it is.
	PlainHTTP bool
	// SessionLifetime bounds a sign-in. Zero takes the built-in default.
	SessionLifetime time.Duration

	// OIDC is an OpenID Connect provider. Issuer being empty means none is
	// configured.
	OIDCName          string
	OIDCIssuer        string
	OIDCClientID      string
	OIDCClientSecret  string
	OIDCGroupsClaim   string
	OIDCUsernameClaim string

	// GitHub is OAuth 2.0 rather than OpenID Connect, so it is configured
	// separately. GitHubOrg being empty means teams are not read at all.
	GitHubClientID     string
	GitHubClientSecret string
	GitHubOrg          string

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
		Addr:            env("ADDR", ":8080"),
		BaseURL:         env("BASE_URL", ""),
		PlainHTTP:       env("PLAIN_HTTP", "") != "",
		SessionLifetime: duration("SESSION_LIFETIME", 0),

		OIDCName:          env("OIDC_NAME", "oidc"),
		OIDCIssuer:        env("OIDC_ISSUER", ""),
		OIDCClientID:      env("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:  env("OIDC_CLIENT_SECRET", ""),
		OIDCGroupsClaim:   env("OIDC_GROUPS_CLAIM", ""),
		OIDCUsernameClaim: env("OIDC_USERNAME_CLAIM", ""),

		GitHubClientID:     env("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: env("GITHUB_CLIENT_SECRET", ""),
		GitHubOrg:          env("GITHUB_ORG", ""),
		LogFormat:          env("LOG_FORMAT", "text"),
		ShutdownGrace:      duration("SHUTDOWN_GRACE", 15*time.Second),
		DatabaseURL:        env("DATABASE_URL", ""),
		ScannerPath:        env("SCANNER_PATH", ""),
		TrustedHeader:      env("TRUSTED_HEADER", ""),
		AutoMigrate:        env("AUTO_MIGRATE", "true") != "false",
		ReadTimeout:        5 * time.Minute,
		WriteTimeout:       5 * time.Minute,
		DBMaxOpen:          number("DB_MAX_OPEN", 25),
		DBMaxIdle:          number("DB_MAX_IDLE", 25),
		DBIdleTimeout:      duration("DB_IDLE_TIMEOUT", time.Minute),
		DBLifetime:         duration("DB_CONN_LIFETIME", 30*time.Minute),
	}

	c.BootstrapAdmins = access.Identities(env("BOOTSTRAP_ADMINS", ""))

	sources, err := access.ParseSources(env("TRUSTED_SOURCES", ""))
	if err != nil {
		return Config{}, fmt.Errorf("%sTRUSTED_SOURCES: %w", envPrefix, err)
	}
	c.TrustedSources = sources
	// A half-configuration is the dangerous state, so it stops the process
	// rather than being quietly ignored: a header named with nothing to trust
	// it from is either a mistake or the first half of one.
	if err := (access.Trust{Header: c.TrustedHeader, From: c.TrustedSources}).Configured(); err != nil {
		return Config{}, fmt.Errorf("%sTRUSTED_HEADER: %w", envPrefix, err)
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
