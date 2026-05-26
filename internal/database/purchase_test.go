package database

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

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
