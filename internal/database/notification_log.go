package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

const (
	NotificationInvoiceAbandoned = "invoice_abandoned"
	NotificationPaymentFailed    = "payment_failed"
	NotificationRenewal          = "renewal"

	NotificationStatusPending = "pending"
	NotificationStatusFailed  = "failed"
	NotificationStatusSent    = "sent"
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

func (r *NotificationLogRepository) Claim(ctx context.Context, customerID int64, notificationType string, dedupeKey string, metadata map[string]interface{}) (bool, error) {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return false, fmt.Errorf("marshal notification metadata: %w", err)
	}

	query := sq.Insert("notification_log").
		Columns("customer_id", "type", "dedupe_key", "status", "metadata", "last_error").
		Values(customerID, notificationType, dedupeKey, NotificationStatusPending, sq.Expr("?::jsonb", string(metadataBytes)), nil).
		Suffix(`
ON CONFLICT (customer_id, type, dedupe_key) DO UPDATE
SET status = EXCLUDED.status,
    metadata = EXCLUDED.metadata,
    last_error = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE notification_log.status = ?
   OR (notification_log.status = ? AND notification_log.updated_at < CURRENT_TIMESTAMP - INTERVAL '15 minutes')
RETURNING id`, NotificationStatusFailed, NotificationStatusPending).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("build notification claim query: %w", err)
	}

	var id int64
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("claim notification log: %w", err)
	}

	return true, nil
}

func (r *NotificationLogRepository) MarkSent(ctx context.Context, customerID int64, notificationType string, dedupeKey string) error {
	query := sq.Update("notification_log").
		Set("status", NotificationStatusSent).
		Set("sent_at", sq.Expr("CURRENT_TIMESTAMP")).
		Set("last_error", nil).
		Set("updated_at", sq.Expr("CURRENT_TIMESTAMP")).
		Where(notificationLogKey(customerID, notificationType, dedupeKey)).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("build notification sent query: %w", err)
	}
	if _, err := r.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("mark notification sent: %w", err)
	}
	return nil
}

func (r *NotificationLogRepository) MarkFailed(ctx context.Context, customerID int64, notificationType string, dedupeKey string, sendErr error) error {
	lastError := ""
	if sendErr != nil {
		lastError = sendErr.Error()
	}

	query := sq.Update("notification_log").
		Set("status", NotificationStatusFailed).
		Set("last_error", lastError).
		Set("updated_at", sq.Expr("CURRENT_TIMESTAMP")).
		Where(notificationLogKey(customerID, notificationType, dedupeKey)).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("build notification failed query: %w", err)
	}
	if _, err := r.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("mark notification failed: %w", err)
	}
	return nil
}

func notificationLogKey(customerID int64, notificationType string, dedupeKey string) sq.And {
	return sq.And{
		sq.Eq{"customer_id": customerID},
		sq.Eq{"type": notificationType},
		sq.Eq{"dedupe_key": dedupeKey},
	}
}
