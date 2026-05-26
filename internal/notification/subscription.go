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

type notificationLogger interface {
	Claim(ctx context.Context, customerID int64, notificationType string, dedupeKey string, metadata map[string]interface{}) (bool, error)
	MarkSent(ctx context.Context, customerID int64, notificationType string, dedupeKey string) error
	MarkFailed(ctx context.Context, customerID int64, notificationType string, dedupeKey string, sendErr error) error
}

type SubscriptionService struct {
	customerRepository customerRepository
	notificationLogger notificationLogger
	telegramBot        *bot.Bot
	tm                 *translation.Manager
	notify             func(context.Context, database.Customer) error
}

func NewSubscriptionService(customerRepository customerRepository,
	notificationLogger notificationLogger,
	telegramBot *bot.Bot,
	tm *translation.Manager) *SubscriptionService {
	svc := &SubscriptionService{customerRepository: customerRepository, notificationLogger: notificationLogger, telegramBot: telegramBot, tm: tm}
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

	for _, customer := range *customers {
		daysUntilExpiration := s.getDaysUntilExpiration(now, *customer.ExpireAt)

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

	slog.Info(fmt.Sprintf("Sent notifications to customers with expiring subscriptions"))
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
