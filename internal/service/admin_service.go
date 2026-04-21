package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tg-craft-bot/internal/domain"
	repo "tg-craft-bot/internal/repository/postgres"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

type AdminService struct {
	store         *repo.Store
	users         *repo.UserRepository
	userStates    *repo.UserStateRepository
	settings      *repo.SettingsRepository
	orders        *repo.OrderRepository
	notifications *repo.NotificationRepository
	recipes       *RecipeCatalog
	log           *zap.Logger
	loc           *time.Location
}

func NewAdminService(
	store *repo.Store,
	users *repo.UserRepository,
	userStates *repo.UserStateRepository,
	settings *repo.SettingsRepository,
	orders *repo.OrderRepository,
	notifications *repo.NotificationRepository,
	recipes *RecipeCatalog,
	log *zap.Logger,
	loc *time.Location,
) *AdminService {
	return &AdminService{
		store:         store,
		users:         users,
		userStates:    userStates,
		settings:      settings,
		orders:        orders,
		notifications: notifications,
		recipes:       recipes,
		log:           log,
		loc:           loc,
	}
}

func (s *AdminService) ensureAdmin(ctx context.Context, telegramID int64) error {
	u, err := s.users.GetByTelegramID(ctx, telegramID)
	if err != nil {
		return err
	}
	if !u.IsAdmin {
		return domain.ErrForbidden
	}
	return nil
}

func (s *AdminService) SetCraftOpen(ctx context.Context, adminTelegramID int64, isOpen bool) error {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return err
	}
	return s.settings.SetOpen(ctx, isOpen, adminTelegramID)
}

func (s *AdminService) Queue(ctx context.Context, adminTelegramID int64) ([]domain.Order, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return nil, err
	}
	return s.orders.ListQueue(ctx)
}

func (s *AdminService) InProgress(ctx context.Context, adminTelegramID int64) ([]domain.Order, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return nil, err
	}
	return s.orders.ListByStatus(ctx, domain.OrderStatusInProgress, domain.OrderStatusReady)
}

func (s *AdminService) BeginAdminSingleOrder(ctx context.Context, adminUserID, adminTelegramID int64, recipeKey string) error {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return err
	}
	if _, ok := s.recipes.Get(recipeKey); !ok {
		return domain.ErrRecipeNotFound
	}
	return s.userStates.SetAdminSingleOrder(ctx, adminUserID, recipeKey)
}

func (s *AdminService) BeginAdminBulkOrder(ctx context.Context, adminUserID, adminTelegramID int64, recipeKey string) error {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return err
	}
	if _, ok := s.recipes.Get(recipeKey); !ok {
		return domain.ErrRecipeNotFound
	}
	return s.userStates.SetAdminBulkOrder(ctx, adminUserID, recipeKey)
}

func (s *AdminService) CreateOrderByNickname(ctx context.Context, adminTelegramID int64, recipeKey, nickname string) (*domain.Order, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return nil, err
	}

	recipe, ok := s.recipes.Get(recipeKey)
	if !ok {
		return nil, domain.ErrRecipeNotFound
	}

	nickname = strings.TrimSpace(nickname)
	if err := validateNickname(nickname); err != nil {
		return nil, err
	}

	return s.orders.CreateAtTail(ctx, &domain.Order{
		UserID:     nil,
		TelegramID: nil,
		Nickname:   nickname,
		RecipeKey:  recipe.Key,
		RecipeName: recipe.Name,
		RecipeKind: recipe.Type,
		Qty:        recipe.DefaultQty,
	})
}

func (s *AdminService) CreateBulkOrdersByNicknames(ctx context.Context, adminTelegramID int64, recipeKey string, nicknames []string) (created []domain.Order, skipped []string, err error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return nil, nil, err
	}

	recipe, ok := s.recipes.Get(recipeKey)
	if !ok {
		return nil, nil, domain.ErrRecipeNotFound
	}

	for _, nick := range nicknames {
		nick = strings.TrimSpace(nick)
		if nick == "" {
			continue
		}
		if err := validateNickname(nick); err != nil {
			skipped = append(skipped, nick)
			continue
		}

		order, err := s.orders.CreateAtTail(ctx, &domain.Order{
			UserID:     nil,
			TelegramID: nil,
			Nickname:   nick,
			RecipeKey:  recipe.Key,
			RecipeName: recipe.Name,
			RecipeKind: recipe.Type,
			Qty:        recipe.DefaultQty,
		})
		if err != nil {
			skipped = append(skipped, nick)
			continue
		}
		created = append(created, *order)
	}

	return created, skipped, nil
}

