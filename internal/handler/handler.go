package handler

import (
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/translation"
)

type Handler struct {
	customerRepository *database.CustomerRepository
	purchaseRepository *database.PurchaseRepository
	translation        *translation.Manager
	purchaseService    *payment.PurchaseService
	syncService        *sync.SyncService
	referralRepository *database.ReferralRepository
	botEventRepository *database.BotEventRepository
	cache              *cache.Cache
}

func NewHandler(
	syncService *sync.SyncService,
	purchaseService *payment.PurchaseService,
	translation *translation.Manager,
	customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	referralRepository *database.ReferralRepository,
	botEventRepository *database.BotEventRepository,
	cache *cache.Cache) *Handler {
	return &Handler{
		syncService:        syncService,
		purchaseService:    purchaseService,
		customerRepository: customerRepository,
		purchaseRepository: purchaseRepository,
		translation:        translation,
		referralRepository: referralRepository,
		botEventRepository: botEventRepository,
		cache:              cache,
	}
}
