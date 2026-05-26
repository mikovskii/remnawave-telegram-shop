package payment

import (
	"context"
	"log/slog"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

type ProvisioningService struct {
	remnawaveClient    *remnawave.Client
	customerRepository *database.CustomerRepository
	referralRepository *database.ReferralRepository
	periodRepository   *database.SubscriptionPeriodRepository
}

type ProvisionResult struct {
	User          *remnawave.User
	ReferralBonus *ReferralBonusResult
}

type ReferralBonusResult struct {
	Customer   *database.Customer
	ReferralID int64
}

func NewProvisioningService(
	remnawaveClient *remnawave.Client,
	customerRepository *database.CustomerRepository,
	referralRepository *database.ReferralRepository,
	periodRepository *database.SubscriptionPeriodRepository,
) *ProvisioningService {
	return &ProvisioningService{
		remnawaveClient:    remnawaveClient,
		customerRepository: customerRepository,
		referralRepository: referralRepository,
		periodRepository:   periodRepository,
	}
}

func (s *ProvisioningService) CalculatePaidExpireAt(ctx context.Context, telegramID int64, months int) (time.Time, error) {
	return s.remnawaveClient.CalculatePaidExpireAt(ctx, telegramID, months*config.DaysInMonth())
}

func (s *ProvisioningService) FulfillPurchase(ctx context.Context, customer *database.Customer, purchase *database.Purchase, expireAt time.Time) (*ProvisionResult, error) {
	user, err := s.remnawaveClient.CreateOrUpdateUserWithExpireAt(ctx, customer.ID, customer.TelegramID, config.TrafficLimit(), expireAt, false)
	if err != nil {
		return nil, err
	}

	customerFields := map[string]interface{}{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
		"lifecycle_stage":   database.LifecycleStagePaid,
		"lead_score":        customer.LeadScore + 100,
	}
	if customer.FirstPaidAt == nil {
		customerFields["first_paid_at"] = time.Now()
	}
	if err := s.customerRepository.UpdateFields(ctx, customer.ID, customerFields); err != nil {
		return nil, err
	}

	result := &ProvisionResult{User: user}
	referralBonus, err := s.grantReferralBonus(context.Background(), customer)
	if err != nil {
		return nil, err
	}
	result.ReferralBonus = referralBonus

	s.createSubscriptionPeriod(ctx, customer.ID, &purchase.ID, database.SubscriptionSourcePaid, purchase.Month, user.ExpireAt, &purchase.Amount, &purchase.Currency, string(purchase.InvoiceType), map[string]interface{}{
		"purchase_id":  purchase.ID,
		"invoice_type": string(purchase.InvoiceType),
	})

	return result, nil
}

func (s *ProvisioningService) ActivateTrial(ctx context.Context, customer *database.Customer) (string, error) {
	user, err := s.remnawaveClient.CreateOrUpdateUser(ctx, customer.ID, customer.TelegramID, config.TrialTrafficLimit(), config.TrialDays(), true)
	if err != nil {
		slog.Error("Error creating user", "error", err)
		return "", err
	}

	if err := s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
		"lifecycle_stage":   database.LifecycleStageTrial,
	}); err != nil {
		return "", err
	}

	s.createSubscriptionPeriod(ctx, customer.ID, nil, database.SubscriptionSourceTrial, 0, user.ExpireAt, nil, nil, "", map[string]interface{}{
		"trial_days": config.TrialDays(),
	})

	return user.SubscriptionUrl, nil
}

func (s *ProvisioningService) grantReferralBonus(ctx context.Context, customer *database.Customer) (*ReferralBonusResult, error) {
	referral, err := s.referralRepository.FindByReferee(ctx, customer.TelegramID)
	if err != nil {
		return nil, err
	}
	if referral == nil || referral.BonusGranted {
		return nil, nil
	}

	referrerCustomer, err := s.customerRepository.FindByTelegramId(ctx, referral.ReferrerID)
	if err != nil {
		return nil, err
	}
	if referrerCustomer == nil {
		return nil, nil
	}

	referrerUser, err := s.remnawaveClient.CreateOrUpdateUser(ctx, referrerCustomer.ID, referrerCustomer.TelegramID, config.TrafficLimit(), config.GetReferralDays(), false)
	if err != nil {
		return nil, err
	}
	if err := s.customerRepository.UpdateFields(ctx, referrerCustomer.ID, map[string]interface{}{
		"subscription_link": referrerUser.SubscriptionUrl,
		"expire_at":         referrerUser.ExpireAt,
		"lifecycle_stage":   database.LifecycleStagePaid,
	}); err != nil {
		return nil, err
	}

	bonusClaimed, err := s.referralRepository.MarkBonusGranted(ctx, referral.ID, config.GetReferralDays())
	if err != nil {
		return nil, err
	}
	if !bonusClaimed {
		slog.Info("referral bonus already granted", "referral_id", referral.ID)
		return nil, nil
	}

	s.createSubscriptionPeriod(ctx, referrerCustomer.ID, nil, database.SubscriptionSourceReferralBonus, 0, referrerUser.ExpireAt, nil, nil, "", map[string]interface{}{
		"referral_id": referral.ID,
		"earned_days": config.GetReferralDays(),
	})
	slog.Info("Granted referral bonus", "customer_id", utils.MaskHalfInt64(referrerCustomer.ID))

	return &ReferralBonusResult{Customer: referrerCustomer, ReferralID: referral.ID}, nil
}
