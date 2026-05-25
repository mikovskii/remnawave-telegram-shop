package handler

import (
	"context"
	"fmt"
	"html"
	"strconv"
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
	existingCustomer, err := h.customerRepository.FindByTelegramId(ctx, update.Message.Chat.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}

	if existingCustomer == nil {
		existingCustomer, err = h.customerRepository.Create(ctxWithTime, &database.Customer{
			TelegramID: update.Message.Chat.ID,
			Language:   langCode,
		})
		if err != nil {
			slog.Error("error creating customer", "error", err)
			return
		}

		args := strings.Fields(update.Message.Text)
		if len(args) > 1 {
			arg := args[1]
			if strings.HasPrefix(arg, "ref_") {
				code := strings.TrimPrefix(arg, "ref_")
				referrerId, err := strconv.ParseInt(code, 10, 64)
				if err != nil {
					slog.Error("error parsing referrer id", "error", err)
					return
				}
				if referrerId != existingCustomer.TelegramID {
					referrerCustomer, err := h.customerRepository.FindByTelegramId(ctx, referrerId)
					if err == nil && referrerCustomer != nil {
						_, err := h.referralRepository.Create(ctx, referrerId, existingCustomer.TelegramID)
						if err != nil {
							slog.Error("error creating referral", "error", err)
							return
						}
						slog.Info("referral created", "referrerId", utils.MaskHalfInt64(referrerId), "refereeId", utils.MaskHalfInt64(existingCustomer.TelegramID))
						h.notifyReferralStarted(ctx, b, referrerCustomer, update.Message.From)
					}
				}
			}
		}
	} else {
		updates := map[string]interface{}{
			"language": langCode,
		}

		err = h.customerRepository.UpdateFields(ctx, existingCustomer.ID, updates)
		if err != nil {
			slog.Error("Error updating customer", "error", err)
			return
		}
	}

	inlineKeyboard := h.buildStartKeyboard(existingCustomer, langCode)

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
		Text: h.buildStartText(existingCustomer, langCode),
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

	_, err = b.EditMessageText(ctxWithTime, &bot.EditMessageTextParams{
		ChatID:    callback.Message.Message.Chat.ID,
		MessageID: callback.Message.Message.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
		Text: h.buildStartText(existingCustomer, langCode),
	})
	if err != nil {
		slog.Error("Error sending /start message", "error", err)
	}
}

func (h Handler) buildStartText(customer *database.Customer, langCode string) string {
	if h.hasActiveSubscription(customer) {
		return h.translation.GetText(langCode, "greeting_active")
	}

	return fmt.Sprintf(
		h.translation.GetText(langCode, "greeting"),
		h.buildPaymentMethodsText(langCode),
	)
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

	if h.hasActiveSubscription(existingCustomer) {
		inlineKeyboard = append(inlineKeyboard, h.resolveConnectButton(langCode))
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "extend_subscription_button").InlineCallback(CallbackBuy)})
		if config.GetReferralDays() > 0 {
			inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "referral_button").InlineCallback(CallbackReferral)})
		}
		if config.SupportURL() != "" {
			inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "support_button").InlineURL(config.SupportURL())})
		}
		return h.appendSecondaryStartButtons(inlineKeyboard, langCode, false)
	}

	if (existingCustomer == nil || existingCustomer.SubscriptionLink == nil) && config.TrialDays() > 0 {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "trial_button").InlineCallback(CallbackTrial)})
	}

	inlineKeyboard = append(inlineKeyboard, [][]models.InlineKeyboardButton{{h.translation.GetButton(langCode, "buy_button").InlineCallback(CallbackBuy)}}...)

	if config.SupportURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "support_button").InlineURL(config.SupportURL())})
	}

	return h.appendSecondaryStartButtons(inlineKeyboard, langCode, true)
}

func (h Handler) appendSecondaryStartButtons(inlineKeyboard [][]models.InlineKeyboardButton, langCode string, includePrimaryReferral bool) [][]models.InlineKeyboardButton {
	if config.ServerStatusURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "server_status_button").InlineURL(config.ServerStatusURL())})
	}

	if includePrimaryReferral && config.GetReferralDays() > 0 {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "referral_button").InlineCallback(CallbackReferral)})
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

func (h Handler) hasActiveSubscription(customer *database.Customer) bool {
	return customer != nil &&
		customer.SubscriptionLink != nil &&
		customer.ExpireAt != nil &&
		customer.ExpireAt.After(time.Now())
}

func (h Handler) buildPaymentMethodsText(langCode string) string {
	var methods []string
	if config.IsYookasaEnabled() || config.IsPlategaCardsEnabled() || config.IsPlategaAcquiringEnabled() || config.IsPlategaWorldwideEnabled() {
		methods = append(methods, h.translation.GetText(langCode, "payment_method_cards"))
	}
	if config.IsPlategaSBPEnabled() {
		methods = append(methods, h.translation.GetText(langCode, "payment_method_sbp"))
	}
	if config.IsCryptoPayEnabled() || config.IsPlategaCryptoEnabled() {
		methods = append(methods, h.translation.GetText(langCode, "payment_method_crypto"))
	}
	if config.IsTelegramStarsEnabled() {
		methods = append(methods, h.translation.GetText(langCode, "payment_method_stars"))
	}
	if config.GetTributePaymentUrl() != "" {
		methods = append(methods, h.translation.GetText(langCode, "payment_method_tribute"))
	}
	if len(methods) == 0 {
		return h.translation.GetText(langCode, "payment_method_default")
	}
	return strings.Join(methods, ", ")
}

func (h Handler) notifyReferralStarted(ctx context.Context, b *bot.Bot, referrer *database.Customer, referee *models.User) {
	if config.GetReferralDays() <= 0 {
		return
	}
	refereeName := h.refereeDisplayName(referee)
	text := fmt.Sprintf(
		h.translation.GetText(referrer.Language, "referral_started"),
		html.EscapeString(refereeName),
		config.GetReferralDays(),
		referralMilestoneFriends(),
	)

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    referrer.TelegramID,
		ParseMode: models.ParseModeHTML,
		Text:      text,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{h.translation.GetButton(referrer.Language, "referral_button").InlineCallback(CallbackReferral)},
		}},
	})
	if err != nil {
		slog.Error("error notifying referrer about referral start", "error", err)
	}
}

func (h Handler) refereeDisplayName(user *models.User) string {
	if user == nil {
		return h.translation.GetText("", "referral_friend_fallback")
	}
	name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	if name != "" {
		return name
	}
	if user.Username != "" {
		return "@" + user.Username
	}
	return h.translation.GetText("", "referral_friend_fallback")
}

func (h Handler) InfoCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	langCode := callback.From.LanguageCode

	var keyboard [][]models.InlineKeyboardButton

	if config.TosURL() != "" {
		keyboard = append(keyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "tos_button").InlineURL(config.TosURL())})
	}

	if config.PrivacyURL() != "" {
		keyboard = append(keyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "privacy_button").InlineURL(config.PrivacyURL())})
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{h.translation.GetButton(langCode, "back_button").InlineCallback(CallbackStart)})

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
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
