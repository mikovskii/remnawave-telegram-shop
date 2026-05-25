package notification

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
)

type invoicePurchaseRepository interface {
	FindAbandonedInvoices(ctx context.Context, olderThan time.Time) (*[]database.Purchase, error)
	MarkAbandonedNotifiedIfUnset(ctx context.Context, purchaseID int64) (bool, error)
	MarkFailedNotifiedIfUnset(ctx context.Context, purchaseID int64) (bool, error)
	UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error
}

type invoiceCustomerRepository interface {
	FindById(ctx context.Context, id int64) (*database.Customer, error)
}

type InvoiceService struct {
	purchaseRepository invoicePurchaseRepository
	customerRepository invoiceCustomerRepository
	telegramBot        *bot.Bot
	tm                 *translation.Manager
}

func NewInvoiceService(purchaseRepository invoicePurchaseRepository, customerRepository invoiceCustomerRepository, telegramBot *bot.Bot, tm *translation.Manager) *InvoiceService {
	return &InvoiceService{
		purchaseRepository: purchaseRepository,
		customerRepository: customerRepository,
		telegramBot:        telegramBot,
		tm:                 tm,
	}
}

func (s *InvoiceService) ProcessAbandonedInvoices() error {
	ctx := context.Background()
	olderThan := time.Now().Add(-15 * time.Minute)
	purchases, err := s.purchaseRepository.FindAbandonedInvoices(ctx, olderThan)
	if err != nil {
		return err
	}

	for _, purchase := range *purchases {
		customer, err := s.customerRepository.FindById(ctx, purchase.CustomerID)
		if err != nil {
			slog.Error("failed to find customer for abandoned invoice", "purchase_id", purchase.ID, "error", err)
			continue
		}
		if customer == nil {
			continue
		}
		claimed, err := s.purchaseRepository.MarkAbandonedNotifiedIfUnset(ctx, purchase.ID)
		if err != nil {
			slog.Error("failed to mark abandoned invoice notification", "purchase_id", purchase.ID, "error", err)
			continue
		}
		if !claimed {
			continue
		}
		if err := s.sendInvoiceAbandoned(ctx, *customer, purchase); err != nil {
			slog.Error("failed to send abandoned invoice notification", "purchase_id", purchase.ID, "error", err)
			continue
		}
	}

	return nil
}

func (s *InvoiceService) NotifyPaymentFailed(ctx context.Context, purchaseID int64) error {
	purchase, ok, err := s.loadPurchaseCustomer(ctx, purchaseID)
	if err != nil || !ok {
		return err
	}
	if purchase.FailedNotifiedAt != nil {
		return nil
	}
	customer, err := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if err != nil || customer == nil {
		return err
	}
	claimed, err := s.purchaseRepository.MarkFailedNotifiedIfUnset(ctx, purchase.ID)
	if err != nil || !claimed {
		return err
	}
	return s.sendPaymentFailed(ctx, *customer, *purchase)
}

func (s *InvoiceService) loadPurchaseCustomer(ctx context.Context, purchaseID int64) (*database.Purchase, bool, error) {
	type finder interface {
		FindById(ctx context.Context, id int64) (*database.Purchase, error)
	}
	repo, ok := s.purchaseRepository.(finder)
	if !ok {
		return nil, false, fmt.Errorf("purchase repository does not support FindById")
	}
	purchase, err := repo.FindById(ctx, purchaseID)
	if err != nil || purchase == nil {
		return purchase, false, err
	}
	return purchase, true, nil
}

func (s *InvoiceService) sendInvoiceAbandoned(ctx context.Context, customer database.Customer, purchase database.Purchase) error {
	return s.sendInvoiceNotification(ctx, customer, purchase, "invoice_abandoned")
}

func (s *InvoiceService) sendPaymentFailed(ctx context.Context, customer database.Customer, purchase database.Purchase) error {
	return s.sendInvoiceNotification(ctx, customer, purchase, "payment_failed")
}

func (s *InvoiceService) sendInvoiceNotification(ctx context.Context, customer database.Customer, purchase database.Purchase, textKey string) error {
	if s.telegramBot == nil || s.tm == nil {
		return nil
	}
	lang := customer.Language
	paymentURL := invoicePaymentURL(purchase)
	keyboard := [][]models.InlineKeyboardButton{}
	if paymentURL != "" {
		keyboard = append(keyboard, []models.InlineKeyboardButton{s.tm.GetButton(lang, "pay_button").InlineURL(paymentURL)})
	}
	keyboard = append(keyboard, []models.InlineKeyboardButton{s.tm.GetButton(lang, "renew_subscription_button").InlineCallback("buy")})

	text := fmt.Sprintf(s.tm.GetText(lang, textKey), purchase.Month, purchase.Amount, purchase.Currency)
	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      customer.TelegramID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: keyboard},
	})
	return err
}

func invoicePaymentURL(purchase database.Purchase) string {
	switch {
	case purchase.CryptoInvoiceLink != nil:
		return *purchase.CryptoInvoiceLink
	case purchase.YookasaURL != nil:
		return *purchase.YookasaURL
	case purchase.PlategaURL != nil:
		return *purchase.PlategaURL
	default:
		return ""
	}
}
