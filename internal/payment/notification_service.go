package payment

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

type NotificationService struct {
	telegramBot *bot.Bot
	translation *translation.Manager
	cache       *cache.Cache
}

func NewNotificationService(telegramBot *bot.Bot, translation *translation.Manager, cache *cache.Cache) *NotificationService {
	return &NotificationService{
		telegramBot: telegramBot,
		translation: translation,
		cache:       cache,
	}
}

func (s *NotificationService) DeleteInvoiceMessage(ctx context.Context, purchaseID int64, telegramID int64) {
	if s.telegramBot == nil || s.cache == nil {
		return
	}
	messageID, ok := s.cache.Get(purchaseID)
	if !ok {
		return
	}
	if _, err := s.telegramBot.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: telegramID, MessageID: messageID}); err != nil {
		slog.Error("Error deleting message", "error", err)
	}
}

func (s *NotificationService) SendSubscriptionActivated(ctx context.Context, customer *database.Customer, purchaseID int64, expireAt time.Time) {
	if s.telegramBot == nil || s.translation == nil {
		return
	}
	formattedExpireAt := expireAt.Format("02.01.2006")
	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: customer.TelegramID,
		Text:   fmt.Sprintf(s.translation.GetText(customer.Language, "subscription_activated"), formattedExpireAt),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createConnectKeyboard(customer),
		},
	})
	if err != nil {
		slog.Error("Error sending subscription activation message", "error", err, "purchase_id", utils.MaskHalfInt64(purchaseID))
	}
}

func (s *NotificationService) SendReferralBonus(ctx context.Context, customer *database.Customer, referralID int64) {
	if s.telegramBot == nil || s.translation == nil || customer == nil {
		return
	}
	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    customer.TelegramID,
		ParseMode: models.ParseModeHTML,
		Text:      fmt.Sprintf(s.translation.GetText(customer.Language, "referral_bonus_granted"), config.GetReferralDays()),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createConnectKeyboard(customer),
		},
	})
	if err != nil {
		slog.Error("Error sending referral bonus message", "error", err, "referral_id", referralID)
	}
}

func (s *NotificationService) SendPaymentFailed(ctx context.Context, customer *database.Customer, purchase *database.Purchase) error {
	if s.telegramBot == nil || s.translation == nil {
		return nil
	}
	paymentURL := paymentURLForPurchase(purchase)
	keyboard := [][]models.InlineKeyboardButton{}
	if paymentURL != "" {
		keyboard = append(keyboard, []models.InlineKeyboardButton{s.translation.GetButton(customer.Language, "pay_button").InlineURL(paymentURL)})
	}
	keyboard = append(keyboard, []models.InlineKeyboardButton{s.translation.GetButton(customer.Language, "renew_subscription_button").InlineCallback("buy")})

	text := fmt.Sprintf(s.translation.GetText(customer.Language, "payment_failed"), purchase.Month, purchase.Amount, purchase.Currency)
	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      customer.TelegramID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: keyboard},
	})
	return err
}

func (s *NotificationService) createConnectKeyboard(customer *database.Customer) [][]models.InlineKeyboardButton {
	var inlineCustomerKeyboard [][]models.InlineKeyboardButton

	button := s.translation.GetButton(customer.Language, "connect_button")
	if config.GetMiniAppURL() != "" {
		inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{button.InlineWebApp(config.GetMiniAppURL())})
	} else {
		inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{button.InlineCallback("connect")})
	}

	inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
		s.translation.GetButton(customer.Language, "back_button").InlineCallback("start"),
	})
	return inlineCustomerKeyboard
}

func paymentURLForPurchase(purchase *database.Purchase) string {
	switch {
	case purchase == nil:
		return ""
	case purchase.PlategaURL != nil:
		return *purchase.PlategaURL
	default:
		return ""
	}
}
