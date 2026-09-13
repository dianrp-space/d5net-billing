package handlers

import (
	"context"

	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// invoiceCustomerAdminFee mengembalikan biaya admin MDR yang ditanggung customer
// untuk ditampilkan di invoice/PDF. 0 bila merchant menanggung atau fee ambigu
// (lebih dari satu PG dengan nominal berbeda).
//
// Urutan: fee tersimpan di payment intent (setelah checkout) → hitung dari
// konfigurasi PG aktif untuk sisa tagihan.
func invoiceCustomerAdminFee(ctx context.Context, d *Deps, tid xid.ID, inv *store.Invoice) int64 {
	if inv == nil {
		return 0
	}
	if pi, err := d.Store.GetLatestPaymentIntentForInvoiceAny(ctx, tid, inv.ID); err == nil && pi != nil {
		if f := metaInt64(pi.Metadata, "fee_amount"); f > 0 {
			return f
		}
	}
	base := invoiceRemaining(inv)
	if base <= 0 {
		// Tagihan lunas tanpa fee di intent (bayar tunai / fee merchant).
		return 0
	}
	return uniqueEnabledCustomerAdminFee(ctx, d, tid, base)
}

// uniqueEnabledCustomerAdminFee menghitung fee dari semua PG online yang aktif
// dengan mode customer. Hanya mengembalikan nilai bila semua fee non-nol sama
// (atau hanya satu PG yang membebankan fee), agar dokumen tidak menyesatkan.
func uniqueEnabledCustomerAdminFee(ctx context.Context, d *Deps, tid xid.ID, base int64) int64 {
	if base <= 0 {
		return 0
	}
	var fees []int64
	if cfg, err := loadDuitkuIntegration(ctx, d, tid); err == nil && duitkuCredentialsReady(d, cfg) {
		if f := duitkuCustomerFee(cfg, base); f > 0 {
			fees = append(fees, f)
		}
	}
	if cfg, err := loadDokuIntegration(ctx, d, tid); err == nil && dokuCredentialsReady(d, cfg) {
		if f := dokuCustomerFee(cfg, base); f > 0 {
			fees = append(fees, f)
		}
	}
	if len(fees) == 0 {
		return 0
	}
	first := fees[0]
	for _, f := range fees[1:] {
		if f != first {
			return 0
		}
	}
	return first
}

// enrichInvoiceAdminFee mengisi AdminFee + PayableAmount pada invoice untuk API portal.
func enrichInvoiceAdminFee(ctx context.Context, d *Deps, tid xid.ID, inv *store.Invoice) {
	if inv == nil {
		return
	}
	fee := invoiceCustomerAdminFee(ctx, d, tid, inv)
	if fee <= 0 {
		inv.AdminFee = 0
		inv.PayableAmount = 0
		return
	}
	inv.AdminFee = fee
	remaining := invoiceRemaining(inv)
	if remaining > 0 {
		inv.PayableAmount = remaining + fee
	} else {
		inv.PayableAmount = inv.TotalAmount + fee
	}
}

// attachInvoiceAdminFees mengisi fee untuk daftar invoice portal.
func attachInvoiceAdminFees(ctx context.Context, d *Deps, tid xid.ID, list []store.Invoice) {
	for i := range list {
		enrichInvoiceAdminFee(ctx, d, tid, &list[i])
	}
}
