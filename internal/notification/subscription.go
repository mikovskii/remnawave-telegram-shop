package notification

import (
	"context"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/translation"
	"time"
)

type customerRepository interface {
	FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]database.Customer, error)
}

type tributeRepository interface {
	FindLatestActiveTributesByCustomerIDs(ctx context.Context, customerIDs []int64) (*[]database.Purchase, error)
}

type paymentProcessor interface {
	CreatePurchase(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType) (string, int64, error)
	ProcessPurchaseById(ctx context.Context, purchaseId int64) error
}

type notificationLogger interface {
	Claim(ctx context.Context, customerID int64, notificationType string, dedupeKey string, metadata map[string]interface{}) (bool, error)
	MarkSent(ctx context.Context, customerID int64, notificationType string, dedupeKey string) error
	MarkFailed(ctx context.Context, customerID int64, notificationType string, dedupeKey string, sendErr error) error
}

type SubscriptionService struct {
	customerRepository customerRepository
	purchaseRepository tributeRepository
	paymentService     paymentProcessor
	notificationLogger notificationLogger
	telegramBot        *bot.Bot
	tm                 *translation.Manager
	notify             func(context.Context, database.Customer) error
}

func NewSubscriptionService(customerRepository customerRepository,
	purchaseRepository tributeRepository,
	paymentService paymentProcessor,
	notificationLogger notificationLogger,
	telegramBot *bot.Bot,
	tm *translation.Manager) *SubscriptionService {
	svc := &SubscriptionService{customerRepository: customerRepository, purchaseRepository: purchaseRepository, paymentService: paymentService, notificationLogger: notificationLogger, telegramBot: telegramBot, tm: tm}
	svc.notify = svc.sendNotification
	return svc
}
func (s *SubscriptionService) ProcessSubscriptionExpiration() error {
	ctx := context.Background()
	customers, err := s.getCustomersWithExpiringSubscriptions()
	if err != nil {
		slog.Error("Failed to get customers with expiring subscriptions", "error", err)
		return err
	}

	slog.Info(fmt.Sprintf("Found %d customers with expiring subscriptions", len(*customers)))
	if len(*customers) == 0 {
		return nil
	}
	now := time.Now()

	customersIds := make([]int64, len(*customers))
	for i, customer := range *customers {
		customersIds[i] = customer.ID
	}

	latestActiveTributes, err := s.purchaseRepository.FindLatestActiveTributesByCustomerIDs(ctx, customersIds)
	if err != nil {
		slog.Error("Failed to query tribute purchases", "error", err)
		return err
	}

	customerIdTributes := make(map[int64]*database.Purchase, len(*latestActiveTributes))
	for i := range *latestActiveTributes {
		p := &(*latestActiveTributes)[i]
		customerIdTributes[p.CustomerID] = p
	}

	tributesProcessed := make(map[int64]bool, len(*latestActiveTributes))

	for _, customer := range *customers {
		daysUntilExpiration := s.getDaysUntilExpiration(now, *customer.ExpireAt)

		if p, ok := customerIdTributes[customer.ID]; ok {
			if daysUntilExpiration != 1 {
				continue
			}
			_, purchaseId, err := s.paymentService.CreatePurchase(ctx, p.Amount, p.Month, &customer, database.InvoiceTypeTribute)
			if err != nil {
				slog.Error("Failed to create tribute purchase", "error", err)
				continue
			}

			err = s.paymentService.ProcessPurchaseById(ctx, purchaseId)
			if err != nil {
				slog.Error("Failed to process tribute purchase", "error", err)
				continue
			}
			slog.Info("Tribute purchase processed successfully", "purchase_id", purchaseId)
			tributesProcessed[customer.ID] = true
		}
		if _, ok := tributesProcessed[customer.ID]; ok {
			continue
		}
		if !shouldSendRenewalNotification(daysUntilExpiration) {
			continue
		}
		dedupeKey := fmt.Sprintf("%s:%d", customer.ExpireAt.Format("2006-01-02"), daysUntilExpiration)
		if s.notificationLogger != nil {
			claimed, err := s.notificationLogger.Claim(ctx, customer.ID, database.NotificationRenewal, dedupeKey, map[string]interface{}{
				"days_until_expiration": daysUntilExpiration,
				"expire_at":             customer.ExpireAt.Format(time.RFC3339),
			})
			if err != nil {
				slog.Error("Failed to claim renewal notification log", "customer_id", customer.ID, "error", err)
				continue
			}
			if !claimed {
				continue
			}
		}

		send := s.notify
		if send == nil {
			send = s.sendNotification
		}

		err := send(ctx, customer)
		if err != nil {
			if s.notificationLogger != nil {
				if markErr := s.notificationLogger.MarkFailed(ctx, customer.ID, database.NotificationRenewal, dedupeKey, err); markErr != nil {
					slog.Error("Failed to mark renewal notification failed", "customer_id", customer.ID, "error", markErr)
				}
			}
			slog.Error("Failed to send notification",
				"customer_id", customer.ID,
				"days_until_expiration", daysUntilExpiration,
				"error", err)
			continue
		}
		if s.notificationLogger != nil {
			if err := s.notificationLogger.MarkSent(ctx, customer.ID, database.NotificationRenewal, dedupeKey); err != nil {
				slog.Error("Failed to mark renewal notification sent",
					"customer_id", customer.ID,
					"days_until_expiration", daysUntilExpiration,
					"error", err)
				continue
			}
		}

		slog.Info("Notification sent successfully",
			"customer_id", customer.ID,
			"days_until_expiration", daysUntilExpiration)
	}

	slog.Info(fmt.Sprintf("Processed tributes customers %d with expiring subscriptions", len(tributesProcessed)))
	slog.Info(fmt.Sprintf("Sent notifications to %d customers with expiring subscriptions", len(*customers)-len(tributesProcessed)))
	return nil
}

func (s *SubscriptionService) getCustomersWithExpiringSubscriptions() (*[]database.Customer, error) {
	now := time.Now()
	startDate := now.AddDate(0, 0, -2)
	endDate := now.AddDate(0, 0, 7)

	dbCustomers, err := s.customerRepository.FindByExpirationRange(context.Background(), startDate, endDate)
	if err != nil {
		return nil, err
	}

	return dbCustomers, nil
}

func (s *SubscriptionService) getDaysUntilExpiration(now time.Time, expireAt time.Time) int {
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	expireDate := time.Date(expireAt.Year(), expireAt.Month(), expireAt.Day(), 0, 0, 0, 0, expireAt.Location())

	duration := expireDate.Sub(nowDate)
	return int(duration.Hours() / 24)
}

func (s *SubscriptionService) sendNotification(ctx context.Context, customer database.Customer) error {
	expireDate := customer.ExpireAt.Format("02.01.2006")
	daysUntilExpiration := s.getDaysUntilExpiration(time.Now(), *customer.ExpireAt)

	messageText := fmt.Sprintf(
		s.tm.GetText(customer.Language, "subscription_expiring"),
		expireDate,
		daysUntilExpiration,
	)

	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    customer.TelegramID,
		Text:      messageText,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{s.tm.GetButton(customer.Language, "renew_subscription_button").InlineCallback(handler.CallbackBuy)},
			},
		},
	})

	return err
}

func shouldSendRenewalNotification(daysUntilExpiration int) bool {
	switch daysUntilExpiration {
	case 7, 3, 1, 0, -2:
		return true
	default:
		return false
	}
}
