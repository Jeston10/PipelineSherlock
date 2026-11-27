// Command server is the PipelineSherlock HTTP service entry point.
//
// Usage:
//
//	server            # run migrations, then serve HTTP (default)
//	server serve      # same as above
//	server migrate    # apply pending migrations and exit
//	server rollback   # revert the most recent migration and exit
//	server issue-key <name> <owner>  # mint an API key for an owner, print once, exit
//	server set-project-token <owner> <gitlab_project_id>  # store a project's GitLab token (read from PROJECT_GITLAB_TOKEN), exit
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	gitlabadapter "github.com/Jeston10/pipeline-sherlock/internal/adapters/gitlab"
	pgrepo "github.com/Jeston10/pipeline-sherlock/internal/adapters/postgres"
	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/config"
	"github.com/Jeston10/pipeline-sherlock/internal/platform/crypto"
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
	case "issue-key":
		name, owner := "", ""
		if len(os.Args) > 2 {
			name = os.Args[2]
		}
		if len(os.Args) > 3 {
			owner = os.Args[3]
		}
		return issueAPIKey(startupCtx, db, name, owner, logger)
	case "set-project-token":
		owner, pid := "", ""
		if len(os.Args) > 2 {
			owner = os.Args[2]
		}
		if len(os.Args) > 3 {
			pid = os.Args[3]
		}
		return setProjectToken(startupCtx, cfg, db, owner, pid, logger)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// issueAPIKey mints a new API key under the given name, printing the plaintext
// to stdout exactly once (never logged), then exits. Operators run this to
// obtain a credential for the authenticated /api/v1 surface.
func issueAPIKey(ctx context.Context, db *postgres.DB, name, owner string, logger *slog.Logger) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(owner) == "" {
		return errors.New("usage: server issue-key <name> <owner>")
	}
	if err := postgres.Migrate(ctx, db); err != nil {
		return fmt.Errorf("migrate before issue-key: %w", err)
	}
	svc := application.NewAPIKeyService(pgrepo.NewAPIKeyRepo(db.Pool))
	res, err := svc.Issue(ctx, name, owner)
	if err != nil {
		return fmt.Errorf("issue api key: %w", err)
	}
	fmt.Printf("API key issued.\n  id:    %s\n  name:  %s\n  owner: %s\n  key:   %s\n",
		res.APIKey.ID, res.APIKey.Name, res.APIKey.OwnerID, res.PlaintextKey)
	logger.Info("api key issued",
		slog.String("id", res.APIKey.ID), slog.String("name", res.APIKey.Name), slog.String("owner", res.APIKey.OwnerID))
	return nil
}

// setProjectToken stores an encrypted per-project GitLab token. The token is
// read from the PROJECT_GITLAB_TOKEN environment variable (never passed on the
// command line, so it stays out of shell history), sealed with the master key,
// and persisted. It requires ENCRYPTION_KEY to be configured and the project to
// belong to the given owner.
func setProjectToken(ctx context.Context, cfg config.Config, db *postgres.DB, owner, gitlabProjectID string, logger *slog.Logger) error {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(gitlabProjectID) == "" {
		return errors.New("usage: server set-project-token <owner> <gitlab_project_id> (token read from PROJECT_GITLAB_TOKEN)")
	}
	pid, err := strconv.ParseInt(strings.TrimSpace(gitlabProjectID), 10, 64)
	if err != nil || pid <= 0 {
		return fmt.Errorf("invalid gitlab project id %q", gitlabProjectID)
	}
	token := strings.TrimSpace(os.Getenv("PROJECT_GITLAB_TOKEN"))
	if token == "" {
		return errors.New("PROJECT_GITLAB_TOKEN must be set to the project's GitLab token")
	}
	if strings.TrimSpace(cfg.EncryptionKey) == "" {
		return errors.New("ENCRYPTION_KEY must be configured to store an encrypted project token")
	}
	sealer, err := crypto.NewSealerFromBase64(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("configure encryption: %w", err)
	}
	if err := postgres.Migrate(ctx, db); err != nil {
		return fmt.Errorf("migrate before set-project-token: %w", err)
	}
	svc := application.NewSetProjectTokenService(
		pgrepo.NewProjectRepo(db.Pool),
		pgrepo.NewProjectCredentialRepo(db.Pool),
		sealer,
	)
	if err := svc.Set(ctx, owner, pid, token); err != nil {
		return fmt.Errorf("set project token: %w", err)
	}
	fmt.Printf("Stored encrypted GitLab token for project %d (owner %s).\n", pid, owner)
	logger.Info("project gitlab token stored",
		slog.Int64("gitlab_project_id", pid), slog.String("owner", owner))
	return nil
}

