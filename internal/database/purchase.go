package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type InvoiceType string

const (
	InvoiceTypeTelegram         InvoiceType = "telegram"
	InvoiceTypePlategaSBP       InvoiceType = "plt_sbp"
	InvoiceTypePlategaCards     InvoiceType = "plt_cards"
	InvoiceTypePlategaAcquiring InvoiceType = "plt_acq"
	InvoiceTypePlategaWorldwide InvoiceType = "plt_ww"
	InvoiceTypePlategaCrypto    InvoiceType = "plt_crypto"
)

type PurchaseStatus string

const (
	PurchaseStatusNew     PurchaseStatus = "new"
	PurchaseStatusPending PurchaseStatus = "pending"
	PurchaseStatusPaid    PurchaseStatus = "paid"
	PurchaseStatusCancel  PurchaseStatus = "cancel"
)

type Purchase struct {
	ID                  int64          `db:"id"`
	Amount              float64        `db:"amount"`
	CustomerID          int64          `db:"customer_id"`
	CreatedAt           time.Time      `db:"created_at"`
	Month               int            `db:"month"`
	PaidAt              *time.Time     `db:"paid_at"`
	Currency            string         `db:"currency"`
	ExpireAt            *time.Time     `db:"expire_at"`
	Status              PurchaseStatus `db:"status"`
	InvoiceType         InvoiceType    `db:"invoice_type"`
	PlategaID           *string        `db:"platega_id"`
	PlategaURL          *string        `db:"platega_url"`
	AbandonedClaimedAt  *time.Time     `db:"abandoned_claimed_at"`
	AbandonedNotifiedAt *time.Time     `db:"abandoned_notified_at"`
	FailedClaimedAt     *time.Time     `db:"failed_claimed_at"`
	FailedNotifiedAt    *time.Time     `db:"failed_notified_at"`
	FulfilledAt         *time.Time     `db:"fulfilled_at"`
}

var purchaseColumns = []string{
	"id",
	"amount",
	"customer_id",
	"created_at",
	"month",
	"paid_at",
	"currency",
	"expire_at",
	"status",
	"invoice_type",
	"platega_id",
	"platega_url",
	"abandoned_claimed_at",
	"abandoned_notified_at",
	"failed_claimed_at",
	"failed_notified_at",
	"fulfilled_at",
}

func scanPurchase(scanner interface {
	Scan(dest ...interface{}) error
}, purchase *Purchase) error {
	return scanner.Scan(
		&purchase.ID,
		&purchase.Amount,
		&purchase.CustomerID,
		&purchase.CreatedAt,
		&purchase.Month,
		&purchase.PaidAt,
		&purchase.Currency,
		&purchase.ExpireAt,
		&purchase.Status,
		&purchase.InvoiceType,
		&purchase.PlategaID,
		&purchase.PlategaURL,
		&purchase.AbandonedClaimedAt,
		&purchase.AbandonedNotifiedAt,
		&purchase.FailedClaimedAt,
		&purchase.FailedNotifiedAt,
		&purchase.FulfilledAt,
	)
}

type PurchaseRepository struct {
	pool *pgxpool.Pool
}

func NewPurchaseRepository(pool *pgxpool.Pool) *PurchaseRepository {
	return &PurchaseRepository{
		pool: pool,
	}
}

func (cr *PurchaseRepository) Create(ctx context.Context, purchase *Purchase) (int64, error) {
	buildInsert := sq.Insert("purchase").
		Columns("amount", "customer_id", "month", "currency", "expire_at", "status", "invoice_type", "platega_id", "platega_url").
		Values(purchase.Amount, purchase.CustomerID, purchase.Month, purchase.Currency, purchase.ExpireAt, purchase.Status, purchase.InvoiceType, purchase.PlategaID, purchase.PlategaURL).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return 0, err
	}

	var id int64
	err = cr.pool.QueryRow(ctx, sql, args...).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (cr *PurchaseRepository) FindByInvoiceTypeAndStatus(ctx context.Context, invoiceType InvoiceType, status PurchaseStatus) (*[]Purchase, error) {
	buildSelect := sq.Select(purchaseColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"invoice_type": invoiceType},
			sq.Eq{"status": status},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query purchases: %w", err)
	}
	defer rows.Close()

	purchases := []Purchase{}
	for rows.Next() {
		purchase := Purchase{}
		if err = scanPurchase(rows, &purchase); err != nil {
			return nil, fmt.Errorf("failed to scan purchase: %w", err)
		}
		purchases = append(purchases, purchase)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &purchases, nil
}

func (cr *PurchaseRepository) FindById(ctx context.Context, id int64) (*Purchase, error) {
	buildSelect := sq.Select(purchaseColumns...).
		From("purchase").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}
	purchase := &Purchase{}

	err = scanPurchase(cr.pool.QueryRow(ctx, sql, args...), purchase)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query purchase: %w", err)
	}

	return purchase, nil
}

func (p *PurchaseRepository) UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	buildUpdate := sq.Update("purchase").
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{"id": id})

	for field, value := range updates {
		buildUpdate = buildUpdate.Set(field, value)
	}

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := p.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update customer: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no customer found with id: %d", id)
	}

	return nil
}

