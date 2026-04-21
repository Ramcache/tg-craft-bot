package service

import (
	"context"
	"fmt"
	"time"

	"tg-craft-bot/internal/domain"
	repo "tg-craft-bot/internal/repository/postgres"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

type NotificationService struct {
	store            *repo.Store
	orders           *repo.OrderRepository
	notifications    *repo.NotificationRepository
	bot              *tgbotapi.BotAPI
	log              *zap.Logger
	loc              *time.Location
	adminTelegramIDs []int64
}

func NewNotificationService(
	store *repo.Store,
	orders *repo.OrderRepository,
	notifications *repo.NotificationRepository,
	bot *tgbotapi.BotAPI,
	log *zap.Logger,
	loc *time.Location,
	adminTelegramIDs []int64,
) *NotificationService {
	return &NotificationService{
		store:            store,
		orders:           orders,
		notifications:    notifications,
		bot:              bot,
		log:              log,
		loc:              loc,
		adminTelegramIDs: adminTelegramIDs,
	}
}

func (s *NotificationService) SendCraftStarted(ctx context.Context, order *domain.Order) error {
	if order.TelegramID == nil {
		return nil
	}
	if order.ReadyAt == nil || order.PickupDeadlineAt == nil {
		return fmt.Errorf("order timestamps are empty")
	}

	text := fmt.Sprintf(
		"❗️Вам поставили крафт.\n\n📍Заказ: %s ×%d\n📝Ник: %s\n📅Крафт будет готов: %s\n📅Забрать крафт до: %s\n\n‼️Если будут проблемы с получением — пишите @derzkiynub в ЛС.",
		order.RecipeName,
		order.Qty,
		order.Nickname,
		order.ReadyAt.In(s.loc).Format("02.01 15:04"),
		order.PickupDeadlineAt.In(s.loc).Format("02.01 15:04"),
	)

	msg := tgbotapi.NewMessage(*order.TelegramID, text)
	_, err := s.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("send craft started message: %w", err)
	}
	return nil
}

func (s *NotificationService) ProcessDue(ctx context.Context, batchSize int) error {
	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		orders := repo.NewOrderRepository(tx)
		notifications := repo.NewNotificationRepository(tx)

		items, err := notifications.LockDue(ctx, batchSize)
		if err != nil {
			return err
		}

		for _, item := range items {
			switch item.Type {
			case domain.NotificationTypeCraftReady:
				order, err := orders.GetByID(ctx, item.OrderID)
				if err != nil {
					_ = notifications.MarkFailed(ctx, item.ID, err.Error())
					continue
				}

				if order.Status == domain.OrderStatusInProgress {
					if err := orders.MarkReady(ctx, order.ID); err != nil {
						_ = notifications.MarkFailed(ctx, item.ID, err.Error())
						continue
					}

					order, err = orders.GetByID(ctx, order.ID)
					if err != nil {
						_ = notifications.MarkFailed(ctx, item.ID, err.Error())
						continue
					}
				}

				if order.PickupDeadlineAt == nil {
					_ = notifications.MarkFailed(ctx, item.ID, "pickup_deadline_at is nil")
					continue
				}

				if order.TelegramID == nil {
					s.NotifyAdminsOrderReady(ctx, order)

					if err := notifications.MarkSent(ctx, item.ID); err != nil {
						return err
					}
					continue
				}

				goldText := s.pickupGoldText(order)

				text := fmt.Sprintf(
					"✅Крафт готов, забирайте.\n\n📍Заказ: %s ×%d\n📝Ник: %s\n📅Забрать крафт до: %s\n💰Вам нужно положить на СКЛАД ЗАМКА: %s ЗОЛОТО💰\n\n‼️После получения нажмите кнопку ниже.\n‼️Если будут проблемы с получением — пишите @derzkiynub в ЛС.",
					order.RecipeName,
					order.Qty,
					order.Nickname,
					order.PickupDeadlineAt.In(s.loc).Format("02.01 15:04"),
					goldText,
				)

				msg := tgbotapi.NewMessage(*order.TelegramID, text)
				msg.ReplyMarkup = orderReadyKeyboard(order.ID)
				if _, err := s.bot.Send(msg); err != nil {
					_ = notifications.MarkFailed(ctx, item.ID, err.Error())
					continue
				}

				s.NotifyAdminsOrderReady(ctx, order)

				if err := notifications.MarkSent(ctx, item.ID); err != nil {
					return err
				}
			default:
				_ = notifications.MarkFailed(ctx, item.ID, "unsupported notification type")
			}
		}

		return nil
	})
}

func (s *NotificationService) NotifyAdminsOrderReady(ctx context.Context, order *domain.Order) {
	text := fmt.Sprintf(
		"✅Крафт готов.\n\n📍Заказ #%d\n📜Рецепт: %s ×%d\n📝Ник: %s",
		order.ID,
		order.RecipeName,
		order.Qty,
		order.Nickname,
	)
	s.notifyAdmins(text)
}

func (s *NotificationService) NotifyAdminsOrderClaimed(ctx context.Context, order *domain.Order) {
	text := fmt.Sprintf(
		"✅ Пользователь подтвердил получение заказа.\n\n📍Заказ #%d\n📜Рецепт: %s ×%d\n📝Ник: %s",
		order.ID,
		order.RecipeName,
		order.Qty,
		order.Nickname,
	)
	s.notifyAdmins(text)
}

func (s *NotificationService) notifyAdmins(text string) {
	for _, adminID := range s.adminTelegramIDs {
		msg := tgbotapi.NewMessage(adminID, text)
		if _, err := s.bot.Send(msg); err != nil {
			s.log.Error("send admin notification", zap.Error(err), zap.Int64("admin_telegram_id", adminID))
		}
	}
}

func orderReadyKeyboard(orderID int64) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Забрал заказ✅", fmt.Sprintf("order:claim:%d", orderID)),
		),
	)
}

func (s *NotificationService) pickupGoldText(order *domain.Order) string {
	switch order.RecipeKind {
	case domain.RecipeScroll:
		return "50.000"
	case domain.RecipePotion, domain.RecipeElixir:
		return "15.000"
	default:
		return "15.000"
	}
}
