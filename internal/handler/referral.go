package handler

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/url"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h Handler) ReferralCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	customer, err := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not found", "telegramId", update.CallbackQuery.From.ID)
		return
	}
	langCode := update.CallbackQuery.From.LanguageCode
	refLink := h.buildReferralLink(update.CallbackQuery.Message.Message.From.Username, customer.TelegramID)

	stats, err := h.referralRepository.StatsByReferrer(ctx, customer.TelegramID)
	if err != nil {
		slog.Error("error loading referral stats", "error", err)
		return
	}
	text := fmt.Sprintf(
		h.translation.GetText(langCode, "referral_text"),
		stats.TotalReferrals,
		stats.PaidReferrals,
		stats.EarnedDays,
		html.EscapeString(refLink),
	)
	callbackMessage := update.CallbackQuery.Message.Message
	disableLinkPreview := true
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:             callbackMessage.Chat.ID,
		MessageID:          callbackMessage.ID,
		Text:               text,
		ParseMode:          models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: &disableLinkPreview},
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{h.translation.GetButton(langCode, "share_referral_button").InlineURL(h.buildReferralShareLink(refLink))},
			{h.translation.GetButton(langCode, "referral_qr_button").InlineCallback(CallbackReferralQR)},
			{h.translation.GetButton(langCode, "back_button").InlineCallback(CallbackStart)},
		}},
	})
	if err != nil {
		slog.Error("Error sending referral message", "error", err)
	}
}

func (h Handler) ReferralQRCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	customer, err := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not found", "telegramId", update.CallbackQuery.From.ID)
		return
	}

	langCode := update.CallbackQuery.From.LanguageCode
	refLink := h.buildReferralLink(update.CallbackQuery.Message.Message.From.Username, customer.TelegramID)

	_, err = b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
		Photo:     &models.InputFileString{Data: h.buildReferralQRCodeURL(refLink)},
		Caption:   fmt.Sprintf(h.translation.GetText(langCode, "referral_qr_caption"), html.EscapeString(refLink)),
		ParseMode: models.ParseModeHTML,
	})
	if err != nil {
		slog.Error("Error sending referral QR code", "error", err)
	}
}

func (h Handler) buildReferralLink(botUsername string, refCode int64) string {
	return fmt.Sprintf("https://t.me/%s?start=ref_%d", botUsername, refCode)
}

func (h Handler) buildReferralShareLink(refLink string) string {
	return fmt.Sprintf("https://telegram.me/share/url?url=%s", url.QueryEscape(refLink))
}

func (h Handler) buildReferralQRCodeURL(refLink string) string {
	return fmt.Sprintf("https://api.qrserver.com/v1/create-qr-code/?size=512x512&data=%s", url.QueryEscape(refLink))
}
