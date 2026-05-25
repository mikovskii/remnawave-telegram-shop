package handler

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

var handlerTrackEventSem = make(chan struct{}, 100)

func (h Handler) trackEvent(ctx context.Context, customer *database.Customer, telegramID int64, eventName string, metadata map[string]interface{}) {
	if h.botEventRepository == nil {
		return
	}

	var customerID *int64
	if customer != nil {
		customerID = &customer.ID
		if telegramID == 0 {
			telegramID = customer.TelegramID
		}
	}

	var telegramIDPtr *int64
	if telegramID != 0 {
		telegramIDPtr = &telegramID
	}

	enriched := enrichEventMetadata(customer, metadata)
	event := &database.BotEvent{
		CustomerID: customerID,
		TelegramID: telegramIDPtr,
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

	select {
	case handlerTrackEventSem <- struct{}{}:
		go func() {
			defer func() { <-handlerTrackEventSem }()
			eventCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			err := h.botEventRepository.Create(eventCtx, event)
			if err != nil {
				slog.Error("failed to track bot event", "event", eventName, "telegram_id", utils.MaskHalfInt64(telegramID), "error", err)
			}
		}()
	default:
		slog.Warn("bot event tracking queue full", "event", eventName, "telegram_id", utils.MaskHalfInt64(telegramID))
	}
}

func parseStartPayload(payload string) map[string]interface{} {
	result := map[string]interface{}{
		"payload": payload,
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		result["source"] = "direct"
		return result
	}

	parts := strings.Split(payload, "__")
	for _, part := range parts {
		if value, ok := strings.CutPrefix(part, "utm_source_"); ok && value != "" {
			result["source"] = value
			continue
		}
		if value, ok := strings.CutPrefix(part, "utm_medium_"); ok && value != "" {
			result["medium"] = value
			continue
		}
		if value, ok := strings.CutPrefix(part, "utm_campaign_"); ok && value != "" {
			result["campaign"] = value
			continue
		}

		key, value, ok := strings.Cut(part, "_")
		if !ok || value == "" {
			continue
		}
		switch key {
		case "ref":
			referrerID, err := strconv.ParseInt(value, 10, 64)
			if err == nil {
				result["referrer_telegram_id"] = referrerID
				result["source"] = "referral"
			}
		case "src":
			result["source"] = value
		case "cmp", "campaign":
			result["campaign"] = value
		}
	}

	if _, ok := result["source"]; !ok {
		result["source"] = payload
	}
	return result
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