func (s *AdminService) StartCraft(ctx context.Context, adminTelegramID, orderID int64) (*domain.Order, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return nil, err
	}

	var out *domain.Order
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		orders := repo.NewOrderRepository(tx)
		notifications := repo.NewNotificationRepository(tx)

		order, err := orders.GetByIDForUpdate(ctx, orderID)
		if err != nil {
			return err
		}
		if order.Status != domain.OrderStatusNew {
			return domain.ErrOrderInvalidStatus
		}

		recipe, ok := s.recipes.Get(order.RecipeKey)
		if !ok {
			return domain.ErrRecipeNotFound
		}

		now := time.Now().In(s.loc)
		readyAt := now.Add(recipe.Duration)
		pickupDeadline := readyAt.Add(4 * time.Hour)

		if err := orders.MarkInProgress(ctx, orderID, now, readyAt, pickupDeadline); err != nil {
			return err
		}
		if err := orders.NormalizeQueue(ctx); err != nil {
			return err
		}

		if order.TelegramID != nil {
			if err := notifications.Create(ctx, domain.Notification{
				OrderID:     orderID,
				TelegramID:  *order.TelegramID,
				Type:        domain.NotificationTypeCraftReady,
				ScheduledAt: readyAt,
			}); err != nil {
				return err
			}
		}

		out, err = orders.GetByID(ctx, orderID)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (s *AdminService) CancelOrder(ctx context.Context, adminTelegramID, orderID int64) error {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return err
	}

	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		orders := repo.NewOrderRepository(tx)

		if err := orders.CancelByAdmin(ctx, orderID); err != nil {
			return err
		}
		return orders.NormalizeQueue(ctx)
	})
}

func (s *AdminService) CompleteOrder(ctx context.Context, adminTelegramID, orderID int64) error {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return err
	}
	return s.orders.MarkCompleted(ctx, orderID)
}

func (s *AdminService) MoveUp(ctx context.Context, adminTelegramID, orderID int64) error {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		orders := repo.NewOrderRepository(tx)
		if err := orders.MoveUp(ctx, orderID); err != nil {
			return err
		}
		return orders.NormalizeQueue(ctx)
	})
}

func (s *AdminService) MoveDown(ctx context.Context, adminTelegramID, orderID int64) error {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		orders := repo.NewOrderRepository(tx)
		if err := orders.MoveDown(ctx, orderID); err != nil {
			return err
		}
		return orders.NormalizeQueue(ctx)
	})
}

func (s *AdminService) MoveToHead(ctx context.Context, adminTelegramID, orderID int64) error {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		orders := repo.NewOrderRepository(tx)
		if err := orders.MoveToHead(ctx, orderID); err != nil {
			return err
		}
		return orders.NormalizeQueue(ctx)
	})
}

func (s *AdminService) MoveToTail(ctx context.Context, adminTelegramID, orderID int64) error {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		orders := repo.NewOrderRepository(tx)
		if err := orders.MoveToTail(ctx, orderID); err != nil {
			return err
		}
		return orders.NormalizeQueue(ctx)
	})
}

func (s *AdminService) QueuePotions(ctx context.Context, adminTelegramID int64) ([]domain.Order, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return nil, err
	}

	return s.orders.ListQueueByKinds(
		ctx,
		domain.RecipePotion,
		domain.RecipeElixir,
		domain.RecipeOther,
	)
}

func (s *AdminService) QueueScrolls(ctx context.Context, adminTelegramID int64) ([]domain.Order, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return nil, err
	}

	return s.orders.ListQueueByKinds(ctx, domain.RecipeScroll)
}

func (s *AdminService) QueueText(ctx context.Context, adminTelegramID int64) (string, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return "", err
	}

	items, err := s.orders.ListQueue(ctx)
	if err != nil {
		return "", err
	}

	if len(items) == 0 {
		return "🗑 Очередь пустая.", nil
	}

	var b strings.Builder
	b.WriteString("Общая очередь:\n\n")

	for _, o := range items {
		pos := int64(0)
		if o.QueuePos != nil {
			pos = *o.QueuePos
		}

		tgText := "нет"
		if o.TelegramID != nil {
			tgText = strconv.FormatInt(*o.TelegramID, 10)
		}

		b.WriteString(fmt.Sprintf(
			"%d. %s ×%d | %s | TG: %s\n",
			pos,
			o.RecipeName,
			o.Qty,
			o.Nickname,
			tgText,
		))
	}

	return b.String(), nil
}

