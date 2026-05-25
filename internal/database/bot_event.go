package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4/pgxpool"
)

const (
	EventStart               = "start"
	EventStartMenuView       = "start_menu_view"
	EventConnectOpen         = "connect_open"
	EventTrialView           = "trial_view"
	EventTrialActivate       = "trial_activate"
	EventBuyView             = "buy_view"
	EventPlanSelect          = "plan_select"
	EventPaymentMethodSelect = "payment_method_select"
	EventInvoiceCreated      = "invoice_created"
	EventPaymentSuccess      = "payment_success"
	EventReferralOpen        = "referral_open"
	EventReferralQR          = "referral_qr"
	EventInfoOpen            = "info_open"
)

const (
	LifecycleStageNew            = "new"
	LifecycleStageLead           = "lead"
	LifecycleStageTrial          = "trial"
	LifecycleStageInvoiceCreated = "invoice_created"
	LifecycleStagePaid           = "paid"
	LifecycleStageExpired        = "expired"
)

type BotEventRepository struct {
	pool *pgxpool.Pool
}

func NewBotEventRepository(pool *pgxpool.Pool) *BotEventRepository {
	return &BotEventRepository{pool: pool}
}

type BotEvent struct {
	ID         int64                  `db:"id"`
	CustomerID *int64                 `db:"customer_id"`
	TelegramID *int64                 `db:"telegram_id"`
	EventName  string                 `db:"event_name"`
	Source     *string                `db:"source"`
	Medium     *string                `db:"medium"`
	Campaign   *string                `db:"campaign"`
	Stage      *string                `db:"stage"`
	Amount     *float64               `db:"amount"`
	Currency   *string                `db:"currency"`
	Months     *int                   `db:"months"`
	Provider   *string                `db:"provider"`
	PurchaseID *int64                 `db:"purchase_id"`
	Metadata   map[string]interface{} `db:"metadata"`
	CreatedAt  time.Time              `db:"created_at"`
}

func (r *BotEventRepository) Create(ctx context.Context, event *BotEvent) error {
	if event == nil {
		return nil
	}

	metadata := event.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal bot event metadata: %w", err)
	}

	buildInsert := sq.Insert("bot_event").
		Columns("customer_id", "telegram_id", "event_name", "source", "medium", "campaign", "stage", "amount", "currency", "months", "provider", "purchase_id", "metadata").
		Values(event.CustomerID, event.TelegramID, event.EventName, event.Source, event.Medium, event.Campaign, event.Stage, event.Amount, event.Currency, event.Months, event.Provider, event.PurchaseID, sq.Expr("?::jsonb", string(metadataBytes))).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return fmt.Errorf("build bot event insert: %w", err)
	}

	_, err = r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("insert bot event: %w", err)
	}

	return nil
}
