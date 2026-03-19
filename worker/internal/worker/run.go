package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/logging"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/ocr"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/pvp"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/species"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/videoproc"
)

// Run starts the worker loop for health checks and queue lifecycle processing.
func Run(cfg config.Config, storage config.StorageConfig) {
	logger := logging.New(cfg.AppEnv, cfg.BetterstackToken, cfg.BetterstackEndpoint)
	workerID := newWorkerID()
	leaseTimeout := leaseTimeoutForPollInterval(cfg.PollIntervalSecs)
	heartbeatInterval := heartbeatIntervalForPollInterval(cfg.PollIntervalSecs)

	catalog, err := species.LoadCatalogFromFile(cfg.GameMasterPath)
	if err != nil {
		logger.Error(
			"worker gamemaster catalog initialization failed",
			"error", err,
			"gamemaster_path", cfg.GameMasterPath,
		)
		return
	}

	videoSampler := videoproc.NewFFmpegSampler(time.Duration(cfg.VideoSamplingIntervalMS) * time.Millisecond)
	databaseDSN := cfg.DatabaseDSN()
	processor := newImageProcessor(databaseDSN, storage, heartbeatInterval, ocr.NewTesseractEngine(), catalog, videoSampler)

	queueStore, err := jobqueue.NewSQLiteStore(databaseDSN)
	if err != nil {
		logger.Error("worker queue store initialization failed", "error", err, "database_url", cfg.DatabaseURL)
		return
	}
	defer queueStore.Close()

	appraisalStore, err := appraisal.NewSQLiteStore(databaseDSN)
	if err != nil {
		logger.Error("worker appraisal store initialization failed", "error", err, "database_url", cfg.DatabaseURL)
		return
	}
	defer appraisalStore.Close()

	evolutionGraph, err := pvp.LoadEvolutionGraphFromFile(cfg.GameMasterPath)
	if err != nil {
		logger.Error("worker evolution graph initialization failed", "error", err, "gamemaster_path", cfg.GameMasterPath)
		return
	}
	pvpRunner := newPVPEvalRunner(appraisalStore, pvp.NewEvaluator(catalog), evolutionGraph, logger)

	logger.Info(
		"worker started",
		"env", cfg.AppEnv,
		"poll_interval_secs", cfg.PollIntervalSecs,
		"health_port", cfg.HealthPort,
		"storage_mode", storage.Mode,
		"worker_id", workerID,
		"lease_timeout", leaseTimeout.String(),
		"heartbeat_interval", heartbeatInterval.String(),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "worker",
			"env":     cfg.AppEnv,
		})
	})

	healthServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("worker health server failed", "error", err)
			stop()
		}
	}()

	ticker := time.NewTicker(time.Duration(cfg.PollIntervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := healthServer.Shutdown(shutdownCtx); err != nil {
				logger.Warn("worker health server shutdown failed", "error", err)
			}

			logger.Info("worker shutting down")
			return
		case t := <-ticker.C:
			runQueueTick(
				ctx,
				queueStore,
				logger,
				workerID,
				leaseTimeout,
				heartbeatInterval,
				pvpRunner,
				processor,
				t,
				time.Now,
			)
		}
	}
}

func runQueueTick(
	ctx context.Context,
	queueStore jobqueue.Store,
	logger *slog.Logger,
	workerID string,
	leaseTimeout time.Duration,
	heartbeatInterval time.Duration,
	pvpRunner pvpQueueRunner,
	processor Processor,
	tickTime time.Time,
	nowFn func() time.Time,
) {
	now := tickTime.UTC()
	cutoff := now.Add(-leaseTimeout)

	if pvpRunner != nil {
		processedRows, err := pvpRunner.ProcessQueue(ctx, defaultPVPEvalQueueBatchSize, now)
		if err != nil {
			logger.Warn("failed to process pvp evaluation queue", "error", err)
		} else if processedRows > 0 {
			logger.Info("processed pvp evaluation queue", "count", processedRows)
		}
	}

	expiredRows, err := queueStore.FailExpiredProcessingJobs(ctx, cutoff, now)
	if err != nil {
		logger.Warn("failed to expire stale jobs", "error", err)
	} else if expiredRows > 0 {
		logger.Info("expired stale jobs", "count", expiredRows, "cutoff", cutoff.Format(time.RFC3339))
	}

	job, _, err := queueStore.ClaimNextQueuedJob(ctx, workerID, now)
	if err != nil {
		logger.Warn("failed to claim queued job", "error", err)
		return
	}

	logger.Info("job claimed", "job_id", job.ID, "upload_id", job.UploadID)
	runClaimedJobLifecycle(ctx, queueStore, logger, job, workerID, heartbeatInterval, processor, nowFn)
}
