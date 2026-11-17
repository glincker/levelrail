package provision

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const (
	doSourceBalance = "digitalocean_balance"
	doSourceHistory = "digitalocean_billing_history"
	doBillingScope  = "billing:read"
)

type doBalanceResponse struct {
	MonthToDateBalance string `json:"month_to_date_balance"`
	AccountBalance     string `json:"account_balance"`
	MonthToDateUsage   string `json:"month_to_date_usage"`
	GeneratedAt        string `json:"generated_at"`
}

type doBillingHistoryResponse struct {
	BillingHistory []struct {
		Description string `json:"description"`
		Amount      string `json:"amount"`
		InvoiceID   string `json:"invoice_id"`
		InvoiceUUID string `json:"invoice_uuid"`
		Date        string `json:"date"`
		Type        string `json:"type"`
	} `json:"billing_history"`
}

// sectionFromError maps a provider error to a spend section state. Only a
// 401 or 403 reads as a missing scope, everything else is unavailable.
func sectionFromError(err error) SpendSection {
	var pe *ProviderError
	if errors.As(err, &pe) && (pe.Status == http.StatusForbidden || pe.Status == http.StatusUnauthorized) {
		return SpendSection{State: SectionScopeMissing, Needs: doBillingScope}
	}
	return SpendSection{State: SectionUnavailable}
}

// ReadBilling calls GET /v2/customers/my/balance and
// /v2/customers/my/billing_history. Both are read-only and independent.
func (d *DigitalOcean) ReadBilling(ctx context.Context) (SpendBalance, SpendInvoice) {
	return d.readBalance(ctx), d.readLastInvoice(ctx)
}

func (d *DigitalOcean) readBalance(ctx context.Context) SpendBalance {
	var out doBalanceResponse
	if err := d.client.do(ctx, http.MethodGet, "/customers/my/balance", nil, &out); err != nil {
		return SpendBalance{SpendSection: sectionFromError(err)}
	}
	var asOf *time.Time
	if t, err := time.Parse(time.RFC3339, out.GeneratedAt); err == nil {
		t = t.UTC()
		asOf = &t
	}
	val := func(v string) *SpendValue { return &SpendValue{Value: v, Source: doSourceBalance, DataAsOf: asOf} }
	return SpendBalance{
		SpendSection:   SpendSection{State: SectionOK},
		AccountBalance: val(out.AccountBalance), MonthToDateBalance: val(out.MonthToDateBalance),
		MonthToDateUsage: val(out.MonthToDateUsage), GeneratedAt: asOf,
	}
}

// readLastInvoice picks the newest "Invoice" entry rather than trusting the
// list order, since payments and credits share the same history.
func (d *DigitalOcean) readLastInvoice(ctx context.Context) SpendInvoice {
	var out doBillingHistoryResponse
	if err := d.client.do(ctx, http.MethodGet, "/customers/my/billing_history?per_page=50", nil, &out); err != nil {
		return SpendInvoice{SpendSection: sectionFromError(err)}
	}
	res := SpendInvoice{SpendSection: SpendSection{State: SectionOK}}
	for _, e := range out.BillingHistory {
		if e.Type != "Invoice" {
			continue
		}
		t, err := time.Parse(time.RFC3339, e.Date)
		if err != nil || (res.Date != nil && !t.After(*res.Date)) {
			continue
		}
		t = t.UTC()
		res.Date = &t
		res.Amount = &SpendValue{Value: e.Amount, Source: doSourceHistory, DataAsOf: &t}
		res.InvoiceID, res.InvoiceUUID, res.Description = e.InvoiceID, e.InvoiceUUID, e.Description
	}
	return res
}
