package service

import (
	"context"
	"regexp"
	"strings"

	"tg-craft-bot/internal/domain"
	repo "tg-craft-bot/internal/repository/postgres"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

type OrderService struct {
	store         *repo.Store
	users         *repo.UserRepository
	userStates    *repo.UserStateRepository
	settings      *repo.SettingsRepository
	orders        *repo.OrderRepository
	notifications *repo.NotificationRepository
	recipes       *RecipeCatalog
	log           *zap.Logger
}

func NewOrderService(
	store *repo.Store,
	users *repo.UserRepository,
	userStates *repo.UserStateRepository,
	settings *repo.SettingsRepository,
	orders *repo.OrderRepository,
	notifications *repo.NotificationRepository,
	recipes *RecipeCatalog,
	log *zap.Logger,
) *OrderService {
	return &OrderService{
		store:         store,
		users:         users,
		userStates:    userStates,
		settings:      settings,
		orders:        orders,
		notifications: notifications,
		recipes:       recipes,
		log:           log,
	}
}

func (s *OrderService) EnsureUser(
	ctx context.Context,
	telegramID int64,
	username, firstName, lastName string,
	isAdmin bool,
) (*domain.User, error) {
	return s.users.UpsertTelegramUser(ctx, telegramID, username, firstName, lastName, isAdmin)
}

func (s *OrderService) BeginCreateOrder(ctx context.Context, user *domain.User, recipeKey string) (autoNickname string, needsNickname bool, err error) {
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return "", false, err
	}
	if !settings.IsOpen {
		return "", false, domain.ErrCraftClosed
	}

	recipe, ok := s.recipes.Get(recipeKey)
	if !ok {
		return "", false, domain.ErrRecipeNotFound
	}

	if recipe.AdminOnly && !user.IsAdmin {
		return "", false, domain.ErrForbidden
	}

	hasActive, err := s.orders.HasActiveOrder(ctx, user.TelegramID)
	if err != nil {
		return "", false, err
	}
	if hasActive {
		return "", false, domain.ErrActiveOrderExists
	}

	if strings.TrimSpace(user.LastNickname) != "" {
		if err := s.userStates.SetConfirmOrder(ctx, user.ID, recipeKey, user.LastNickname); err != nil {
			return "", false, err
		}
		return user.LastNickname, false, nil
	}

	if err := s.userStates.SetAwaitingNickname(ctx, user.ID, recipeKey); err != nil {
		return "", false, err
	}
	return "", true, nil
}

func (s *OrderService) SetNewNicknameForPendingOrder(ctx context.Context, user *domain.User, nickname string) error {
	nickname = strings.TrimSpace(nickname)
	if err := validateNickname(nickname); err != nil {
		return err
	}

	state, err := s.userStates.Get(ctx, user.ID)
	if err != nil {
		return err
	}
	if state.PendingRecipeKey == nil {
		return domain.ErrBadInput
	}

	return s.userStates.SetConfirmOrder(ctx, user.ID, *state.PendingRecipeKey, nickname)
}

func (s *OrderService) ConfirmCreateOrder(ctx context.Context, user *domain.User) (*domain.Order, error) {
	state, err := s.userStates.Get(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if state.Step != "confirm_order" || state.PendingRecipeKey == nil || state.PendingNickname == nil {
		return nil, domain.ErrBadInput
	}

	recipe, ok := s.recipes.Get(*state.PendingRecipeKey)
	if !ok {
		return nil, domain.ErrRecipeNotFound
	}

	nickname := strings.TrimSpace(*state.PendingNickname)
	if err := validateNickname(nickname); err != nil {
		return nil, err
	}

	var created *domain.Order
	err = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		users := repo.NewUserRepository(tx)
		userStates := repo.NewUserStateRepository(tx)
		settings := repo.NewSettingsRepository(tx)
		orders := repo.NewOrderRepository(tx)

		cfg, err := settings.Get(ctx)
		if err != nil {
			return err
		}
		if !cfg.IsOpen {
			return domain.ErrCraftClosed
		}

		hasActive, err := orders.HasActiveOrder(ctx, user.TelegramID)
		if err != nil {
			return err
		}
		if hasActive {
			return domain.ErrActiveOrderExists
		}

		if err := users.SetLastNickname(ctx, user.ID, nickname); err != nil {
			return err
		}

		tgID := user.TelegramID
		userID := user.ID
		created, err = orders.CreateAtTail(ctx, &domain.Order{
			UserID:     &userID,
			TelegramID: &tgID,
			Nickname:   nickname,
			RecipeKey:  recipe.Key,
			RecipeName: recipe.Name,
			RecipeKind: recipe.Type,
			Qty:        recipe.DefaultQty,
		})
		if err != nil {
			if repo.IsUniqueViolation(err) {
				return domain.ErrActiveOrderExists
			}
			return err
		}

		if err := userStates.Clear(ctx, user.ID); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (s *OrderService) RequestNicknameChange(ctx context.Context, user *domain.User) error {
	state, err := s.userStates.Get(ctx, user.ID)
	if err != nil {
		return err
	}
	if state.PendingRecipeKey == nil {
		return domain.ErrBadInput
	}
	return s.userStates.SetAwaitingNickname(ctx, user.ID, *state.PendingRecipeKey)
}

func (s *OrderService) CancelOwnOrder(ctx context.Context, telegramID int64) error {
	active, err := s.orders.GetActiveByTelegramID(ctx, telegramID)
	if err != nil {
		return err
	}

	if active.Status != domain.OrderStatusNew {
		return domain.ErrOrderInvalidStatus
	}

	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		orders := repo.NewOrderRepository(tx)

		if err := orders.CancelByUser(ctx, active.ID); err != nil {
			return err
		}
		return orders.NormalizeQueue(ctx)
	})
}

func (s *OrderService) GetMyActiveOrder(ctx context.Context, telegramID int64) (*domain.Order, error) {
	return s.orders.GetActiveByTelegramID(ctx, telegramID)
}

func (s *OrderService) GetCraftSettings(ctx context.Context) (*domain.CraftSettings, error) {
	return s.settings.Get(ctx)
}

func (s *OrderService) GetUserState(ctx context.Context, userID int64) (*domain.UserState, error) {
	return s.userStates.Get(ctx, userID)
}

func (s *OrderService) ClearUserState(ctx context.Context, userID int64) error {
	return s.userStates.Clear(ctx, userID)
}

func (s *OrderService) Recipes() []domain.Recipe {
	return s.recipes.List()
}

func (s *OrderService) ClaimReadyOrder(ctx context.Context, telegramID, orderID int64) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if order.TelegramID == nil || *order.TelegramID != telegramID {
		return nil, domain.ErrForbidden
	}

	if order.Status != domain.OrderStatusReady {
		return nil, domain.ErrOrderInvalidStatus
	}

	if err := s.orders.MarkCompleted(ctx, orderID); err != nil {
		return nil, err
	}

	return s.orders.GetByID(ctx, orderID)
}

func (s *OrderService) UserRecipes(isAdmin bool) []domain.Recipe {
	all := s.recipes.List()
	if isAdmin {
		return all
	}

	out := make([]domain.Recipe, 0, len(all))
	for _, r := range all {
		if r.AdminOnly {
			continue
		}
		out = append(out, r)
	}
	return out
}

var nicknameRE = regexp.MustCompile(`^[A-Za-z0-9 _-]{3,32}$`)

func validateNickname(nickname string) error {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		return domain.ErrBadInput
	}

	if !nicknameRE.MatchString(nickname) {
		return domain.ErrBadInput
	}

	return nil
}
