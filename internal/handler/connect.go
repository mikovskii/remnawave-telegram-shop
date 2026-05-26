package handler

import (
	"context"
	"fmt"
	"remnawave-tg-shop-bot/internal/config"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

func (h Handler) ConnectCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	customer, err := h.customerRepository.FindByTelegramId(ctx, update.Message.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "telegramId", utils.MaskHalfInt64(update.Message.Chat.ID), "error", err)
		return
	}

	langCode := update.Message.From.LanguageCode
	h.trackEvent(ctx, customer, update.Message.Chat.ID, database.EventConnectOpen, map[string]interface{}{
		"source":               "command",
		"language":             langCode,
		"has_active_access":    customer.ExpireAt != nil && customer.ExpireAt.After(time.Now()),
		"has_subscription":     customer.SubscriptionLink != nil,
		"mini_app_enabled":     config.GetMiniAppURL() != "",
		"web_app_link_enabled": config.IsWepAppLinkEnabled(),
	})

	bd := h.translation.GetButton(langCode, "connect_button")
	var markup [][]models.InlineKeyboardButton
	if config.GetMiniAppURL() != "" {
		markup = append(markup, []models.InlineKeyboardButton{bd.InlineWebApp(config.GetMiniAppURL())})
	} else if config.IsWepAppLinkEnabled() {
		if customer.SubscriptionLink != nil && customer.ExpireAt.After(time.Now()) {
			markup = append(markup, []models.InlineKeyboardButton{bd.InlineWebApp(*customer.SubscriptionLink)})
		}
	}
	markup = append(markup, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "back_button").InlineCallback(CallbackStart)})

	isDisabled := true
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      buildConnectText(customer, langCode),
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: markup,
		},
	})

	if err != nil {
		slog.Error("Error sending connect message", "error", err)
	}
}

func (h Handler) ConnectCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message

	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "telegramId", utils.MaskHalfInt64(callback.Chat.ID), "error", err)
		return
	}

	langCode := update.CallbackQuery.From.LanguageCode
	h.trackEvent(ctx, customer, callback.Chat.ID, database.EventConnectOpen, map[string]interface{}{
		"source":               "callback",
		"language":             langCode,
		"has_active_access":    customer.ExpireAt != nil && customer.ExpireAt.After(time.Now()),
		"has_subscription":     customer.SubscriptionLink != nil,
		"mini_app_enabled":     config.GetMiniAppURL() != "",
		"web_app_link_enabled": config.IsWepAppLinkEnabled(),
	})

	cbd := h.translation.GetButton(langCode, "connect_button")
	var markup [][]models.InlineKeyboardButton
	if config.GetMiniAppURL() != "" {
		markup = append(markup, []models.InlineKeyboardButton{cbd.InlineWebApp(config.GetMiniAppURL())})
	} else if config.IsWepAppLinkEnabled() {
		if customer.SubscriptionLink != nil && customer.ExpireAt.After(time.Now()) {
			markup = append(markup, []models.InlineKeyboardButton{cbd.InlineWebApp(*customer.SubscriptionLink)})
		}
	}
	markup = append(markup, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "back_button").InlineCallback(CallbackStart)})

	isDisabled := true
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		Text:      buildConnectText(customer, langCode),
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: markup,
		},
	})

	if err != nil {
		slog.Error("Error sending connect message", "error", err)
	}
}

func buildConnectText(customer *database.Customer, langCode string) string {
	var info strings.Builder

	tm := translation.GetInstance()

	if customer.ExpireAt != nil {
		currentTime := time.Now()

		if currentTime.Before(*customer.ExpireAt) {
			formattedDate := customer.ExpireAt.Format("02.01.2006 15:04")

			subscriptionActiveText := tm.GetText(langCode, "subscription_active")
			info.WriteString(fmt.Sprintf(subscriptionActiveText, formattedDate))
			info.WriteString("\n\n")
			info.WriteString(tm.GetText(langCode, "connect_hint"))

			if customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
				if config.GetMiniAppURL() == "" && !config.IsWepAppLinkEnabled() {
					subscriptionLinkText := tm.GetText(langCode, "subscription_link")
					info.WriteString(fmt.Sprintf(subscriptionLinkText, *customer.SubscriptionLink))
				}
			}
		} else {
			noSubscriptionText := tm.GetText(langCode, "no_subscription")
			info.WriteString(noSubscriptionText)
		}
	} else {
		noSubscriptionText := tm.GetText(langCode, "no_subscription")
		info.WriteString(noSubscriptionText)
	}

	return info.String()
}
