package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"tg-craft-bot/internal/config"
	"tg-craft-bot/internal/pkg/logger"
	pg "tg-craft-bot/internal/pkg/postgres"
	repo "tg-craft-bot/internal/repository/postgres"
	"tg-craft-bot/internal/service"
	telegramTransport "tg-craft-bot/internal/transport/telegram"
	"tg-craft-bot/internal/worker"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log, err := logger.New(cfg.LogLevel)
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatal("load timezone", zap.Error(err))
	}

	dbPool, err := pg.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("init db pool", zap.Error(err))
	}
	defer dbPool.Close()

	store := repo.NewStore(dbPool)

	usersRepo := repo.NewUserRepository(dbPool)
	userStatesRepo := repo.NewUserStateRepository(dbPool)
	settingsRepo := repo.NewSettingsRepository(dbPool)
	ordersRepo := repo.NewOrderRepository(dbPool)
	notificationsRepo := repo.NewNotificationRepository(dbPool)

	recipes := service.NewRecipeCatalog()

	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Fatal("init telegram bot", zap.Error(err))
	}
	bot.Debug = cfg.AppEnv == "local"

	orderService := service.NewOrderService(
		store,
		usersRepo,
		userStatesRepo,
		settingsRepo,
		ordersRepo,
		notificationsRepo,
		recipes,
		log,
	)

	adminService := service.NewAdminService(
		store,
		usersRepo,
		userStatesRepo,
		settingsRepo,
		ordersRepo,
		notificationsRepo,
		recipes,
		log,
		loc,
	)

	notificationService := service.NewNotificationService(
		store,
		ordersRepo,
		notificationsRepo,
		bot,
		log,
		loc,
		adminIDsToSlice(cfg.AdminTelegramIDs),
	)

	handler := telegramTransport.NewHandler(
		bot,
		orderService,
		adminService,
		notificationService,
		log,
		loc,
		cfg.AdminTelegramIDs,
	)

	notifyWorker := worker.NewNotificationWorker(notificationService, cfg.WorkerPollInterval, log)
	go notifyWorker.Run(ctx)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = cfg.BotPollTimeout

	updates := bot.GetUpdatesChan(u)

	log.Info("bot started", zap.String("bot_username", bot.Self.UserName))

	for {
		select {
		case <-ctx.Done():
			log.Info("shutdown signal received")
			return
		case upd := <-updates:
			upd = upd
			go func() {
				runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				defer cancel()

				handler.HandleUpdate(runCtx, upd)
			}()
		}
	}
}

func adminIDsToSlice(m map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}
