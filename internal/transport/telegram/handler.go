package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tg-craft-bot/internal/domain"
	"tg-craft-bot/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

type Handler struct {
	bot           *tgbotapi.BotAPI
	orderService  *service.OrderService
	adminService  *service.AdminService
	notifyService *service.NotificationService
	log           *zap.Logger
	loc           *time.Location
	adminIDs      map[int64]struct{}
}

func NewHandler(
	bot *tgbotapi.BotAPI,
	orderService *service.OrderService,
	adminService *service.AdminService,
	notifyService *service.NotificationService,
	log *zap.Logger,
	loc *time.Location,
	adminIDs map[int64]struct{},
) *Handler {
	return &Handler{
		bot:           bot,
		orderService:  orderService,
		adminService:  adminService,
		notifyService: notifyService,
		log:           log,
		loc:           loc,
		adminIDs:      adminIDs,
	}
}

func (h *Handler) HandleUpdate(ctx context.Context, upd tgbotapi.Update) {
	switch {
	case upd.Message != nil:
		h.handleMessage(ctx, upd.Message)
	case upd.CallbackQuery != nil:
		h.handleCallback(ctx, upd.CallbackQuery)
	}
}

func (h *Handler) isAdmin(telegramID int64) bool {
	_, ok := h.adminIDs[telegramID]
	return ok
}

func (h *Handler) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	if msg.From == nil {
		return
	}

	user, err := h.orderService.EnsureUser(
		ctx,
		msg.From.ID,
		msg.From.UserName,
		msg.From.FirstName,
		msg.From.LastName,
		h.isAdmin(msg.From.ID),
	)
	if err != nil {
		h.log.Error("ensure user", zap.Error(err))
		h.reply(msg.Chat.ID, "Ошибка, попробуйте позже.", 0)
		return
	}

	text := strings.TrimSpace(msg.Text)

	switch text {
	case "/start":
		h.replyWithKeyboard(msg.Chat.ID, "Выберите действие.", mainKeyboard(user.IsAdmin), 0)
		return

	case "Заказать":
		settings, err := h.orderService.GetCraftSettings(ctx)
		if err != nil {
			h.reply(msg.Chat.ID, "❌Ошибка, попробуйте позже.", 0)
			return
		}
		if !settings.IsOpen {
			h.reply(msg.Chat.ID, "❌Крафт на текущий момент закрыт, следите за новостями.", 0)
			return
		}
		m := tgbotapi.NewMessage(msg.Chat.ID, "📍Выберите нужный крафт из списка ниже.")
		m.ReplyMarkup = recipesKeyboard(h.orderService.UserRecipes(user.IsAdmin), "order:recipe:")
		_, _ = h.bot.Send(m)
		return

	case "Мой заказ":
		order, err := h.orderService.GetMyActiveOrder(ctx, user.TelegramID)
		if err != nil {
			if err == domain.ErrOrderNotFound {
				h.reply(msg.Chat.ID, "Активного заказа нет.", 0)
				return
			}
			h.reply(msg.Chat.ID, "Ошибка, попробуйте позже.", 0)
			return
		}

		msgOut := tgbotapi.NewMessage(msg.Chat.ID, formatOrder(order, h.loc))
		if order.Status == domain.OrderStatusNew {
			msgOut.ReplyMarkup = myOrderKeyboard(order.ID)
		}
		_, _ = h.bot.Send(msgOut)
		return

	case "Админ":
		if !user.IsAdmin {
			h.reply(msg.Chat.ID, "Нет доступа.", 0)
			return
		}
		settings, err := h.orderService.GetCraftSettings(ctx)
		if err != nil {
			h.reply(msg.Chat.ID, "Ошибка, попробуйте позже.", 0)
			return
		}
		m := tgbotapi.NewMessage(msg.Chat.ID, "Админ-панель")
		m.ReplyMarkup = adminMenuKeyboard(settings.IsOpen)
		_, _ = h.bot.Send(m)
		return
	}

	state, err := h.orderService.GetUserState(ctx, user.ID)
	if err != nil {
		if err != domain.ErrNoUserState {
			h.log.Error("get user state", zap.Error(err), zap.Int64("user_id", user.ID))
			h.reply(msg.Chat.ID, "Ошибка состояния заказа. Попробуйте заново.", 0)
		}
		return
	}

	h.log.Info("user state loaded",
		zap.Int64("user_id", user.ID),
		zap.String("step", state.Step),
		zap.Stringp("pending_recipe_key", state.PendingRecipeKey),
		zap.Stringp("pending_nickname", state.PendingNickname),
	)

	switch state.Step {
	case "awaiting_nickname":
		if err := h.orderService.SetNewNicknameForPendingOrder(ctx, user, text); err != nil {
			h.reply(msg.Chat.ID, "Ошибка в нике. Укажите полный игровой ник на английском языке.", 0)
			return
		}
		s, err := h.orderService.GetUserState(ctx, user.ID)
		if err != nil || s.PendingRecipeKey == nil || s.PendingNickname == nil {
			h.reply(msg.Chat.ID, "Ошибка, попробуйте заново.", 0)
			return
		}
		recipe, ok := recipeByKey(h.orderService.Recipes(), *s.PendingRecipeKey)
		if !ok {
			h.reply(msg.Chat.ID, "Ошибка рецепта.", 0)
			return
		}
		msgOut := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf(
			"📜 Рецепт: %s ×%d\nНик: %s\n\n‼️Подтвердить заказ или сменить ник?\n",
			recipe.Name, recipe.DefaultQty, *s.PendingNickname,
		))
		msgOut.ReplyMarkup = confirmOrderKeyboard()
		_, _ = h.bot.Send(msgOut)
		return

	case "admin_single_order_nickname":
		order, err := h.createAdminSingleByState(ctx, user, text)
		if err != nil {
			if err == domain.ErrBadInput {
				h.reply(msg.Chat.ID, "Ошибка в нике. Укажите полный игровой ник на английском языке.", 0)
				return
			}
			h.reply(msg.Chat.ID, "❌Не удалось создать заказ.", 0)
			return
		}
		_ = h.orderService.ClearUserState(ctx, user.ID)
		h.reply(msg.Chat.ID, fmt.Sprintf("Заказ создан: %s ×%d, ник %s. Добавлен в конец очереди.", order.RecipeName, order.Qty, order.Nickname), 0)
		return

	case "admin_bulk_order_nicknames":
		created, skipped, err := h.createAdminBulkByState(ctx, user, text)
		if err != nil {
			h.reply(msg.Chat.ID, "❌Не удалось создать массовый заказ.", 0)
			return
		}
		_ = h.orderService.ClearUserState(ctx, user.ID)

		var b strings.Builder
		b.WriteString(fmt.Sprintf("Создано заказов: %d\n", len(created)))
		if len(skipped) > 0 {
			b.WriteString("\nПропущены:\n")
			for _, s := range skipped {
				b.WriteString("- " + s + "\n")
			}
		}
		h.reply(msg.Chat.ID, b.String(), 0)
		return
	}
}

