package worker

import (
	"context"
	"time"

	"tg-craft-bot/internal/service"

	"go.uber.org/zap"
)

type NotificationWorker struct {
	service  *service.NotificationService
	interval time.Duration
	log      *zap.Logger
}

func NewNotificationWorker(
	service *service.NotificationService,
	interval time.Duration,
	log *zap.Logger,
) *NotificationWorker {
	return &NotificationWorker{
		service:  service,
		interval: interval,
		log:      log,
	}
}

func (w *NotificationWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.log.Info("notification worker started", zap.Duration("interval", w.interval))

	for {
		select {
		case <-ctx.Done():
			w.log.Info("notification worker stopped")
			return
		case <-ticker.C:
			runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := w.service.ProcessDue(runCtx, 50)
			cancel()
			if err != nil {
				w.log.Error("process due notifications", zap.Error(err))
			}
		}
	}
}
