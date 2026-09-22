package store

import (
	"context"
	"errors"
	"strings"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

const invoiceSettingKey = "invoice.customization"

// Warna default dokumen invoice (dipertahankan dari desain awal).
const (
	// DefaultInvoiceAccent adalah hijau zaitun untuk bar aksen, judul,
	// header tabel, dan garis pemisah.
	DefaultInvoiceAccent = "#5A5A40"
	// DefaultInvoiceStamp adalah biru untuk watermark/stempel LUNAS.
	DefaultInvoiceStamp = "#1F40B0"
)

// InvoiceSettings holds tenant-configurable content for the invoice document
// (header/company identity, payment instructions and footer). Empty fields fall
// back to sensible defaults derived from the tenant profile at render time.
type InvoiceSettings struct {
	// CompanyName overrides the business name printed on the invoice header.
	CompanyName string `json:"company_name"`
	// Address is the business address block (multi-line allowed).
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Website string `json:"website"`
	// TaxID e.g. NPWP.
	TaxID string `json:"tax_id"`
	// PaymentInstructions e.g. bank transfer / VA details (multi-line).
	PaymentInstructions string `json:"payment_instructions"`
	// FooterNote printed at the bottom (terms, thank-you note).
	FooterNote string `json:"footer_note"`
	// AccentColor ("#rrggbb") untuk bar aksen, judul INVOICE, header tabel,
	// dan garis pemisah. Kosong = default hijau zaitun.
	AccentColor string `json:"accent_color"`
	// StampColor ("#rrggbb") untuk watermark/stempel LUNAS.
	// Kosong = default biru.
	StampColor string `json:"stamp_color"`
}

func trimInvoiceMultiline(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

func NormalizeInvoiceSettings(c InvoiceSettings) InvoiceSettings {
	c.CompanyName = strings.TrimSpace(c.CompanyName)
	c.Address = trimInvoiceMultiline(c.Address)
	c.Phone = strings.TrimSpace(c.Phone)
	c.Email = strings.TrimSpace(c.Email)
	c.Website = strings.TrimSpace(c.Website)
	c.TaxID = strings.TrimSpace(c.TaxID)
	c.PaymentInstructions = trimInvoiceMultiline(c.PaymentInstructions)
	c.FooterNote = trimInvoiceMultiline(c.FooterNote)
	if v := NormalizeHexColor(c.AccentColor); v != "" {
		c.AccentColor = v
	} else {
		c.AccentColor = DefaultInvoiceAccent
	}
	if v := NormalizeHexColor(c.StampColor); v != "" {
		c.StampColor = v
	} else {
		c.StampColor = DefaultInvoiceStamp
	}
	return c
}

func (s *Store) GetInvoiceSettings(ctx context.Context, tenantID xid.ID) (InvoiceSettings, error) {
	var cfg InvoiceSettings
	err := s.GetSettingJSON(ctx, tenantID, invoiceSettingKey, &cfg)
	if errors.Is(err, ErrNotFound) {
		return InvoiceSettings{}, nil
	}
	if err != nil {
		return InvoiceSettings{}, err
	}
	return NormalizeInvoiceSettings(cfg), nil
}

func (s *Store) UpsertInvoiceSettings(ctx context.Context, tenantID xid.ID, cfg InvoiceSettings) error {
	return s.UpsertSettingJSON(ctx, tenantID, invoiceSettingKey, NormalizeInvoiceSettings(cfg))
}