func serve(cfg config.Config, db *postgres.DB, logger *slog.Logger) error {
	// Repositories (adapters) and use cases (application), wired here.
	projectRepo := pgrepo.NewProjectRepo(db.Pool)
	eventRepo := pgrepo.NewWebhookEventRepo(db.Pool)
	pipelineRepo := pgrepo.NewPipelineRepo(db.Pool)
	fingerprintRepo := pgrepo.NewFingerprintRepo(db.Pool)
	incidentRepo := pgrepo.NewIncidentRepo(db.Pool)
	registerSvc := application.NewRegisterProjectService(projectRepo)
	ingestSvc := application.NewIngestWebhookService(projectRepo, eventRepo)
	incidentSvc := application.NewIncidentService(projectRepo, incidentRepo)
	apiKeySvc := application.NewAPIKeyService(pgrepo.NewAPIKeyRepo(db.Pool))

	// Per-project GitLab tokens are stored encrypted; the sealer unwraps them at
	// resolve time. It is optional: without an encryption key only the global
	// token (if any) is used.
	var sealer application.TokenSealer
	if cfg.EncryptionKey != "" {
		s, err := crypto.NewSealerFromBase64(cfg.EncryptionKey)
		if err != nil {
			return fmt.Errorf("configure encryption: %w", err)
		}
		sealer = s
		logger.Info("per-project credential encryption enabled")
	}

	// The background worker reconciles inbox events against GitLab using each
	// project's own token when one is stored, falling back to the global token.
	// A project with neither is reconciled from the webhook payload instead.
	var gitlabClient application.GitLabClient
	var procOpts []application.ProcessOption
	if cfg.GitLabConfigured() || sealer != nil {
		resolver := application.NewProjectTokenResolver(projectRepo, pgrepo.NewProjectCredentialRepo(db.Pool), sealer, cfg.GitLabToken)
		client := gitlabadapter.NewMultiTenant(cfg.GitLabBaseURL, resolver)
		gitlabClient = client
		logger.Info("gitlab reconciliation enabled",
			slog.String("base_url", cfg.GitLabBaseURL),
			slog.Bool("global_token", cfg.GitLabConfigured()),
			slog.Bool("per_project_tokens", sealer != nil),
		)

		// Merge-request write-back is opt-in and needs a global GitLab token to post.
		if cfg.WriteBackActive() {
			writeBack := application.NewWriteBackService(pgrepo.NewCommentRepo(db.Pool), client, logger)
			procOpts = append(procOpts, application.WithWriteBack(writeBack))
			logger.Info("gitlab merge-request write-back enabled")
		}
	} else {
		logger.Info("gitlab reconciliation disabled (no GITLAB_TOKEN or ENCRYPTION_KEY); using webhook payloads only")
	}
	processor := application.NewProcessEventsService(eventRepo, pipelineRepo, fingerprintRepo, gitlabClient, logger, procOpts...)
	worker := application.NewWorker(processor, logger)

	handler := transporthttp.NewRouter(transporthttp.Deps{
		Readiness: db,
		Projects:  registerSvc,
		Webhooks:  ingestSvc,
		Incidents: incidentSvc,
		APIKeys:   apiKeySvc,
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

	// Start the background worker; its lifetime is tied to shutdown.
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if err := worker.Run(workerCtx); err != nil {
			logger.Error("worker exited", slog.String("error", err.Error()))
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		cancelWorker()
		<-workerDone
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		cancelWorker() // signal the worker to stop draining
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		<-workerDone // wait for the worker goroutine to return
		logger.Info("server stopped cleanly")
		return nil
	}
}
