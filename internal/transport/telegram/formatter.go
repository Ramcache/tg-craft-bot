package telegram

import "tg-craft-bot/internal/domain"

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
