package invoice

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/dianrp/drp-billing/internal/store"
)

func TestRenderPDFValidDocument(t *testing.T) {
	issued := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	inv := &store.Invoice{
		InvoiceNumber:  "INV-2026-0001",
		CustomerName:   "Budi Santoso",
		CustomerCode:   "CUST-001",
		Subtotal:       150000,
		TaxAmount:      16500,
		DiscountAmount: 5000,
		TotalAmount:    161500,
		PaidAmount:     50000,
		Status:         "partial",
		DueDate:        issued.AddDate(0, 0, 7),
		IssuedAt:       &issued,
	}
	items := []store.InvoiceItem{
		{Description: "Paket Home 30Mbps", Quantity: 1, UnitPrice: 150000, Amount: 150000},
		{Description: "Denda keterlambatan", Quantity: 1, UnitPrice: 5000, Amount: 5000},
	}
	opts := RenderOptions{
		Settings: store.InvoiceSettings{
			CompanyName:         "PT Internet Cepat",
			Address:             "Jl. Merdeka No. 1\nJakarta",
			Phone:               "021-555-0100",
			Email:               "billing@cepat.id",
			PaymentInstructions: "Transfer BCA 1234567890 a.n. PT Internet Cepat",
			FooterNote:          "Terima kasih telah berlangganan.",
		},
		FallbackCompany: "Fallback ISP",
		CustomerAddress: "Jl. Kenanga No. 5",
		CustomerPhone:   "08123456789",
	}

	out := RenderPDF(inv, items, opts)
	if !bytes.HasPrefix(out, []byte("%PDF-1.")) {
		t.Fatalf("missing PDF header: %q", out[:min(16, len(out))])
	}
	if !bytes.Contains(out, []byte("%%EOF")) {
		t.Fatal("missing PDF EOF marker")
	}
	if len(out) < 400 {
		t.Fatalf("pdf too small: %d bytes", len(out))
	}
}

func TestRenderPDFEmbedsLogo(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			img.Set(x, y, color.RGBA{R: 90, G: 90, B: 64, A: 255})
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	logo := EncodeLogoJPEG(pngBuf.Bytes())
	if logo == nil || len(logo.Data) == 0 {
		t.Fatal("expected JPEG logo")
	}
	inv := &store.Invoice{
		InvoiceNumber: "INV-LOGO",
		CustomerName:  "Budi",
		TotalAmount:   10000,
		Status:        "issued",
		DueDate:       time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
	}
	out := RenderPDF(inv, nil, RenderOptions{
		Settings:        store.InvoiceSettings{CompanyName: "PT Logo"},
		FallbackCompany: "ISP",
		Logo:            logo,
	})
	if !bytes.Contains(out, []byte("/Im1")) || !bytes.Contains(out, []byte("/DCTDecode")) {
		t.Fatal("expected embedded JPEG XObject")
	}
}

func TestRenderPDFPaidWatermark(t *testing.T) {
	inv := &store.Invoice{
		InvoiceNumber: "INV-PAID",
		CustomerName:  "Budi",
		TotalAmount:   10000,
		PaidAmount:    10000,
		Status:        "paid",
		DueDate:       time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
	}
	out := RenderPDF(inv, nil, RenderOptions{FallbackCompany: "ISP"})
	if !bytes.Contains(out, []byte("(LUNAS)")) {
		t.Fatal("expected LUNAS watermark on paid invoice")
	}
	if !bytes.Contains(out, []byte("/ExtGState")) {
		t.Fatal("expected ExtGState for watermark opacity")
	}
	unpaid := *inv
	unpaid.Status = "issued"
	unpaid.PaidAmount = 0
	plain := RenderPDF(&unpaid, nil, RenderOptions{FallbackCompany: "ISP"})
	if bytes.Contains(plain, []byte("(LUNAS)")) {
		t.Fatal("unpaid invoice must not have LUNAS watermark")
	}
}

func TestRenderPDFCropsEmptyA4(t *testing.T) {
	inv := &store.Invoice{
		InvoiceNumber: "INV-SHORT",
		CustomerName:  "Budi",
		TotalAmount:   10000,
		Status:        "issued",
		DueDate:       time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
	}
	out := RenderPDF(inv, []store.InvoiceItem{
		{Description: "Paket 30Mbps", Quantity: 1, UnitPrice: 150000, Amount: 150000},
	}, RenderOptions{FallbackCompany: "ISP"})
	if bytes.Contains(out, []byte("/MediaBox [0 0.0 595 842]")) || bytes.Contains(out, []byte("/MediaBox [0 0 595 842]")) {
		t.Fatal("short invoice should not use a full empty A4 page")
	}
}

func TestRupiahGrouping(t *testing.T) {
	cases := map[int64]string{
		0:       "Rp 0",
		1500:    "Rp 1.500",
		150000:  "Rp 150.000",
		1234567: "Rp 1.234.567",
		-2000:   "-Rp 2.000",
	}
	for in, want := range cases {
		if got := rupiah(in); got != want {
			t.Errorf("rupiah(%d) = %q, want %q", in, got, want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
