// Command server is the PipelineSherlock HTTP service entry point.
//
// Usage:
//
//	server            # run migrations, then serve HTTP (default)
//	server serve      # same as above
//	server migrate    # apply pending migrations and exit
//	server rollback   # revert the most recent migration and exit
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pgrepo "github.com/Jeston10/pipeline-sherlock/internal/adapters/postgres"
	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/config"
	"github.com/Jeston10/pipeline-sherlock/internal/platform/postgres"
	transporthttp "github.com/Jeston10/pipeline-sherlock/internal/transport/http"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(logger); err != nil {
		logger.Error("fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger.Info("configuration loaded", slog.String("config", cfg.Redacted()))

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	// Bound the time spent connecting / migrating at startup.
	startupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := postgres.Connect(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	switch cmd {
	case "migrate":
		if err := postgres.Migrate(startupCtx, db); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		logger.Info("migrations applied")
		return nil
	case "rollback":
		if err := postgres.Rollback(startupCtx, db); err != nil {
			return fmt.Errorf("rollback: %w", err)
		}
		logger.Info("rolled back latest migration")
		return nil
	case "serve":
		if err := postgres.Migrate(startupCtx, db); err != nil {
			return fmt.Errorf("migrate before serve: %w", err)
		}
		return serve(cfg, db, logger)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func serve(cfg config.Config, db *postgres.DB, logger *slog.Logger) error {
	// Repositories (adapters) and use cases (application), wired here.
	projectRepo := pgrepo.NewProjectRepo(db.Pool)
	eventRepo := pgrepo.NewWebhookEventRepo(db.Pool)
	registerSvc := application.NewRegisterProjectService(projectRepo)
	ingestSvc := application.NewIngestWebhookService(projectRepo, eventRepo)

	handler := transporthttp.NewRouter(transporthttp.Deps{
		Readiness: db,
		Projects:  registerSvc,
		Webhooks:  ingestSvc,
		Logger:    logger,
	})

	srv := &nethttp.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// Trap SIGINT/SIGTERM for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		logger.Info("server stopped cleanly")
		return nil
	}
}
