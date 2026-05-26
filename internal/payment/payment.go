package payment

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"

	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/platega"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

type PurchaseService struct {
	purchaseRepository *database.PurchaseRepository
	customerRepository *database.CustomerRepository
	provisioning       *ProvisioningService
	notifications      *NotificationService
	providers          map[database.InvoiceType]PaymentProvider
	botEventRepository *database.BotEventRepository
}

func NewPurchaseService(
	translation *translation.Manager,
	purchaseRepository *database.PurchaseRepository,
	remnawaveClient *remnawave.Client,
	customerRepository *database.CustomerRepository,
	telegramBot *bot.Bot,
	plategaClient *platega.Client,
	referralRepository *database.ReferralRepository,
	botEventRepository *database.BotEventRepository,
	periodRepository *database.SubscriptionPeriodRepository,
	cache *cache.Cache,
) *PurchaseService {
	notifications := NewNotificationService(telegramBot, translation, cache)
	return &PurchaseService{
		purchaseRepository: purchaseRepository,
		customerRepository: customerRepository,
		provisioning: NewProvisioningService(
			remnawaveClient,
			customerRepository,
			referralRepository,
			periodRepository,
		),
		notifications:      notifications,
		providers:          NewPaymentProviders(telegramBot, translation, plategaClient),
		botEventRepository: botEventRepository,
	}
}

func (s PurchaseService) CreateInvoice(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType) (url string, purchaseID int64, err error) {
	provider, ok := s.providers[invoiceType]
	if !ok {
		return "", 0, fmt.Errorf("unknown invoice type: %s", invoiceType)
	}

	purchaseID, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType: invoiceType,
		Status:      database.PurchaseStatusNew,
		Amount:      amount,
		Currency:    provider.Currency(),
		CustomerID:  customer.ID,
		Month:       months,
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	invoice, err := provider.CreateInvoice(ctx, InvoiceRequest{
		PurchaseID:  purchaseID,
		Amount:      amount,
		Months:      months,
		Customer:    customer,
		Description: utils.FormatSubscriptionDescription(months),
		ReturnURL:   config.BotURL(),
	})
	if err != nil {
		slog.Error("Error creating invoice", "error", err, "invoice_type", invoiceType)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"status": database.PurchaseStatusPending,
	}
	if invoice.ExternalID != "" {
		updates["platega_id"] = invoice.ExternalID
	}
	if invoice.URL != "" && invoiceType != database.InvoiceTypeTelegram {
		updates["platega_url"] = invoice.URL
	}
	if err := s.purchaseRepository.UpdateFields(ctx, purchaseID, updates); err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoice.URL, purchaseID, nil
}

func (s PurchaseService) ProcessPurchaseById(ctx context.Context, purchaseID int64) error {
	return s.purchaseRepository.WithFulfillmentLock(ctx, purchaseID, func(lockCtx context.Context) error {
		return s.processPurchaseByIdLocked(lockCtx, purchaseID)
	})
}

func (s PurchaseService) processPurchaseByIdLocked(ctx context.Context, purchaseID int64) error {
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseID)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase %s not found", utils.MaskHalfInt64(purchaseID))
	}
	if purchase.FulfilledAt != nil {
		slog.Info("purchase already fulfilled", "purchase_id", utils.MaskHalfInt64(purchase.ID))
		return nil
	}

	customer, err := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return fmt.Errorf("customer %s not found", utils.MaskHalfInt64(purchase.CustomerID))
	}

	s.notifications.DeleteInvoiceMessage(ctx, purchase.ID, customer.TelegramID)

	targetExpireAt, err := s.markPaid(ctx, purchase, customer)
	if err != nil {
		return err
	}

	result, err := s.provisioning.FulfillPurchase(ctx, customer, purchase, targetExpireAt)
	if err != nil {
		return err
	}

	fulfilled, err := s.purchaseRepository.MarkFulfilled(ctx, purchase.ID)
	if err != nil {
		return err
	}
	if !fulfilled {
		slog.Info("purchase fulfillment state already recorded", "purchase_id", utils.MaskHalfInt64(purchase.ID))
	}

	s.trackEvent(ctx, customer, database.EventPaymentSuccess, map[string]interface{}{
		"purchase_id":  purchase.ID,
		"month":        purchase.Month,
		"amount":       purchase.Amount,
		"currency":     purchase.Currency,
		"invoice_type": string(purchase.InvoiceType),
		"stage":        database.LifecycleStagePaid,
	})

	if result.ReferralBonus != nil {
		s.notifications.SendReferralBonus(ctx, result.ReferralBonus.Customer, result.ReferralBonus.ReferralID)
	}
	s.notifications.SendSubscriptionActivated(ctx, customer, purchase.ID)

	slog.Info("purchase processed", "purchase_id", utils.MaskHalfInt64(purchase.ID), "type", purchase.InvoiceType, "customer_id", utils.MaskHalfInt64(customer.ID))
	return nil
}

func (s PurchaseService) markPaid(ctx context.Context, purchase *database.Purchase, customer *database.Customer) (time.Time, error) {
	if purchase.Status == database.PurchaseStatusPaid && purchase.ExpireAt != nil {
		slog.Info("purchase payment state already recorded", "purchase_id", utils.MaskHalfInt64(purchase.ID))
		return *purchase.ExpireAt, nil
	}

	expireAt, err := s.provisioning.CalculatePaidExpireAt(ctx, customer.TelegramID, purchase.Month)
	if err != nil {
		return time.Time{}, err
	}
	markedPaid, err := s.purchaseRepository.MarkAsPaid(ctx, purchase.ID, expireAt)
	if err != nil {
		return time.Time{}, err
	}
	if markedPaid {
		return expireAt, nil
	}

	updatedPurchase, err := s.purchaseRepository.FindById(ctx, purchase.ID)
	if err != nil {
		return time.Time{}, err
	}
	if updatedPurchase == nil || updatedPurchase.ExpireAt == nil {
		return time.Time{}, fmt.Errorf("purchase %s paid state was not recorded", utils.MaskHalfInt64(purchase.ID))
	}
	return *updatedPurchase.ExpireAt, nil
}

func (s PurchaseService) ActivateTrial(ctx context.Context, telegramID int64) (string, error) {
	if config.TrialDays() == 0 {
		return "", nil
	}
	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return "", err
	}
	if customer == nil {
		return "", fmt.Errorf("customer %d not found", telegramID)
	}
	return s.provisioning.ActivateTrial(ctx, customer)
}

func (s PurchaseService) CancelPayment(ctx context.Context, purchaseID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	purchase, err := s.purchaseRepository.FindById(ctx, purchaseID)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase %s not found", utils.MaskHalfInt64(purchaseID))
	}

	if err := s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{
		"status": database.PurchaseStatusCancel,
	}); err != nil {
		return err
	}

	customer, err := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil || purchase.FailedNotifiedAt != nil {
		return nil
	}

	claimed, err := s.purchaseRepository.ClaimFailedNotification(ctx, purchase.ID, time.Now().Add(-15*time.Minute))
	if err != nil {
		slog.Error("claim payment failed notification", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
		return nil
	}
	if !claimed {
		return nil
	}
	if err := s.notifications.SendPaymentFailed(ctx, customer, purchase); err != nil {
		slog.Error("send payment failed notification", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
		return nil
	}
	if _, err := s.purchaseRepository.MarkFailedNotifiedIfUnset(ctx, purchase.ID); err != nil {
		slog.Error("mark payment failed notification", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
	}
	return nil
}
