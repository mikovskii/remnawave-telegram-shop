package database

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
)

func TestBuildLatestActiveTributesQuery(t *testing.T) {
	customerIDs := []int64{10, 20}

	builder := buildLatestActiveTributesQuery(customerIDs).PlaceholderFormat(sq.Dollar)
	sql, args, err := builder.ToSql()
	if err != nil {
		t.Fatalf("ToSql() returned error: %v", err)
	}

	if !strings.Contains(sql, "created_at = (SELECT MAX(created_at)") {
		t.Fatalf("expected SQL to contain subquery selecting latest tribute, got: %s", sql)
	}

	if !strings.Contains(sql, "status <>") {
		t.Fatalf("expected SQL to exclude cancelled tributes, got: %s", sql)
	}

	expectedArgs := []interface{}{InvoiceTypeTribute, int64(10), int64(20), InvoiceTypeTribute, PurchaseStatusCancel}
	if !reflect.DeepEqual(args, expectedArgs) {
		t.Fatalf("unexpected args, want %v, got %v", expectedArgs, args)
	}
}

func TestFindLatestActiveTributesByCustomerIDsEmpty(t *testing.T) {
	repo := &PurchaseRepository{}

	result, err := repo.FindLatestActiveTributesByCustomerIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatalf("result should not be nil")
	}

	if len(*result) != 0 {
		t.Fatalf("expected empty result, got %d", len(*result))
	}
}

func TestBuildClaimAbandonedNotificationQueryRequiresUnpaidStatusAndRetryableClaim(t *testing.T) {
	staleBefore := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	builder := buildClaimAbandonedNotificationQuery(42, staleBefore)
	sql, args, err := builder.ToSql()
	if err != nil {
		t.Fatalf("ToSql() returned error: %v", err)
	}

	if !strings.Contains(sql, "abandoned_notified_at IS NULL") {
		t.Fatalf("expected SQL to require unset abandoned notification, got: %s", sql)
	}
	if !strings.Contains(sql, "status IN") {
		t.Fatalf("expected SQL to restrict abandoned claim by status, got: %s", sql)
	}
	if !strings.Contains(sql, "abandoned_claimed_at IS NULL") || !strings.Contains(sql, "abandoned_claimed_at <=") {
		t.Fatalf("expected SQL to allow stale abandoned claims to be retried, got: %s", sql)
	}

	expectedWhereArgs := []interface{}{int64(42), PurchaseStatusNew, PurchaseStatusPending, staleBefore}
	if len(args) != 5 || !reflect.DeepEqual(args[1:], expectedWhereArgs) {
		t.Fatalf("unexpected args, want timestamp followed by %v, got %v", expectedWhereArgs, args)
	}
}

func TestBuildClaimFailedNotificationQueryRequiresUnsetNotificationAndRetryableClaim(t *testing.T) {
	staleBefore := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	builder := buildClaimFailedNotificationQuery(42, staleBefore)
	sql, args, err := builder.ToSql()
	if err != nil {
		t.Fatalf("ToSql() returned error: %v", err)
	}

	if !strings.Contains(sql, "failed_notified_at IS NULL") {
		t.Fatalf("expected SQL to require unset failed notification, got: %s", sql)
	}
	if !strings.Contains(sql, "failed_claimed_at IS NULL") || !strings.Contains(sql, "failed_claimed_at <=") {
		t.Fatalf("expected SQL to allow stale failed notification claims to be retried, got: %s", sql)
	}

	expectedWhereArgs := []interface{}{int64(42), staleBefore}
	if len(args) != 3 || !reflect.DeepEqual(args[1:], expectedWhereArgs) {
		t.Fatalf("unexpected args, want timestamp followed by %v, got %v", expectedWhereArgs, args)
	}
}
