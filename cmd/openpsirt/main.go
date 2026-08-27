// Command openpsirt serves the openpsirt API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/bhouse-nexthop/openpsirt/internal/config"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/httpapi"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
	"github.com/bhouse-nexthop/openpsirt/internal/version"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "openpsirt: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr *os.File) error {
	fs := flag.NewFlagSet("openpsirt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the build and exit")
	dumpSpec := fs.Bool("openapi", false, "write the OpenAPI document to stdout and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		v := version.Get()
		_, err := fmt.Fprintf(stdout, "openpsirt %s (%s, built %s, %s)\n", v.Version, v.Commit, v.Date, v.Go)
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg, stderr)

	// Generating the document from the running registrations is what keeps it
	// from drifting away from the server. It needs no database.
	if *dumpSpec {
		_, api := httpapi.New(logger, nil)
		doc, err := api.OpenAPI().YAML()
		if err != nil {
			return fmt.Errorf("render OpenAPI document: %w", err)
		}
		_, err = stdout.Write(doc)
		return err
	}

	ctx := context.Background()
	if fs.NArg() > 0 && fs.Arg(0) == "migrate" {
		return runMigrate(ctx, cfg, logger, stdout, fs.Args()[1:])
	}

	db, err := openDatabase(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer closeQuietly(db, logger)

	// Migrating before serving means a request never arrives against a schema
	// the code does not expect.
	if cfg.AutoMigrate {
		if err := schema.Up(ctx, db, logger); err != nil {
			return err
		}
	} else {
		logger.Info("automatic migration is off; run \"openpsirt migrate up\" separately")
	}

	handler, _ := httpapi.New(logger, func(ctx context.Context) error {
		return db.PingContext(ctx)
	})
	return serve(cfg, logger, handler)
}

// closeQuietly closes the database at shutdown. A failure here changes nothing
// about the exit, but silence would hide a genuinely stuck connection.
func closeQuietly(db *database.DB, logger *slog.Logger) {
	if err := db.Close(); err != nil {
		logger.Warn("closing the database", "error", err)
	}
}

func openDatabase(ctx context.Context, cfg config.Config, logger *slog.Logger) (*database.DB, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("no database configured: set OPENPSIRT_DATABASE_URL")
	}
	target, err := database.ParseURL(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	db, err := database.Open(ctx, target)
	if err != nil {
		return nil, err
	}
	logger.Info("database connected",
		"engine", db.Server.Engine, "version", db.Server.Version, "url", target.Redacted)
	if !db.Server.Engine.IsProduction() {
		logger.Warn("this database is for development and testing only",
			"engine", db.Server.Engine)
	}
	return db, nil
}

// runMigrate applies or rolls back schema changes on their own, so an operator
// can run them under different credentials and at a time they choose.
func runMigrate(ctx context.Context, cfg config.Config, logger *slog.Logger, stdout *os.File, args []string) error {
	action := "up"
	if len(args) > 0 {
		action = args[0]
	}

	db, err := openDatabase(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer closeQuietly(db, logger)

	switch action {
	case "up":
		return schema.Up(ctx, db, logger)
	case "down":
		return schema.Down(ctx, db, logger)
	case "status":
		v, err := schema.Version(ctx, db)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "%s %s, schema version %d\n",
			db.Server.Engine, db.Server.Version, v)
		return err
	}
	return fmt.Errorf("unknown migrate action %q: want up, down or status", action)
}

func newLogger(cfg config.Config, w *os.File) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

func serve(cfg config.Config, logger *slog.Logger, handler http.Handler) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: cfg.Addr, Handler: handler}
	errs := make(chan error, 1)

	go func() {
		v := version.Get()
		logger.Info("listening", "addr", cfg.Addr, "version", v.Version, "commit", v.Commit)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}

	// Let in-flight requests finish rather than cutting them off.
	logger.Info("shutting down", "grace", cfg.ShutdownGrace)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("stopped")
	return nil
}
