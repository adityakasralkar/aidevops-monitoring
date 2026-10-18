// Command server runs the AIDevOps ingestion server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/api"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/config"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/store"
)

// Time limits for startup steps, so a broken database cannot make the
// server hang forever before it starts.
const (
	connectTimeout = 10 * time.Second
	schemaTimeout  = 30 * time.Second
)

func main() {
	if err := run(); err != nil {
		// Print to stderr directly: if the config was invalid, the logger
		// was never created.
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
}

// run starts the server and blocks until it is stopped.
// It returns an error if startup fails or shutdown is not clean.
func run() error {
	// 1. Load and check the settings.
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// 2. Set up logging (the config is logged with secrets hidden).
	logger := newLogger(cfg)
	slog.SetDefault(logger)
	logger.Info("starting server", "config", cfg)

	// 3. Listen for Ctrl+C (SIGINT) and SIGTERM. When one arrives, ctx is cancelled.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 4. Connect to MongoDB and create collections and indexes.
	db, err := connectDB(ctx, cfg)
	if err != nil {
		return err
	}
	// Safety net: make sure the database is closed when run returns.
	// On a normal stop, shutdown() closes it first and this does nothing.
	defer func() { _ = db.Close(context.Background()) }()
	logger.Info("connected to mongodb", "database", cfg.MongoDBName)

	// 5. Create the HTTP server. The timeouts protect against slow or
	// stuck clients holding connections open forever.
	srv := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler:           api.NewRouter(logger, db),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	// 6. Start the HTTP server in the background. ListenAndServe blocks
	// until the server stops, so it runs in its own goroutine and reports
	// its result through the serverErr channel.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", srv.Addr)
		serverErr <- srv.ListenAndServe()
	}()

	// 7. Wait for whichever happens first: the server fails, or a stop signal.
	select {
	case err := <-serverErr:
		// The server could not start (e.g. port already in use) or stopped by itself.
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		// Restore normal signal handling, so a second Ctrl+C stops the
		// process immediately if shutdown gets stuck.
		stop()
		logger.Info("shutdown signal received", "timeout", cfg.ShutdownTimeout)
	}

	// 8. Stop cleanly.
	return shutdown(logger, srv, db, cfg.ShutdownTimeout)
}

// connectDB connects to MongoDB and makes sure collections and indexes exist.
func connectDB(ctx context.Context, cfg config.Config) (*store.Store, error) {
	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	db, err := store.Connect(connectCtx, cfg.MongoURI, cfg.MongoDBName)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	schemaCtx, cancel := context.WithTimeout(ctx, schemaTimeout)
	defer cancel()
	if err := db.EnsureSchema(schemaCtx); err != nil {
		_ = db.Close(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("ensure schema: %w", err)
	}
	return db, nil
}

// shutdown stops the server in a safe order, within the given time limit:
// stop taking new requests, let running requests finish, then close the database.
func shutdown(logger *slog.Logger, srv *http.Server, db *store.Store, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var errs []error
	if err := srv.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("http shutdown: %w", err))
	}
	// Later: the ingestion queue will be emptied into MongoDB here,
	// before the database connection is closed.
	if err := db.Close(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}
	logger.Info("server stopped cleanly")
	return nil
}

// newLogger returns a human-readable text logger in development and a
// JSON logger in production (easier for log tools to process).
func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.AppEnv == config.EnvProduction {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
