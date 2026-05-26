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

type purchaseRepoMock struct {
	tributes    *[]database.Purchase
	err         error
	receivedIDs []int64
}

func (m *purchaseRepoMock) FindLatestActiveTributesByCustomerIDs(ctx context.Context, customerIDs []int64) (*[]database.Purchase, error) {
	m.receivedIDs = append([]int64(nil), customerIDs...)
	return m.tributes, m.err
}

type paymentServiceMock struct {
	createCalls        int
	processCalls       int
	amounts            []float64
	months             []int
	customers          []int64
	processIDs         []int64
	createErr          error
	processErr         error
	purchaseIDToReturn int64
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

func (m *paymentServiceMock) CreatePurchase(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType) (string, int64, error) {
	m.createCalls++
	m.amounts = append(m.amounts, amount)
	m.months = append(m.months, months)
	if customer != nil {
		m.customers = append(m.customers, customer.ID)
	}
	if m.purchaseIDToReturn == 0 {
		m.purchaseIDToReturn = int64(m.createCalls)
	}
	return "", m.purchaseIDToReturn, m.createErr
}

func (m *paymentServiceMock) ProcessPurchaseById(ctx context.Context, purchaseId int64) error {
	m.processCalls++
	m.processIDs = append(m.processIDs, purchaseId)
	return m.processErr
}

func TestSubscriptionService_ProcessSubscriptionExpiration_ProcessesTribute(t *testing.T) {
	expireAt := time.Now().Add(24 * time.Hour)
	customers := []database.Customer{{ID: 1, ExpireAt: &expireAt}}
	tributes := []database.Purchase{{CustomerID: 1, Amount: 10.5, Month: 2}}

	cRepo := &customerRepoMock{customers: &customers}
	pRepo := &purchaseRepoMock{tributes: &tributes}
	payMock := &paymentServiceMock{purchaseIDToReturn: 77}

	svc := NewSubscriptionService(cRepo, pRepo, payMock, nil, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		t.Fatalf("sendNotification should not be called in successful tribute processing scenario")
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}

	if payMock.createCalls != 1 {
		t.Fatalf("expected create purchase to be called once, got %d", payMock.createCalls)
	}
	if payMock.processCalls != 1 {
		t.Fatalf("expected process purchase to be called once, got %d", payMock.processCalls)
	}
	if len(payMock.amounts) != 1 || payMock.amounts[0] != tributes[0].Amount {
		t.Fatalf("unexpected amount used for purchase: %#v", payMock.amounts)
	}
	if len(payMock.months) != 1 || payMock.months[0] != tributes[0].Month {
		t.Fatalf("unexpected months used for purchase: %#v", payMock.months)
	}
	if len(payMock.processIDs) != 1 || payMock.processIDs[0] != payMock.purchaseIDToReturn {
		t.Fatalf("expected process to be called with purchase id %d, got %#v", payMock.purchaseIDToReturn, payMock.processIDs)
	}
	if len(pRepo.receivedIDs) != 1 || pRepo.receivedIDs[0] != customers[0].ID {
		t.Fatalf("expected purchase repository to query by customer id %d, got %#v", customers[0].ID, pRepo.receivedIDs)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_SkipsAutoRenewWhenNotOneDay(t *testing.T) {
	expireAt := time.Now().Add(48 * time.Hour)
	customers := []database.Customer{{ID: 5, ExpireAt: &expireAt}}
	tributes := []database.Purchase{{CustomerID: 5, Amount: 20, Month: 1}}

	cRepo := &customerRepoMock{customers: &customers}
	pRepo := &purchaseRepoMock{tributes: &tributes}
	payMock := &paymentServiceMock{purchaseIDToReturn: 101}

	svc := NewSubscriptionService(cRepo, pRepo, payMock, nil, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		t.Fatalf("sendNotification should not be called when auto-renew is skipped due to days remaining")
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}

	if payMock.createCalls != 0 {
		t.Fatalf("expected create purchase not to be called, got %d", payMock.createCalls)
	}
	if payMock.processCalls != 0 {
		t.Fatalf("expected process purchase not to be called, got %d", payMock.processCalls)
	}
	if len(pRepo.receivedIDs) != 1 || pRepo.receivedIDs[0] != customers[0].ID {
		t.Fatalf("expected purchase repository to be queried with customer id %d, got %#v", customers[0].ID, pRepo.receivedIDs)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_SkipsAutoRenewWhenLastTributeCancelled(t *testing.T) {
	expireAt := time.Now().Add(24 * time.Hour)
	customers := []database.Customer{{ID: 9, ExpireAt: &expireAt}}
	tributes := []database.Purchase{}

	cRepo := &customerRepoMock{customers: &customers}
	pRepo := &purchaseRepoMock{tributes: &tributes}
	payMock := &paymentServiceMock{}
	notifyCalls := 0

	svc := NewSubscriptionService(cRepo, pRepo, payMock, nil, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		notifyCalls++
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}

	if payMock.createCalls != 0 {
		t.Fatalf("expected create purchase not to be called, got %d", payMock.createCalls)
	}
	if payMock.processCalls != 0 {
		t.Fatalf("expected process purchase not to be called, got %d", payMock.processCalls)
	}
	if notifyCalls != 1 {
		t.Fatalf("expected notification to be sent once, got %d", notifyCalls)
	}
	if len(pRepo.receivedIDs) != 1 || pRepo.receivedIDs[0] != customers[0].ID {
		t.Fatalf("expected purchase repository to query by customer id %d, got %#v", customers[0].ID, pRepo.receivedIDs)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_MarksRenewalSentAfterSuccessfulSend(t *testing.T) {
	expireAt := time.Now().AddDate(0, 0, 3)
	customers := []database.Customer{{ID: 11, ExpireAt: &expireAt}}
	tributes := []database.Purchase{}
	logger := &notificationLoggerMock{claimResult: true}
	notifyCalls := 0

	svc := NewSubscriptionService(&customerRepoMock{customers: &customers}, &purchaseRepoMock{tributes: &tributes}, &paymentServiceMock{}, logger, nil, nil)
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
	tributes := []database.Purchase{}
	logger := &notificationLoggerMock{claimResult: true}
	sendErr := errors.New("telegram rate limit")

	svc := NewSubscriptionService(&customerRepoMock{customers: &customers}, &purchaseRepoMock{tributes: &tributes}, &paymentServiceMock{}, logger, nil, nil)
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
	tributes := []database.Purchase{}
	logger := &notificationLoggerMock{claimResult: false}
	notifyCalls := 0

	svc := NewSubscriptionService(&customerRepoMock{customers: &customers}, &purchaseRepoMock{tributes: &tributes}, &paymentServiceMock{}, logger, nil, nil)
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
