package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type customerRepoMock struct {
	customers *[]database.Customer
	err       error
}

func (m *customerRepoMock) FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]database.Customer, error) {
	return m.customers, m.err
}

type notificationLoggerMock struct {
	claimResult     bool
	claimErr        error
	claimCalls      int
	markSentCalls   int
	markFailedCalls int
	markFailedErrs  []string
}

func (m *notificationLoggerMock) Claim(ctx context.Context, customerID int64, notificationType string, dedupeKey string, metadata map[string]interface{}) (bool, error) {
	m.claimCalls++
	return m.claimResult, m.claimErr
}

func (m *notificationLoggerMock) MarkSent(ctx context.Context, customerID int64, notificationType string, dedupeKey string) error {
	m.markSentCalls++
	return nil
}

func (m *notificationLoggerMock) MarkFailed(ctx context.Context, customerID int64, notificationType string, dedupeKey string, sendErr error) error {
	m.markFailedCalls++
	if sendErr != nil {
		m.markFailedErrs = append(m.markFailedErrs, sendErr.Error())
	}
	return nil
}

func TestSubscriptionService_ProcessSubscriptionExpiration_MarksRenewalSentAfterSuccessfulSend(t *testing.T) {
	expireAt := time.Now().AddDate(0, 0, 3)
	customers := []database.Customer{{ID: 11, ExpireAt: &expireAt}}
	logger := &notificationLoggerMock{claimResult: true}
	notifyCalls := 0

	svc := NewSubscriptionService(&customerRepoMock{customers: &customers}, logger, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		notifyCalls++
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}

	if logger.claimCalls != 1 {
		t.Fatalf("expected renewal notification to be claimed once, got %d", logger.claimCalls)
	}
	if notifyCalls != 1 {
		t.Fatalf("expected notification to be sent once, got %d", notifyCalls)
	}
	if logger.markSentCalls != 1 {
		t.Fatalf("expected renewal notification to be marked sent once, got %d", logger.markSentCalls)
	}
	if logger.markFailedCalls != 0 {
		t.Fatalf("expected renewal notification not to be marked failed, got %d", logger.markFailedCalls)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_MarksRenewalFailedAfterSendFailure(t *testing.T) {
	expireAt := time.Now().AddDate(0, 0, 3)
	customers := []database.Customer{{ID: 12, ExpireAt: &expireAt}}
	logger := &notificationLoggerMock{claimResult: true}
	sendErr := errors.New("telegram rate limit")

	svc := NewSubscriptionService(&customerRepoMock{customers: &customers}, logger, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		return sendErr
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}

	if logger.claimCalls != 1 {
		t.Fatalf("expected renewal notification to be claimed once, got %d", logger.claimCalls)
	}
	if logger.markFailedCalls != 1 {
		t.Fatalf("expected renewal notification to be marked failed once, got %d", logger.markFailedCalls)
	}
	if len(logger.markFailedErrs) != 1 || logger.markFailedErrs[0] != sendErr.Error() {
		t.Fatalf("unexpected failed notification error log: %#v", logger.markFailedErrs)
	}
	if logger.markSentCalls != 0 {
		t.Fatalf("expected renewal notification not to be marked sent, got %d", logger.markSentCalls)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_SkipsRenewalWhenClaimExists(t *testing.T) {
	expireAt := time.Now().AddDate(0, 0, 3)
	customers := []database.Customer{{ID: 13, ExpireAt: &expireAt}}
	logger := &notificationLoggerMock{claimResult: false}
	notifyCalls := 0

	svc := NewSubscriptionService(&customerRepoMock{customers: &customers}, logger, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		notifyCalls++
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}

	if logger.claimCalls != 1 {
		t.Fatalf("expected renewal notification to be claimed once, got %d", logger.claimCalls)
	}
	if notifyCalls != 0 {
		t.Fatalf("expected notification not to be sent after duplicate claim, got %d", notifyCalls)
	}
	if logger.markSentCalls != 0 || logger.markFailedCalls != 0 {
		t.Fatalf("expected duplicate claim not to update delivery state, got sent=%d failed=%d", logger.markSentCalls, logger.markFailedCalls)
	}
}
