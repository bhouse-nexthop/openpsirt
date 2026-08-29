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
	"sync"
	"syscall"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/config"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/httpapi"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/queue"
	"github.com/bhouse-nexthop/openpsirt/internal/sbom"
	"github.com/bhouse-nexthop/openpsirt/internal/scanner"
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
		// Asking for help is not a failure.
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *showVersion {
		v := version.Get()
		_, err := fmt.Fprintf(stdout, "OpenPSIRT %s (%s, built %s, %s)\n", v.Version, v.Commit, v.Date, v.Go)
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
		_, api := httpapi.New(logger, nil, httpapi.Ingest{})
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

	// Readiness asks whether we can serve, which means the database answers
	// and answers promptly — not merely that the process is up.
	// Named administrators are granted at every start, which is what makes
	// this the way back in rather than a one-time setup step.
	if err := access.Bootstrap(ctx, access.NewStore(db.DB), cfg.BootstrapAdmins); err != nil {
		return err
	}
	if len(cfg.BootstrapAdmins) > 0 {
		logger.Info("administrators granted from configuration", "count", len(cfg.BootstrapAdmins))
	}

	work := queue.New(db, queue.DefaultOptions())
	handler, _ := httpapi.New(logger, db.Validate, httpapi.Ingest{
		DB: db, Queue: work,
		Access: access.NewResolver(access.NewStore(db.DB), access.Trust{
			Header: cfg.TrustedHeader, From: cfg.TrustedSources,
		}).WithLogger(logger),
	})

	// Every replica serves, reads and scans. Separate worker deployments would
	// be more things to run and more things to get wrong for an installation
	// this size, and the queue already stops two of them taking the same work.
	name := workerName()
	reader := ingest.NewReader(db, work, sbom.Limits{}, logger, name)
	runner := scanner.NewRunner(db, work, scanner.Grype{Path: cfg.ScannerPath}, logger, name)
	return serve(cfg, logger, handler, reader, runner)
}

// closeQuietly closes the database at shutdown. A failure here changes nothing
// about the exit, but silence would hide a genuinely stuck connection.
// readInterval is how long an idle reader waits before asking for work again.
//
// It bounds how long a producer waits to see its scan reflected, which nobody
// is watching a clock for, and a queue that is not empty drains without
// waiting for it.
const readInterval = 5 * time.Second

// workerName identifies this process in a claim, so a job held by something
// that has since died can be told apart from one being worked on.
func workerName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s/%d", host, os.Getpid())
}

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
	db, err := database.OpenWithPool(ctx, target, database.Pool{
		MaxOpen:     cfg.DBMaxOpen,
		MaxIdle:     cfg.DBMaxIdle,
		IdleTimeout: cfg.DBIdleTimeout,
		Lifetime:    cfg.DBLifetime,
	})
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

func serve(cfg config.Config, logger *slog.Logger, handler http.Handler, reader *ingest.Reader, runner *scanner.Runner) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Reading stops when the signal arrives, before the server drains, so a
	// scan is not picked up during the seconds we are on our way out.
	//
	// Waited for, not merely signalled. A worker can be mid-query when the
	// signal lands, and returning from here closes the database underneath it
	// — which turns an orderly shutdown into a failed scan and a job that has
	// to be retried for no reason.
	var workers sync.WaitGroup
	if reader != nil {
		workers.Add(1)
		go func() {
			defer workers.Done()
			reader.Run(ctx, readInterval)
		}()
	}
	if runner != nil {
		workers.Add(1)
		go func() {
			defer workers.Done()
			runner.Run(ctx, readInterval)
		}()
	}
	defer workers.Wait()

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: handler,
		// Without these a client can hold a connection, a goroutine and a file
		// descriptor open indefinitely by sending headers slowly. Enough such
		// connections exhaust every replica while the liveness probe keeps
		// passing, because the process itself is fine.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       120 * time.Second,
	}
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
