package payment

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

func (s PaymentService) trackEvent(ctx context.Context, customer *database.Customer, eventName string, metadata map[string]interface{}) {
	if s.botEventRepository == nil || customer == nil {
		return
	}

	enriched := enrichEventMetadata(customer, metadata)
	event := &database.BotEvent{
		CustomerID: &customer.ID,
		TelegramID: &customer.TelegramID,
		EventName:  eventName,
		Source:     stringPtrFromMetadata(enriched, "source"),
		Medium:     stringPtrFromMetadata(enriched, "medium"),
		Campaign:   stringPtrFromMetadata(enriched, "campaign"),
		Stage:      stringPtrFromMetadata(enriched, "stage"),
		Amount:     floatPtrFromMetadata(enriched, "amount"),
		Currency:   stringPtrFromMetadata(enriched, "currency"),
		Months:     intPtrFromMetadata(enriched, "months", "month"),
		Provider:   stringPtrFromMetadata(enriched, "provider", "invoice_type"),
		PurchaseID: int64PtrFromMetadata(enriched, "purchase_id"),
		Metadata:   enriched,
	}

	go func() {
		eventCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		err := s.botEventRepository.Create(eventCtx, event)
		if err != nil {
			slog.Error("failed to track bot event", "event", eventName, "telegram_id", utils.MaskHalfInt64(customer.TelegramID), "error", err)
		}
	}()
}

func enrichEventMetadata(customer *database.Customer, metadata map[string]interface{}) map[string]interface{} {
	enriched := map[string]interface{}{}
	for key, value := range metadata {
		enriched[key] = value
	}
	if customer == nil {
		return enriched
	}
	if customer.Source != nil {
		setIfMissing(enriched, "source", *customer.Source)
	}
	if customer.Medium != nil {
		setIfMissing(enriched, "medium", *customer.Medium)
	}
	if customer.Campaign != nil {
		setIfMissing(enriched, "campaign", *customer.Campaign)
	}
	if customer.LifecycleStage != "" {
		setIfMissing(enriched, "stage", customer.LifecycleStage)
	}
	return enriched
}

func setIfMissing(metadata map[string]interface{}, key string, value interface{}) {
	if _, ok := metadata[key]; !ok {
		metadata[key] = value
	}
}

func stringPtrFromMetadata(metadata map[string]interface{}, keys ...string) *string {
	for _, key := range keys {
		if value, ok := metadata[key]; ok {
			if s, ok := value.(string); ok && s != "" {
				return &s
			}
		}
	}
	return nil
}

func intPtrFromMetadata(metadata map[string]interface{}, keys ...string) *int {
	for _, key := range keys {
		if value, ok := metadata[key]; ok {
			switch v := value.(type) {
			case int:
				return &v
			case int64:
				i := int(v)
				return &i
			case float64:
				i := int(v)
				return &i
			case string:
				i, err := strconv.Atoi(v)
				if err == nil {
					return &i
				}
			}
		}
	}
	return nil
}

func int64PtrFromMetadata(metadata map[string]interface{}, key string) *int64 {
	if value, ok := metadata[key]; ok {
		switch v := value.(type) {
		case int64:
			return &v
		case int:
			i := int64(v)
			return &i
		case float64:
			i := int64(v)
			return &i
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				return &i
			}
		}
	}
	return nil
}

func floatPtrFromMetadata(metadata map[string]interface{}, key string) *float64 {
	if value, ok := metadata[key]; ok {
		switch v := value.(type) {
		case float64:
			return &v
		case int:
			f := float64(v)
			return &f
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err == nil {
				return &f
			}
		}
	}
	return nil
}

func (s PaymentService) createSubscriptionPeriod(ctx context.Context, customerID int64, purchaseID *int64, sourceType string, months int, expiresAt time.Time, amount *float64, currency *string, provider string, metadata map[string]interface{}) {
	if s.periodRepository == nil || expiresAt.IsZero() {
		return
	}

	startsAt := time.Now()
	monthsPtr := &months
	if months == 0 {
		monthsPtr = nil
	}
	var providerPtr *string
	if provider != "" {
		providerPtr = &provider
	}

	period := &database.SubscriptionPeriod{
		CustomerID: customerID,
		PurchaseID: purchaseID,
		SourceType: sourceType,
		StartsAt:   startsAt,
		ExpiresAt:  expiresAt,
		Amount:     amount,
		Currency:   currency,
		Months:     monthsPtr,
		Provider:   providerPtr,
		Metadata:   metadata,
	}

	periodCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := s.periodRepository.Create(periodCtx, period); err != nil {
		slog.Error("failed to create subscription period", "customer_id", customerID, "source_type", sourceType, "error", err)
	}
}
