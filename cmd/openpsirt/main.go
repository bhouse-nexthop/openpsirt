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
	"github.com/bhouse-nexthop/openpsirt/internal/httpapi"
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
	handler, api := httpapi.New(logger)

	// Generating the document from the running registrations is what keeps it
	// from drifting away from the server.
	if *dumpSpec {
		doc, err := api.OpenAPI().YAML()
		if err != nil {
			return fmt.Errorf("render OpenAPI document: %w", err)
		}
		_, err = stdout.Write(doc)
		return err
	}

	return serve(cfg, logger, handler)
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