func (pr *PurchaseRepository) MarkAsPaid(ctx context.Context, purchaseID int64, expireAt time.Time) (bool, error) {
	query := sq.Update("purchase").
		Set("status", PurchaseStatusPaid).
		Set("paid_at", time.Now()).
		Set("expire_at", expireAt).
		Where(sq.And{
			sq.Eq{"id": purchaseID},
			sq.Or{
				sq.NotEq{"status": PurchaseStatusPaid},
				sq.Eq{"expire_at": nil},
			},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build mark paid query: %w", err)
	}

	result, err := pr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("failed to mark purchase paid: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

func (pr *PurchaseRepository) MarkFulfilled(ctx context.Context, purchaseID int64) error {
	query := sq.Update("purchase").
		Set("fulfilled_at", time.Now()).
		Where(sq.Eq{"id": purchaseID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build mark fulfilled query: %w", err)
	}

	if _, err := pr.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("failed to mark purchase fulfilled: %w", err)
	}

	return nil
}

func (pr *PurchaseRepository) MarkAbandonedNotifiedIfUnset(ctx context.Context, purchaseID int64) (bool, error) {
	query := buildMarkAbandonedNotifiedQuery(purchaseID)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build abandoned notification mark query: %w", err)
	}

	result, err := pr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("failed to mark abandoned purchase notification: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

func (pr *PurchaseRepository) ClaimAbandonedNotification(ctx context.Context, purchaseID int64, staleBefore time.Time) (bool, error) {
	query := buildClaimAbandonedNotificationQuery(purchaseID, staleBefore)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build abandoned notification claim query: %w", err)
	}

	result, err := pr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("failed to claim abandoned purchase notification: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

func (pr *PurchaseRepository) MarkFailedNotifiedIfUnset(ctx context.Context, purchaseID int64) (bool, error) {
	return pr.markNotifiedIfUnset(ctx, purchaseID, "failed_notified_at")
}

func (pr *PurchaseRepository) ClaimFailedNotification(ctx context.Context, purchaseID int64, staleBefore time.Time) (bool, error) {
	query := buildClaimFailedNotificationQuery(purchaseID, staleBefore)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build failed notification claim query: %w", err)
	}

	result, err := pr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("failed to claim failed purchase notification: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

func (pr *PurchaseRepository) markNotifiedIfUnset(ctx context.Context, purchaseID int64, field string) (bool, error) {
	query := sq.Update("purchase").
		Set(field, time.Now()).
		Where(sq.And{
			sq.Eq{"id": purchaseID},
			sq.Eq{field: nil},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build notification mark query: %w", err)
	}

	result, err := pr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("failed to mark purchase notification: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

func buildMarkAbandonedNotifiedQuery(purchaseID int64) sq.UpdateBuilder {
	return sq.Update("purchase").
		Set("abandoned_notified_at", time.Now()).
		Where(sq.And{
			sq.Eq{"id": purchaseID},
			sq.Eq{"abandoned_notified_at": nil},
			sq.Eq{"status": []PurchaseStatus{PurchaseStatusNew, PurchaseStatusPending}},
		}).
		PlaceholderFormat(sq.Dollar)
}

func buildClaimAbandonedNotificationQuery(purchaseID int64, staleBefore time.Time) sq.UpdateBuilder {
	return sq.Update("purchase").
		Set("abandoned_claimed_at", time.Now()).
		Where(sq.And{
			sq.Eq{"id": purchaseID},
			sq.Eq{"abandoned_notified_at": nil},
			sq.Eq{"status": []PurchaseStatus{PurchaseStatusNew, PurchaseStatusPending}},
			sq.Or{
				sq.Eq{"abandoned_claimed_at": nil},
				sq.LtOrEq{"abandoned_claimed_at": staleBefore},
			},
		}).
		PlaceholderFormat(sq.Dollar)
}

func buildClaimFailedNotificationQuery(purchaseID int64, staleBefore time.Time) sq.UpdateBuilder {
	return sq.Update("purchase").
		Set("failed_claimed_at", time.Now()).
		Where(sq.And{
			sq.Eq{"id": purchaseID},
			sq.Eq{"failed_notified_at": nil},
			sq.Or{
				sq.Eq{"failed_claimed_at": nil},
				sq.LtOrEq{"failed_claimed_at": staleBefore},
			},
		}).
		PlaceholderFormat(sq.Dollar)
}

func (pr *PurchaseRepository) FindAbandonedInvoices(ctx context.Context, olderThan time.Time) (*[]Purchase, error) {
	query := sq.Select(purchaseColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"status": []PurchaseStatus{PurchaseStatusNew, PurchaseStatusPending}},
			sq.LtOrEq{"created_at": olderThan},
			sq.Eq{"abandoned_notified_at": nil},
			sq.Or{
				sq.Eq{"abandoned_claimed_at": nil},
				sq.LtOrEq{"abandoned_claimed_at": olderThan},
			},
		}).
		OrderBy("created_at ASC").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build abandoned invoices query: %w", err)
	}

	rows, err := pr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query abandoned invoices: %w", err)
	}
	defer rows.Close()

	var purchases []Purchase
	for rows.Next() {
		var purchase Purchase
		if err := scanPurchase(rows, &purchase); err != nil {
			return nil, fmt.Errorf("scan abandoned invoice: %w", err)
		}
		purchases = append(purchases, purchase)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate abandoned invoices: %w", err)
	}

	return &purchases, nil
}

func (pr *PurchaseRepository) FindSuccessfulPaidPurchaseByCustomer(ctx context.Context, customerID int64) (*Purchase, error) {
	query := sq.Select(purchaseColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
			sq.Or{
				sq.Eq{"invoice_type": InvoiceTypePlategaSBP},
				sq.Eq{"invoice_type": InvoiceTypePlategaCards},
				sq.Eq{"invoice_type": InvoiceTypePlategaAcquiring},
				sq.Eq{"invoice_type": InvoiceTypePlategaWorldwide},
				sq.Eq{"invoice_type": InvoiceTypePlategaCrypto},
			},
		}).
		OrderBy("paid_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	p := &Purchase{}
	err = scanPurchase(pr.pool.QueryRow(ctx, sql, args...), p)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase: %w", err)
	}

	return p, nil
}
