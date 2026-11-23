package application

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// defaultIdleDelay is how long the worker waits before polling the inbox again
// once it has been drained.
const defaultIdleDelay = 2 * time.Second

// Worker drains the durable inbox continuously. It drains eagerly while events
// are available and backs off to a poll interval when the inbox is empty. It
// stops cleanly when its context is cancelled.
type Worker struct {
	processor *ProcessEventsService
	idleDelay time.Duration
	logger    *slog.Logger
}

// NewWorker wires a Worker around a processor.
func NewWorker(processor *ProcessEventsService, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		processor: processor,
		idleDelay: defaultIdleDelay,
		logger:    logger,
	}
}

// Run loops until ctx is cancelled. It returns nil on a clean stop.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("inbox worker started")
	for {
		if ctx.Err() != nil {
			w.logger.Info("inbox worker stopped")
			return nil
		}

		progressed, err := w.processor.ProcessNext(ctx)
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			w.logger.Info("inbox worker stopped")
			return nil
		case err != nil:
			w.logger.Error("inbox worker error", slog.String("error", err.Error()))
			if sleepErr := w.idle(ctx); sleepErr != nil {
				return nil
			}
		case !progressed:
			// Inbox empty; wait before polling again.
			if sleepErr := w.idle(ctx); sleepErr != nil {
				return nil
			}
		default:
			// Made progress; loop immediately to keep draining.
		}
	}
}

// idle waits idleDelay or until ctx is cancelled.
func (w *Worker) idle(ctx context.Context) error {
	t := time.NewTimer(w.idleDelay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
