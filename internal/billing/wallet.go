package billing

import (
	"context"

	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// WalletAutoPayResult merangkum hasil percobaan auto-pay satu tagihan dari saldo.
type WalletAutoPayResult struct {
	WalletEnabled bool                `json:"wallet_enabled"`
	Paid          bool                `json:"paid"`
	Balance       int64               `json:"balance"`
	Invoice       *store.Invoice      `json:"invoice,omitempty"`
	Items         []store.InvoiceItem `json:"items,omitempty"`
	Customer      *store.Customer     `json:"customer,omitempty"`
	Payment       *store.Payment      `json:"payment,omitempty"`
}

// TryAutoPayInvoice melunasi tagihan dari saldo pelanggan bila fitur saldo aktif
// dan saldo mencukupi. Tidak memotong saldo bila kurang (paid=false).
func (e *Engine) TryAutoPayInvoice(ctx context.Context, tenantID, invoiceID xid.ID) (*WalletAutoPayResult, error) {
	res := &WalletAutoPayResult{}
	if !e.store.WalletEnabled(ctx, tenantID) {
		return res, nil
	}
	res.WalletEnabled = true
	inv, items, err := e.store.GetInvoice(ctx, tenantID, invoiceID)
	if err != nil {
		return res, err
	}
	res.Invoice = inv
	res.Items = items
	if cust, cerr := e.store.GetCustomer(ctx, tenantID, inv.CustomerID); cerr == nil {
		res.Customer = cust
	}
	payment, paid, err := e.store.PayInvoiceFromWallet(ctx, tenantID, inv.CustomerID, invoiceID)
	if err != nil {
		return res, err
	}
	res.Paid = paid
	res.Payment = payment
	if bal, berr := e.store.GetWallet(ctx, tenantID, inv.CustomerID); berr == nil {
		res.Balance = bal
	}
	return res, nil
}

// AutoSettleCustomer melunasi tagihan menunggak pelanggan dari saldo, dari yang
// paling lama, sampai saldo tidak lagi cukup. Hanya tagihan yang berhasil lunas
// yang dikembalikan.
func (e *Engine) AutoSettleCustomer(ctx context.Context, tenantID, customerID xid.ID) ([]WalletAutoPayResult, error) {
	results := []WalletAutoPayResult{}
	if !e.store.WalletEnabled(ctx, tenantID) {
		return results, nil
	}
	ids, err := e.store.OutstandingInvoiceIDs(ctx, tenantID, customerID)
	if err != nil {
		return results, err
	}
	for _, id := range ids {
		res, err := e.TryAutoPayInvoice(ctx, tenantID, id)
		if err != nil {
			return results, err
		}
		if !res.Paid {
			break
		}
		results = append(results, *res)
	}
	return results, nil
}
