package payment

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/platega"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
)

type PaymentProvider interface {
	InvoiceType() database.InvoiceType
	Currency() string
	CreateInvoice(ctx context.Context, req InvoiceRequest) (*Invoice, error)
}

type InvoiceRequest struct {
	PurchaseID  int64
	Amount      float64
	Months      int
	Customer    *database.Customer
	Description string
	ReturnURL   string
}

type Invoice struct {
	URL        string
	ExternalID string
}

func NewPaymentProviders(telegramBot *bot.Bot, tm *translation.Manager, plategaClient *platega.Client) map[database.InvoiceType]PaymentProvider {
	providers := map[database.InvoiceType]PaymentProvider{
		database.InvoiceTypeTelegram: &StarsProvider{telegramBot: telegramBot, tm: tm},
	}

	for _, invoiceType := range []database.InvoiceType{
		database.InvoiceTypePlategaSBP,
		database.InvoiceTypePlategaCards,
		database.InvoiceTypePlategaAcquiring,
		database.InvoiceTypePlategaWorldwide,
		database.InvoiceTypePlategaCrypto,
	} {
		providers[invoiceType] = &PlategaProvider{client: plategaClient, invoiceType: invoiceType}
	}

	return providers
}

type PlategaProvider struct {
	client      *platega.Client
	invoiceType database.InvoiceType
}

func (p *PlategaProvider) InvoiceType() database.InvoiceType {
	return p.invoiceType
}

func (p *PlategaProvider) Currency() string {
	return "RUB"
}

func (p *PlategaProvider) CreateInvoice(ctx context.Context, req InvoiceRequest) (*Invoice, error) {
	if p.client == nil {
		return nil, fmt.Errorf("platega client not configured")
	}
	provider, err := platega.ProviderFor(p.client, p.invoiceType)
	if err != nil {
		return nil, err
	}
	redirectURL, transactionID, err := provider.CreateInvoice(ctx, req.PurchaseID, req.Amount, p.Currency(), req.Description, req.ReturnURL)
	if err != nil {
		return nil, err
	}
	return &Invoice{URL: redirectURL, ExternalID: transactionID}, nil
}

type StarsProvider struct {
	telegramBot *bot.Bot
	tm          *translation.Manager
}

func (p *StarsProvider) InvoiceType() database.InvoiceType {
	return database.InvoiceTypeTelegram
}

func (p *StarsProvider) Currency() string {
	return "STARS"
}

func (p *StarsProvider) CreateInvoice(ctx context.Context, req InvoiceRequest) (*Invoice, error) {
	if p.telegramBot == nil {
		return nil, fmt.Errorf("telegram bot not configured")
	}
	invoiceURL, err := p.telegramBot.CreateInvoiceLink(ctx, &bot.CreateInvoiceLinkParams{
		Title:    p.tm.GetText(req.Customer.Language, "invoice_title"),
		Currency: "XTR",
		Prices: []models.LabeledPrice{
			{
				Label:  p.tm.GetText(req.Customer.Language, "invoice_label"),
				Amount: int(req.Amount),
			},
		},
		Description: p.tm.GetText(req.Customer.Language, "invoice_description"),
		Payload:     fmt.Sprintf("%d&%s", req.PurchaseID, remnawave.UsernameFromCtx(ctx)),
	})
	if err != nil {
		return nil, err
	}
	return &Invoice{URL: invoiceURL}, nil
}
