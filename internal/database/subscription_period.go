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
	SubscriptionSourcePaid          = "paid"
	SubscriptionSourceTrial         = "trial"
	SubscriptionSourceReferralBonus = "referral_bonus"
)

type SubscriptionPeriodRepository struct {
	pool *pgxpool.Pool
}

func NewSubscriptionPeriodRepository(pool *pgxpool.Pool) *SubscriptionPeriodRepository {
	return &SubscriptionPeriodRepository{pool: pool}
}

type SubscriptionPeriod struct {
	ID         int64                  `db:"id"`
	CustomerID int64                  `db:"customer_id"`
	PurchaseID *int64                 `db:"purchase_id"`
	SourceType string                 `db:"source_type"`
	StartsAt   time.Time              `db:"starts_at"`
	ExpiresAt  time.Time              `db:"expires_at"`
	Amount     *float64               `db:"amount"`
	Currency   *string                `db:"currency"`
	Months     *int                   `db:"months"`
	Provider   *string                `db:"provider"`
	Metadata   map[string]interface{} `db:"metadata"`
	CreatedAt  time.Time              `db:"created_at"`
}

func (r *SubscriptionPeriodRepository) Create(ctx context.Context, period *SubscriptionPeriod) error {
	if period == nil {
		return nil
	}

	metadata := period.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal subscription period metadata: %w", err)
	}

	buildInsert := sq.Insert("subscription_period").
		Columns("customer_id", "purchase_id", "source_type", "starts_at", "expires_at", "amount", "currency", "months", "provider", "metadata").
		Values(period.CustomerID, period.PurchaseID, period.SourceType, period.StartsAt, period.ExpiresAt, period.Amount, period.Currency, period.Months, period.Provider, sq.Expr("?::jsonb", string(metadataBytes))).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return fmt.Errorf("build subscription period insert: %w", err)
	}

	if _, err := r.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("insert subscription period: %w", err)
	}

	return nil
}
