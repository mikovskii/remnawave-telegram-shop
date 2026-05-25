package handler

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

func (h Handler) StartCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	langCode := update.Message.From.LanguageCode
	startPayload := ""
	parts := strings.SplitN(update.Message.Text, " ", 2)
	if len(parts) == 2 {
		startPayload = strings.TrimSpace(parts[1])
	}

	existingCustomer, err := h.customerRepository.FindByTelegramId(ctx, update.Message.Chat.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}

	if existingCustomer == nil {
		attribution := parseStartPayload(startPayload)
		existingCustomer, err = h.customerRepository.Create(ctxWithTime, &database.Customer{
			TelegramID: update.Message.Chat.ID,
			Language:   langCode,
		})
		if err != nil {
			slog.Error("error creating customer", "error", err)
			return
		}

		if referrerId, ok := attribution["referrer_telegram_id"].(int64); ok {
			_, err = h.customerRepository.FindByTelegramId(ctx, referrerId)
			if err == nil {
				_, err := h.referralRepository.Create(ctx, referrerId, existingCustomer.TelegramID)
				if err != nil {
					slog.Error("error creating referral", "error", err)
					return
				}
				slog.Info("referral created", "referrerId", utils.MaskHalfInt64(referrerId), "refereeId", utils.MaskHalfInt64(existingCustomer.TelegramID))
			}
		}

		updates := map[string]interface{}{
			"first_start_payload": startPayload,
			"last_seen_at":        time.Now(),
			"lifecycle_stage":     database.LifecycleStageLead,
			"lead_score":          existingCustomer.LeadScore + 1,
		}
		if source, ok := attribution["source"].(string); ok {
			updates["source"] = source
		}
		if medium, ok := attribution["medium"].(string); ok {
			updates["medium"] = medium
		}
		if campaign, ok := attribution["campaign"].(string); ok {
			updates["campaign"] = campaign
		}
		if referrerTelegramID, ok := attribution["referrer_telegram_id"].(int64); ok {
			updates["referrer_telegram_id"] = referrerTelegramID
		}
		if err := h.customerRepository.UpdateFields(ctx, existingCustomer.ID, updates); err != nil {
			slog.Error("Error updating customer attribution", "error", err)
		} else if refreshedCustomer, err := h.customerRepository.FindByTelegramId(ctx, update.Message.Chat.ID); err == nil {
			existingCustomer = refreshedCustomer
		}

		metadata := map[string]interface{}{
			"new_customer": true,
			"payload":      startPayload,
			"language":     langCode,
			"stage":        database.LifecycleStageLead,
		}
		for key, value := range attribution {
			metadata[key] = value
		}
		h.trackEvent(ctx, existingCustomer, update.Message.Chat.ID, database.EventStart, metadata)
	} else {
		updates := map[string]interface{}{
			"language":     langCode,
			"last_seen_at": time.Now(),
			"lead_score":   existingCustomer.LeadScore + 1,
		}

		err = h.customerRepository.UpdateFields(ctx, existingCustomer.ID, updates)
		if err != nil {
			slog.Error("Error updating customer", "error", err)
			return
		}
		existingCustomer.LeadScore++
		h.trackEvent(ctx, existingCustomer, update.Message.Chat.ID, database.EventStart, map[string]interface{}{
			"new_customer": false,
			"payload":      startPayload,
			"language":     langCode,
		})
	}

	inlineKeyboard := h.buildStartKeyboard(existingCustomer, langCode)
	h.trackEvent(ctxWithTime, existingCustomer, update.Message.Chat.ID, database.EventStartMenuView, map[string]interface{}{
		"source":   "command",
		"language": langCode,
	})

	m, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "🧹",
		ReplyMarkup: models.ReplyKeyboardRemove{
			RemoveKeyboard: true,
		},
	})

	if err != nil {
		slog.Error("Error sending removing reply keyboard", "error", err)
		return
	}

	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.Message.Chat.ID,
		MessageID: m.ID,
	})

	if err != nil {
		slog.Error("Error deleting message", "error", err)
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
		Text: h.translation.GetText(langCode, "greeting"),
	})
	if err != nil {
		slog.Error("Error sending /start message", "error", err)
	}
}

func (h Handler) StartCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	callback := update.CallbackQuery
	langCode := callback.From.LanguageCode

	existingCustomer, err := h.customerRepository.FindByTelegramId(ctxWithTime, callback.From.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}

	inlineKeyboard := h.buildStartKeyboard(existingCustomer, langCode)
	h.trackEvent(ctxWithTime, existingCustomer, callback.From.ID, database.EventStartMenuView, map[string]interface{}{
		"source":   "callback",
		"language": langCode,
	})

	_, err = b.EditMessageText(ctxWithTime, &bot.EditMessageTextParams{
		ChatID:    callback.Message.Message.Chat.ID,
		MessageID: callback.Message.Message.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
		Text: h.translation.GetText(langCode, "greeting"),
	})
	if err != nil {
		slog.Error("Error sending /start message", "error", err)
	}
}

func (h Handler) resolveConnectButton(lang string) []models.InlineKeyboardButton {
	bd := h.translation.GetButton(lang, "connect_button")

	if config.GetMiniAppURL() != "" {
		return []models.InlineKeyboardButton{bd.InlineWebApp(config.GetMiniAppURL())}
	}
	return []models.InlineKeyboardButton{bd.InlineCallback(CallbackConnect)}
}

func (h Handler) buildStartKeyboard(existingCustomer *database.Customer, langCode string) [][]models.InlineKeyboardButton {
	var inlineKeyboard [][]models.InlineKeyboardButton

	if existingCustomer.SubscriptionLink == nil && config.TrialDays() > 0 {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "trial_button").InlineCallback(CallbackTrial)})
	}

	inlineKeyboard = append(inlineKeyboard, [][]models.InlineKeyboardButton{{h.translation.GetButton(langCode, "buy_button").InlineCallback(CallbackBuy)}}...)

	if existingCustomer.SubscriptionLink != nil && existingCustomer.ExpireAt.After(time.Now()) {
		inlineKeyboard = append(inlineKeyboard, h.resolveConnectButton(langCode))
	}

	if config.GetReferralDays() > 0 {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "referral_button").InlineCallback(CallbackReferral)})
	}

	if config.ServerStatusURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "server_status_button").InlineURL(config.ServerStatusURL())})
	}

	if config.SupportURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "support_button").InlineURL(config.SupportURL())})
	}

	if config.FeedbackURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "feedback_button").InlineURL(config.FeedbackURL())})
	}

	if config.ChannelURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "channel_button").InlineURL(config.ChannelURL())})
	}

	if config.TosURL() != "" || config.PrivacyURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "info_button").InlineCallback(CallbackInfo)})
	}
	return inlineKeyboard
}

func (h Handler) InfoCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	langCode := callback.From.LanguageCode
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.From.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
	} else {
		h.trackEvent(ctx, customer, callback.From.ID, database.EventInfoOpen, map[string]interface{}{
			"language": langCode,
		})
	}

	var keyboard [][]models.InlineKeyboardButton

	if config.TosURL() != "" {
		keyboard = append(keyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "tos_button").InlineURL(config.TosURL())})
	}

	if config.PrivacyURL() != "" {
		keyboard = append(keyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "privacy_button").InlineURL(config.PrivacyURL())})
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "back_button").InlineCallback(CallbackStart)})

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Message.Message.Chat.ID,
		MessageID: callback.Message.Message.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: h.translation.GetText(langCode, "info_text"),
	})
	if err != nil {
		slog.Error("Error sending info message", "error", err)
	}
}