func (s *AdminService) QueuePotionsText(ctx context.Context, adminTelegramID int64) (string, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return "", err
	}

	items, err := s.orders.ListQueueByKinds(
		ctx,
		domain.RecipePotion,
		domain.RecipeElixir,
		domain.RecipeOther,
	)
	if err != nil {
		return "", err
	}

	return buildOrdersText("Очередь по банкам", items), nil
}

func (s *AdminService) QueueScrollsText(ctx context.Context, adminTelegramID int64) (string, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return "", err
	}

	items, err := s.orders.ListQueueByKinds(ctx, domain.RecipeScroll)
	if err != nil {
		return "", err
	}

	return buildOrdersText("Очередь по свиткам", items), nil
}

func (s *AdminService) InProgressText(ctx context.Context, adminTelegramID int64) (string, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return "", err
	}

	items, err := s.orders.ListByStatus(ctx, domain.OrderStatusInProgress, domain.OrderStatusReady)
	if err != nil {
		return "", err
	}

	return buildOrdersText("Заказы в крафте / готовые", items), nil
}

func (s *AdminService) InProgressPotionsText(ctx context.Context, adminTelegramID int64) (string, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return "", err
	}

	items, err := s.orders.ListByStatus(ctx, domain.OrderStatusInProgress, domain.OrderStatusReady)
	if err != nil {
		return "", err
	}

	filtered := filterOrdersByKinds(items, domain.RecipePotion, domain.RecipeElixir, domain.RecipeOther)
	return buildOrdersText("Банки в крафте / готовые", filtered), nil
}

func (s *AdminService) InProgressScrollsText(ctx context.Context, adminTelegramID int64) (string, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return "", err
	}

	items, err := s.orders.ListByStatus(ctx, domain.OrderStatusInProgress, domain.OrderStatusReady)
	if err != nil {
		return "", err
	}

	filtered := filterOrdersByKinds(items, domain.RecipeScroll)
	return buildOrdersText("Свитки в крафте / готовые", filtered), nil
}

func buildOrdersText(title string, items []domain.Order) string {
	if len(items) == 0 {
		return title + ":\n\nПусто."
	}

	var b strings.Builder
	b.WriteString(title)
	b.WriteString(":\n\n")

	for _, o := range items {
		posText := "-"
		if o.QueuePos != nil {
			posText = strconv.FormatInt(*o.QueuePos, 10)
		}

		tgText := "нет"
		if o.TelegramID != nil {
			tgText = strconv.FormatInt(*o.TelegramID, 10)
		}

		b.WriteString(fmt.Sprintf(
			"ID:%d | Очередь:%s | %s ×%d | Ник:%s | TG:%s | Статус:%s\n",
			o.ID,
			posText,
			o.RecipeName,
			o.Qty,
			o.Nickname,
			tgText,
			RuStatus(o.Status),
		))
	}

	return b.String()
}
func RuStatus(s domain.OrderStatus) string {
	switch s {
	case domain.OrderStatusNew:
		return "Создан"
	case domain.OrderStatusInProgress:
		return "Крафтится"
	case domain.OrderStatusReady:
		return "Готов"
	case domain.OrderStatusCompleted:
		return "Выполнен"
	default:
		return "Неизвестно"
	}
}
func filterOrdersByKinds(items []domain.Order, kinds ...domain.RecipeType) []domain.Order {
	if len(kinds) == 0 {
		return items
	}

	allowed := make(map[domain.RecipeType]struct{}, len(kinds))
	for _, k := range kinds {
		allowed[k] = struct{}{}
	}

	out := make([]domain.Order, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.RecipeKind]; ok {
			out = append(out, item)
		}
	}
	return out
}

func (s *AdminService) InProgressPotions(ctx context.Context, adminTelegramID int64) ([]domain.Order, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return nil, err
	}

	items, err := s.orders.ListByStatus(ctx, domain.OrderStatusInProgress, domain.OrderStatusReady)
	if err != nil {
		return nil, err
	}

	return filterOrdersByKinds(
		items,
		domain.RecipePotion,
		domain.RecipeElixir,
		domain.RecipeOther,
	), nil
}

func (s *AdminService) InProgressScrolls(ctx context.Context, adminTelegramID int64) ([]domain.Order, error) {
	if err := s.ensureAdmin(ctx, adminTelegramID); err != nil {
		return nil, err
	}

	items, err := s.orders.ListByStatus(ctx, domain.OrderStatusInProgress, domain.OrderStatusReady)
	if err != nil {
		return nil, err
	}

	return filterOrdersByKinds(items, domain.RecipeScroll), nil
}
