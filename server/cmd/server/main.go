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
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/ingest"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
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

	// 5. Start the ingest queues (one for logs, one for metrics): handlers
	// put items on them and reply at once; workers save them to MongoDB in
	// the background. Each queue gets the configured capacity and workers.
	logQueue, err := ingest.New(ingest.Config[model.LogRecord]{
		Name:     "logs",
		Capacity: cfg.IngestQueueSize,
		Workers:  cfg.IngestWorkers,
		Write:    ingest.WriteLogs(db),
		Logger:   logger,
	})
	if err != nil {
		return err
	}
	metricQueue, err := ingest.New(ingest.Config[model.MetricPoint]{
		Name:     "metrics",
		Capacity: cfg.IngestQueueSize,
		Workers:  cfg.IngestWorkers,
		Write:    db.InsertMetrics,
		Logger:   logger,
	})
	if err != nil {
		return err
	}
	logQueue.Start()
	metricQueue.Start()
	logger.Info("ingest queues started", "capacity", cfg.IngestQueueSize, "workers_each", cfg.IngestWorkers)

	// 6. Create the HTTP server. The timeouts protect against slow or
	// stuck clients holding connections open forever.
	srv := &http.Server{
		Addr: net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler: api.NewRouter(api.Deps{
			Logger:   logger,
			DB:       db,
			Users:    db,
			Sessions: db,
			Projects: db,
			Keys:     db,
			Logs:     logQueue,
			Metrics:  metricQueue,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	// 7. Start the HTTP server in the background. ListenAndServe blocks
	// until the server stops, so it runs in its own goroutine and reports
	// its result through the serverErr channel.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", srv.Addr)
		serverErr <- srv.ListenAndServe()
	}()

	// 8. Wait for whichever happens first: the server fails, or a stop signal.
	var startErr error
	select {
	case err := <-serverErr:
		// The server could not start (e.g. port already in use) or stopped
		// by itself. Still shut down properly so queued logs are saved.
		startErr = fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		// Restore normal signal handling, so a second Ctrl+C stops the
		// process immediately if shutdown gets stuck.
		stop()
		logger.Info("shutdown signal received", "timeout", cfg.ShutdownTimeout)
	}

	// 9. Stop cleanly.
	return errors.Join(startErr, shutdown(logger, srv, logQueue, metricQueue, db, cfg.ShutdownTimeout))
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

// drainer is a queue that can be emptied on shutdown and report its counters.
type drainer interface {
	Drain(ctx context.Context) error
	Stats() ingest.Stats
}

// shutdown stops the server in a safe order, within the given time limit:
// stop taking new requests and let running ones finish, save everything
// still in the queues, then close the database.
func shutdown(logger *slog.Logger, srv *http.Server, logQueue, metricQueue drainer, db *store.Store, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var errs []error
	if err := srv.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("http shutdown: %w", err))
	}
	// No request can add items now, so drain both queues into MongoDB at
	// the same time (they share the remaining time limit).
	queues := map[string]drainer{"logs": logQueue, "metrics": metricQueue}
	drainErrs := make(chan error, len(queues))
	for _, q := range queues {
		go func() { drainErrs <- q.Drain(ctx) }()
	}
	for range queues {
		if err := <-drainErrs; err != nil {
			errs = append(errs, fmt.Errorf("ingest drain: %w", err))
		}
	}
	for name, q := range queues {
		st := q.Stats()
		logger.Info("ingest queue stopped", "queue", name, "accepted", st.Accepted, "saved", st.Saved, "dropped", st.Dropped)
	}

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
