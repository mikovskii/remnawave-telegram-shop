package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4/pgxpool"
)

const (
	NotificationInvoiceAbandoned = "invoice_abandoned"
	NotificationPaymentFailed    = "payment_failed"
	NotificationRenewal          = "renewal"
)

type NotificationLogRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationLogRepository(pool *pgxpool.Pool) *NotificationLogRepository {
	return &NotificationLogRepository{pool: pool}
}

func (r *NotificationLogRepository) Create(ctx context.Context, customerID int64, notificationType string, dedupeKey string, metadata map[string]interface{}) (bool, error) {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return false, fmt.Errorf("marshal notification metadata: %w", err)
	}

	query := sq.Insert("notification_log").
		Columns("customer_id", "type", "dedupe_key", "metadata").
		Values(customerID, notificationType, dedupeKey, sq.Expr("?::jsonb", string(metadataBytes))).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("build notification log insert: %w", err)
	}

	_, err = r.pool.Exec(ctx, sql, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return false, nil
		}
		return false, fmt.Errorf("insert notification log: %w", err)
	}

	return true, nil
}
