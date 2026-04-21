package telegram

import (
	"fmt"
	"strings"

	"tg-craft-bot/internal/domain"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func mainKeyboard(isAdmin bool) tgbotapi.ReplyKeyboardMarkup {
	rows := [][]tgbotapi.KeyboardButton{
		{
			tgbotapi.NewKeyboardButton("📜Заказать крафт"),
			tgbotapi.NewKeyboardButton("📜Мой заказ"),
		},
	}
	if isAdmin {
		rows = append(rows, tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🔐Админ-панель"),
		))
	}

	kb := tgbotapi.NewReplyKeyboard(rows...)
	kb.ResizeKeyboard = true
	return kb
}

func recipesKeyboard(recipes []domain.Recipe, prefix string) tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(recipes)+4)

	var potions []domain.Recipe
	var scrolls []domain.Recipe
	var others []domain.Recipe

	for _, r := range recipes {
		switch r.Type {
		case domain.RecipePotion, domain.RecipeElixir:
			potions = append(potions, r)
		case domain.RecipeScroll:
			scrolls = append(scrolls, r)
		default:
			others = append(others, r)
		}
	}

	if len(potions) > 0 {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("──── БАНКИ ────", "noop"),
		))
		for _, r := range potions {
			label := r.MenuLabel
			if strings.TrimSpace(label) == "" {
				label = fmt.Sprintf("%s ×%d", r.Name, r.DefaultQty)
			}

			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(label, prefix+r.Key),
			))
		}
	}

	if len(scrolls) > 0 {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("──── СВИТКИ ────", "noop"),
		))
		for _, r := range scrolls {
			label := r.MenuLabel
			if strings.TrimSpace(label) == "" {
				label = fmt.Sprintf("%s ×%d", r.Name, r.DefaultQty)
			}

			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(label, prefix+r.Key),
			))
		}
	}

	if len(others) > 0 {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("──── ДРУГОЕ ────", "noop"),
		))
		for _, r := range others {
			label := r.MenuLabel
			if strings.TrimSpace(label) == "" {
				label = fmt.Sprintf("%s ×%d", r.Name, r.DefaultQty)
			}

			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(label, prefix+r.Key),
			))
		}
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌Назад или Отменить", "flow:cancel"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func confirmOrderKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅Подтвердить", "order:confirm"),
			tgbotapi.NewInlineKeyboardButtonData("Сменить ник", "order:change_nick"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌Назад или Отменить", "flow:cancel"),
		),
	)
}

func adminMenuKeyboard(isOpen bool) tgbotapi.InlineKeyboardMarkup {
	toggleLabel := "❌Закрыть крафт"
	toggleData := "admin:craft:close"
	if !isOpen {
		toggleLabel = "✅Открыть крафт"
		toggleData = "admin:craft:open"
	}

	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗞Очередь: свитки", "admin:queue:view:scrolls"),
			tgbotapi.NewInlineKeyboardButtonData("🧴Очередь: банки", "admin:queue:view:potions"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗞Очередь текстом: свитки", "admin:queue:text:scrolls"),
			tgbotapi.NewInlineKeyboardButtonData("🧴Очередь текстом: банки", "admin:queue:text:potions"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📍Очередь текстом: всё", "admin:queue:text:all"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗞В крафте: свитки", "admin:progress:view:scrolls"),
			tgbotapi.NewInlineKeyboardButtonData("🧴В крафте: банки", "admin:progress:view:potions"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗞В крафте текстом: свитки", "admin:progress:text:scrolls"),
			tgbotapi.NewInlineKeyboardButtonData("🧴В крафте текстом: банки", "admin:progress:text:potions"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📍В крафте текстом: всё", "admin:progress:text:all"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✉️Создать заказ", "admin:create"),
			tgbotapi.NewInlineKeyboardButtonData("📦Массовый заказ", "admin:bulk"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(toggleLabel, toggleData),
		),
	)
}

func adminQueueItemKeyboard(section string, orderID int64) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬆️", fmt.Sprintf("admin:queue:%s:up:%d", section, orderID)),
			tgbotapi.NewInlineKeyboardButtonData("⬇️", fmt.Sprintf("admin:queue:%s:down:%d", section, orderID)),
			tgbotapi.NewInlineKeyboardButtonData("⏫", fmt.Sprintf("admin:queue:%s:head:%d", section, orderID)),
			tgbotapi.NewInlineKeyboardButtonData("⏬", fmt.Sprintf("admin:queue:%s:tail:%d", section, orderID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Поставить в крафт", fmt.Sprintf("admin:order:%s:start:%d", section, orderID)),
			tgbotapi.NewInlineKeyboardButtonData("Отменить", fmt.Sprintf("admin:order:%s:cancel:%d", section, orderID)),
		),
	)
}

func adminProgressItemKeyboard(orderID int64) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅Завершить", fmt.Sprintf("admin:order:complete:%d", orderID)),
			tgbotapi.NewInlineKeyboardButtonData("❌Отменить", fmt.Sprintf("admin:order:cancel:%d", orderID)),
		),
	)
}

func myOrderKeyboard(orderID int64) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌Удалить заказ", fmt.Sprintf("order:delete:%d", orderID)),
		),
	)
}