func (h *Handler) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	if cb.From == nil {
		return
	}
	defer h.answerCallback(cb.ID, "")
	if cb.Data == "noop" {
		return
	}

	user, err := h.orderService.EnsureUser(
		ctx,
		cb.From.ID,
		cb.From.UserName,
		cb.From.FirstName,
		cb.From.LastName,
		h.isAdmin(cb.From.ID),
	)
	if err != nil {
		h.log.Error("ensure user on callback", zap.Error(err))
		return
	}

	parts := strings.Split(cb.Data, ":")
	if len(parts) < 1 {
		return
	}

	switch parts[0] {
	case "flow":
		if len(parts) == 2 && parts[1] == "cancel" {
			_ = h.orderService.ClearUserState(ctx, user.ID)
			h.reply(cb.Message.Chat.ID, "Действие отменено.", 0)
			return
		}
	case "order":
		h.handleOrderCallback(ctx, user, cb, parts)
	case "admin":
		h.handleAdminCallback(ctx, user, cb, parts)
	}
}

func (h *Handler) handleOrderCallback(ctx context.Context, user *domain.User, cb *tgbotapi.CallbackQuery, parts []string) {
	if len(parts) == 3 && parts[1] == "recipe" {
		recipeKey := parts[2]
		autoNickname, needsNickname, err := h.orderService.BeginCreateOrder(ctx, user, recipeKey)
		if err != nil {
			switch err {
			case domain.ErrCraftClosed:
				h.reply(cb.Message.Chat.ID, "❌Крафт на текущий момент закрыт, следите за новостями.\n.", 0)
			case domain.ErrActiveOrderExists:
				h.reply(cb.Message.Chat.ID, "‼️У вас уже есть активный заказ. Новый оформить нельзя.", 0)
			case domain.ErrForbidden:
				h.reply(cb.Message.Chat.ID, "🔐Этот рецепт доступен только админу.", 0)
			default:
				h.reply(cb.Message.Chat.ID, "❌Ошибка, попробуйте позже.", 0)
			}
			return
		}

		if needsNickname {
			h.reply(cb.Message.Chat.ID, "Отправьте ваш полный игровой ник одним сообщением на английском языке.", 0)
			return
		}

		recipe, ok := recipeByKey(h.orderService.Recipes(), recipeKey)
		if !ok {
			h.reply(cb.Message.Chat.ID, "Ошибка рецепта.", 0)
			return
		}

		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, fmt.Sprintf(
			"📜Рецепт: %s ×%d\nНик: %s\n\n‼️Подтвердить заказ или сменить ник?\n",
			recipe.Name, recipe.DefaultQty, autoNickname,
		))
		msg.ReplyMarkup = confirmOrderKeyboard()
		_, _ = h.bot.Send(msg)
		return
	}

	if len(parts) == 3 && parts[1] == "claim" {
		h.log.Info("claim callback received",
			zap.Int64("telegram_id", user.TelegramID),
			zap.String("callback_data", cb.Data),
		)

		orderID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			h.reply(cb.Message.Chat.ID, "Некорректный ID заказа.", 0)
			return
		}

		order, err := h.orderService.ClaimReadyOrder(ctx, user.TelegramID, orderID)
		if err != nil {
			h.log.Error("claim ready order failed",
				zap.Error(err),
				zap.Int64("order_id", orderID),
				zap.Int64("telegram_id", user.TelegramID),
			)

			switch err {
			case domain.ErrForbidden:
				h.reply(cb.Message.Chat.ID, "Это не ваш заказ.", 0)
			case domain.ErrOrderInvalidStatus:
				h.reply(cb.Message.Chat.ID, "Этот заказ сейчас нельзя подтвердить.", 0)
			case domain.ErrOrderNotFound:
				h.reply(cb.Message.Chat.ID, "Заказ не найден.", 0)
			default:
				h.reply(cb.Message.Chat.ID, "❌Не удалось подтвердить получение.", 0)
			}
			return
		}

		h.notifyService.NotifyAdminsOrderClaimed(ctx, order)
		h.reply(cb.Message.Chat.ID, "✅Отлично, заказ отмечен как полученный.", 0)
		return
	}

	if len(parts) == 2 && parts[1] == "confirm" {
		order, err := h.orderService.ConfirmCreateOrder(ctx, user)
		if err != nil {
			h.reply(cb.Message.Chat.ID, "❌Не удалось создать заказ.", 0)
			return
		}
		h.reply(cb.Message.Chat.ID, fmt.Sprintf(
			"✅ Заказ создан.\n\n📜 Рецепт: %s ×%d\nНик: %s\n\nВы в очереди.",
			order.RecipeName, order.Qty, order.Nickname,
		), 0)
		return
	}

	if len(parts) == 2 && parts[1] == "change_nick" {
		if err := h.orderService.RequestNicknameChange(ctx, user); err != nil {
			h.reply(cb.Message.Chat.ID, "❌Не удалось переключиться на ввод ника.", 0)
			return
		}
		h.reply(cb.Message.Chat.ID, "Отправьте ваш полный игровой ник одним сообщением на английском языке.", 0)
		return
	}

	if len(parts) == 3 && parts[1] == "delete" {
		orderID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			h.reply(cb.Message.Chat.ID, "Некорректный ID заказа.", 0)
			return
		}

		order, err := h.orderService.GetMyActiveOrder(ctx, user.TelegramID)
		if err != nil {
			if err == domain.ErrOrderNotFound {
				h.reply(cb.Message.Chat.ID, "У вас нет активного заказа.", 0)
				return
			}
			h.reply(cb.Message.Chat.ID, "Ошибка, попробуйте позже.", 0)
			return
		}

		if order.ID != orderID {
			h.reply(cb.Message.Chat.ID, "Можно удалить только свой текущий заказ.", 0)
			return
		}

		err = h.orderService.CancelOwnOrder(ctx, user.TelegramID)
		if err != nil {
			switch err {
			case domain.ErrOrderInvalidStatus:
				h.reply(cb.Message.Chat.ID, "❌Заказ уже в крафте или готов. Удалить его нельзя.", 0)
			default:
				h.reply(cb.Message.Chat.ID, "❌Не удалось удалить заказ.", 0)
			}
			return
		}

		h.reply(cb.Message.Chat.ID, "❌Заказ удален. Если оформите новый — он встанет в конец очереди.", 0)
		return
	}
}

func (h *Handler) handleAdminCallback(ctx context.Context, user *domain.User, cb *tgbotapi.CallbackQuery, parts []string) {
	if !user.IsAdmin {
		h.reply(cb.Message.Chat.ID, "Нет доступа.", 0)
		return
	}

	if len(parts) == 4 && parts[1] == "queue" {
		switch parts[2] {
		case "view":
			switch parts[3] {
			case "potions":
				h.sendQueuePotions(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			case "scrolls":
				h.sendQueueScrolls(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			}
		case "text":
			switch parts[3] {
			case "all":
				h.sendQueueTextAll(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			case "potions":
				h.sendQueueTextPotions(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			case "scrolls":
				h.sendQueueTextScrolls(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			}
		}
	}

	if len(parts) == 4 && parts[1] == "progress" {
		switch parts[2] {
		case "view":
			switch parts[3] {
			case "all":
				h.sendProgress(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			case "potions":
				h.sendProgressPotions(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			case "scrolls":
				h.sendProgressScrolls(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			}

		case "text":
			switch parts[3] {
			case "all":
				h.sendProgressTextAll(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			case "potions":
				h.sendProgressTextPotions(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			case "scrolls":
				h.sendProgressTextScrolls(ctx, cb.Message.Chat.ID, user.TelegramID)
				return
			}
		}
	}

	if len(parts) == 2 && parts[1] == "create" {
		m := tgbotapi.NewMessage(cb.Message.Chat.ID, "Выберите рецепт для одиночного заказа.")
		m.ReplyMarkup = recipesKeyboard(h.orderService.Recipes(), "admin:create:recipe:")
		_, _ = h.bot.Send(m)
		return
	}
	if len(parts) == 2 && parts[1] == "bulk" {
		m := tgbotapi.NewMessage(cb.Message.Chat.ID, "Выберите рецепт для массового заказа.")
		m.ReplyMarkup = recipesKeyboard(h.orderService.Recipes(), "admin:bulk:recipe:")
		_, _ = h.bot.Send(m)
		return
	}

	if len(parts) == 3 && parts[1] == "craft" {
		switch parts[2] {
		case "open":
			if err := h.adminService.SetCraftOpen(ctx, user.TelegramID, true); err != nil {
				h.reply(cb.Message.Chat.ID, "❌Не удалось открыть крафт.", 0)
				return
			}
			h.reply(cb.Message.Chat.ID, "✅Крафт открыт.", 0)
		case "close":
			if err := h.adminService.SetCraftOpen(ctx, user.TelegramID, false); err != nil {
				h.reply(cb.Message.Chat.ID, "❌Не удалось закрыть крафт.", 0)
				return
			}
			h.reply(cb.Message.Chat.ID, "❌Крафт закрыт.", 0)
		}
		return
	}

	if len(parts) == 4 && parts[1] == "create" && parts[2] == "recipe" {
		if err := h.adminService.BeginAdminSingleOrder(ctx, user.ID, user.TelegramID, parts[3]); err != nil {
			h.reply(cb.Message.Chat.ID, "❌Не удалось начать создание заказа.", 0)
			return
		}
		h.reply(cb.Message.Chat.ID, "Отправьте ваш полный игровой ник одним сообщением на английском языке.", 0)
		return
	}

	if len(parts) == 4 && parts[1] == "bulk" && parts[2] == "recipe" {
		if err := h.adminService.BeginAdminBulkOrder(ctx, user.ID, user.TelegramID, parts[3]); err != nil {
			h.reply(cb.Message.Chat.ID, "❌Не удалось начать массовое создание.", 0)
			return
		}
		h.reply(cb.Message.Chat.ID, "📦Отправьте полные игровые ники списком, каждый с новой строки, на английском языке.", 0)
		return
	}

	if len(parts) == 5 && parts[1] == "queue" {
		section := parts[2]

		orderID, err := strconv.ParseInt(parts[4], 10, 64)
		if err != nil {
			return
		}

		var opErr error
		switch parts[3] {
		case "up":
			opErr = h.adminService.MoveUp(ctx, user.TelegramID, orderID)
		case "down":
			opErr = h.adminService.MoveDown(ctx, user.TelegramID, orderID)
		case "head":
			opErr = h.adminService.MoveToHead(ctx, user.TelegramID, orderID)
		case "tail":
			opErr = h.adminService.MoveToTail(ctx, user.TelegramID, orderID)
		}
		if opErr != nil {
			h.reply(cb.Message.Chat.ID, "❌Не удалось изменить очередь.", 0)
			return
		}

		h.reply(cb.Message.Chat.ID, "Очередь обновлена.", 0)

		switch section {
		case "potions":
			h.sendQueuePotions(ctx, cb.Message.Chat.ID, user.TelegramID)
		case "scrolls":
			h.sendQueueScrolls(ctx, cb.Message.Chat.ID, user.TelegramID)
		default:
			h.sendQueueTextAll(ctx, cb.Message.Chat.ID, user.TelegramID)
		}
		return
	}

	if len(parts) == 5 && parts[1] == "order" {
		section := parts[2]

		orderID, err := strconv.ParseInt(parts[4], 10, 64)
		if err != nil {
			return
		}

		switch parts[3] {
		case "start":
			order, err := h.adminService.StartCraft(ctx, user.TelegramID, orderID)
			if err != nil {
				h.reply(cb.Message.Chat.ID, "❌Не удалось поставить в крафт.", 0)
				return
			}
			if err := h.notifyService.SendCraftStarted(ctx, order); err != nil {
				h.log.Error("send craft started", zap.Error(err), zap.Int64("order_id", order.ID))
			}
			h.reply(cb.Message.Chat.ID, "✅Заказ поставлен на крафт.", 0)

			switch section {
			case "potions":
				h.sendQueuePotions(ctx, cb.Message.Chat.ID, user.TelegramID)
			case "scrolls":
				h.sendQueueScrolls(ctx, cb.Message.Chat.ID, user.TelegramID)
			default:
				h.sendQueueTextAll(ctx, cb.Message.Chat.ID, user.TelegramID)
			}
			return

		case "cancel":
			if err := h.adminService.CancelOrder(ctx, user.TelegramID, orderID); err != nil {
				h.reply(cb.Message.Chat.ID, "❌Не удалось отменить заказ.", 0)
				return
			}
			h.reply(cb.Message.Chat.ID, "Заказ отменен.", 0)

			switch section {
			case "potions":
				h.sendQueuePotions(ctx, cb.Message.Chat.ID, user.TelegramID)
			case "scrolls":
				h.sendQueueScrolls(ctx, cb.Message.Chat.ID, user.TelegramID)
			default:
				h.sendQueueTextAll(ctx, cb.Message.Chat.ID, user.TelegramID)
			}
			return

		case "complete":
			if err := h.adminService.CompleteOrder(ctx, user.TelegramID, orderID); err != nil {
				h.reply(cb.Message.Chat.ID, "❌Не удалось завершить заказ.", 0)
				return
			}
			h.reply(cb.Message.Chat.ID, "Заказ отмечен как выданный.", 0)
			h.sendProgress(ctx, cb.Message.Chat.ID, user.TelegramID)
			return
		}
	}
}

func (h *Handler) createAdminSingleByState(ctx context.Context, user *domain.User, nickname string) (*domain.Order, error) {
	state, err := h.orderService.GetUserState(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if state.PendingRecipeKey == nil {
		return nil, domain.ErrBadInput
	}
	return h.adminService.CreateOrderByNickname(ctx, user.TelegramID, *state.PendingRecipeKey, nickname)
}

func (h *Handler) createAdminBulkByState(ctx context.Context, user *domain.User, raw string) ([]domain.Order, []string, error) {
	state, err := h.orderService.GetUserState(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}
	if state.PendingRecipeKey == nil {
		return nil, nil, domain.ErrBadInput
	}

	lines := strings.Split(raw, "\n")
	nicknames := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			nicknames = append(nicknames, line)
		}
	}

	return h.adminService.CreateBulkOrdersByNicknames(ctx, user.TelegramID, *state.PendingRecipeKey, nicknames)
}

func (h *Handler) sendProgress(ctx context.Context, chatID, adminTelegramID int64) {
	items, err := h.adminService.InProgress(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось загрузить список.", 0)
		return
	}
	if len(items) == 0 {
		h.reply(chatID, "Нет заказов в крафте/готовых.", 0)
		return
	}
	for _, o := range items {
		msg := tgbotapi.NewMessage(chatID, formatOrder(&o, h.loc))
		msg.ReplyMarkup = adminProgressItemKeyboard(o.ID)
		_, _ = h.bot.Send(msg)
	}
}

func (h *Handler) reply(chatID int64, text string, replyTo int) {
	msg := tgbotapi.NewMessage(chatID, text)
	if replyTo != 0 {
		msg.ReplyToMessageID = replyTo
	}
	_, _ = h.bot.Send(msg)
}

func (h *Handler) replyWithKeyboard(chatID int64, text string, kb tgbotapi.ReplyKeyboardMarkup, replyTo int) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = kb
	if replyTo != 0 {
		msg.ReplyToMessageID = replyTo
	}
	_, _ = h.bot.Send(msg)
}

func (h *Handler) answerCallback(callbackID, text string) {
	cfg := tgbotapi.NewCallback(callbackID, text)
	_, _ = h.bot.Request(cfg)
}

func (h *Handler) sendQueuePotions(ctx context.Context, chatID, adminTelegramID int64) {
	items, err := h.adminService.QueuePotions(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось загрузить очередь по банкам.", 0)
		return
	}
	if len(items) == 0 {
		h.reply(chatID, "❌Очередь по банкам пустая.", 0)
		return
	}
	for _, o := range items {
		msg := tgbotapi.NewMessage(chatID, formatQueueOrder(&o, h.loc))
		msg.ReplyMarkup = adminQueueItemKeyboard("potions", o.ID)
		_, _ = h.bot.Send(msg)
	}
}

func (h *Handler) sendQueueScrolls(ctx context.Context, chatID, adminTelegramID int64) {
	items, err := h.adminService.QueueScrolls(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось загрузить очередь по свиткам.", 0)
		return
	}
	if len(items) == 0 {
		h.reply(chatID, "❌Очередь по свиткам пустая.", 0)
		return
	}
	for _, o := range items {
		msg := tgbotapi.NewMessage(chatID, formatQueueOrder(&o, h.loc))
		msg.ReplyMarkup = adminQueueItemKeyboard("scrolls", o.ID)
		_, _ = h.bot.Send(msg)
	}
}

func (h *Handler) sendQueueTextAll(ctx context.Context, chatID, adminTelegramID int64) {
	text, err := h.adminService.QueueText(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось собрать очередь текстом.", 0)
		return
	}
	h.sendLongText(chatID, text)
}

func (h *Handler) sendQueueTextPotions(ctx context.Context, chatID, adminTelegramID int64) {
	text, err := h.adminService.QueuePotionsText(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось собрать очередь по банкам текстом.", 0)
		return
	}
	h.sendLongText(chatID, text)
}

func (h *Handler) sendQueueTextScrolls(ctx context.Context, chatID, adminTelegramID int64) {
	text, err := h.adminService.QueueScrollsText(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось собрать очередь по свиткам текстом.", 0)
		return
	}
	h.sendLongText(chatID, text)
}

func (h *Handler) sendProgressTextAll(ctx context.Context, chatID, adminTelegramID int64) {
	text, err := h.adminService.InProgressText(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось собрать список в крафте текстом.", 0)
		return
	}
	h.sendLongText(chatID, text)
}

func (h *Handler) sendProgressTextPotions(ctx context.Context, chatID, adminTelegramID int64) {
	text, err := h.adminService.InProgressPotionsText(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось собрать список банков в крафте текстом.", 0)
		return
	}
	h.sendLongText(chatID, text)
}

func (h *Handler) sendProgressTextScrolls(ctx context.Context, chatID, adminTelegramID int64) {
	text, err := h.adminService.InProgressScrollsText(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось собрать список свитков в крафте текстом.", 0)
		return
	}
	h.sendLongText(chatID, text)
}

func (h *Handler) sendLongText(chatID int64, text string) {
	const maxLen = 3500
	for len(text) > maxLen {
		part := text[:maxLen]
		cut := strings.LastIndex(part, "\n")
		if cut <= 0 {
			cut = maxLen
		}
		h.reply(chatID, text[:cut], 0)
		text = text[cut:]
		text = strings.TrimLeft(text, "\n")
	}
	if strings.TrimSpace(text) != "" {
		h.reply(chatID, text, 0)
	}
}

func recipeByKey(recipes []domain.Recipe, key string) (domain.Recipe, bool) {
	for _, r := range recipes {
		if r.Key == key {
			return r, true
		}
	}
	return domain.Recipe{}, false
}

func formatOrder(o *domain.Order, loc *time.Location) string {
	var extra strings.Builder
	if o.QueuePos != nil {
		extra.WriteString(fmt.Sprintf("📈Место в очереди: %d\n", *o.QueuePos))
	}
	if o.ReadyAt != nil {
		extra.WriteString(fmt.Sprintf("✅ Готово: %s\n", o.ReadyAt.In(loc).Format("02.01 15:04")))
	}
	if o.PickupDeadlineAt != nil {
		extra.WriteString(fmt.Sprintf("📅Забрать до: %s\n", o.PickupDeadlineAt.In(loc).Format("02.01 15:04")))
	}

	tgText := "нет"
	if o.TelegramID != nil {
		tgText = strconv.FormatInt(*o.TelegramID, 10)
	}

	return fmt.Sprintf(
		"📍Заказ #%d\n❗️Статус: %s\n📜Рецепт: %s ×%d\n📝Ник: %s\n📝TG: %s\n%s📅Создан: %s",
		o.ID,
		RuStatus(o.Status),
		o.RecipeName,
		o.Qty,
		o.Nickname,
		tgText,
		extra.String(),
		o.CreatedAt.In(loc).Format("02.01 15:04"),
	)
}

func formatQueueOrder(o *domain.Order, loc *time.Location) string {
	pos := int64(0)
	if o.QueuePos != nil {
		pos = *o.QueuePos
	}

	tgText := "нет"
	if o.TelegramID != nil {
		tgText = strconv.FormatInt(*o.TelegramID, 10)
	}

	return fmt.Sprintf(
		"Очередь #%d\nЗаказ #%d\nРецепт: %s ×%d\nНик: %s\nTG: %s\nСоздан: %s",
		pos,
		o.ID,
		o.RecipeName,
		o.Qty,
		o.Nickname,
		tgText,
		o.CreatedAt.In(loc).Format("02.01 15:04"),
	)
}

func (h *Handler) sendProgressPotions(ctx context.Context, chatID, adminTelegramID int64) {
	items, err := h.adminService.InProgressPotions(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось загрузить банки в крафте.", 0)
		return
	}
	if len(items) == 0 {
		h.reply(chatID, "❌Банки в крафте отсутствуют.", 0)
		return
	}

	for _, o := range items {
		msg := tgbotapi.NewMessage(chatID, formatOrder(&o, h.loc))
		msg.ReplyMarkup = adminProgressItemKeyboard(o.ID)
		_, _ = h.bot.Send(msg)
	}
}

func (h *Handler) sendProgressScrolls(ctx context.Context, chatID, adminTelegramID int64) {
	items, err := h.adminService.InProgressScrolls(ctx, adminTelegramID)
	if err != nil {
		h.reply(chatID, "❌Не удалось загрузить свитки в крафте.", 0)
		return
	}
	if len(items) == 0 {
		h.reply(chatID, "❌Свитки в крафте отсутствуют.", 0)
		return
	}

	for _, o := range items {
		msg := tgbotapi.NewMessage(chatID, formatOrder(&o, h.loc))
		msg.ReplyMarkup = adminProgressItemKeyboard(o.ID)
		_, _ = h.bot.Send(msg)
	}
}
