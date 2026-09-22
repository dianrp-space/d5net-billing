package invoice

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/store"
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
	if !bytes.Contains(out, []byte("0.12 0.25 0.69")) {
		t.Fatal("expected blue stamp color on LUNAS watermark")
	}
	if !bytes.Contains(out, []byte("/ca 0.14")) {
		t.Fatal("expected brighter LUNAS watermark opacity")
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

func TestRenderPDFAlwaysShowsTax(t *testing.T) {
	inv := &store.Invoice{
		InvoiceNumber: "INV-TAX0",
		CustomerName:  "Budi",
		Subtotal:      150000,
		TaxAmount:     0,
		TotalAmount:   150000,
		Status:        "issued",
		DueDate:       time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
	}
	out := RenderPDF(inv, []store.InvoiceItem{
		{Description: "Paket 30Mbps", Quantity: 1, UnitPrice: 150000, Amount: 150000},
	}, RenderOptions{FallbackCompany: "ISP"})
	if !bytes.Contains(out, []byte("(Pajak)")) {
		t.Fatal("tax line must render even when TaxAmount is 0")
	}
}

func TestRenderPDFMetaIsLabeled(t *testing.T) {
	issued := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	inv := &store.Invoice{
		InvoiceNumber: "INV-BTC-2026090001-0920265633L9",
		CustomerName:  "Budi Santoso",
		TotalAmount:   34232,
		PaidAmount:    34232,
		Status:        "paid",
		DueDate:       issued.AddDate(0, 0, 27),
		IssuedAt:      &issued,
	}
	out := RenderPDF(inv, []store.InvoiceItem{
		{Description: "Tes 3", Quantity: 1, UnitPrice: 34232, Amount: 34232},
	}, RenderOptions{
		Settings: store.InvoiceSettings{
			CompanyName:         "Delima Net",
			Address:             "Villa Tamansari Raudha Blok D 5",
			PaymentInstructions: "Login portal https://delimanet.dianrp.com",
			FooterNote:          "Terima kasih telah menjadi pelanggan Delima Net.",
		},
		FallbackCompany: "Delima Net",
	})
	for _, want := range []string{"(Nomor)", "(Terbit)", "(Jatuh tempo)", "(Status)", "(Deskripsi)", "(INVOICE)"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("missing %s", want)
		}
	}
	if !bytes.Contains(out, []byte("INV-BTC-2026090001")) || !bytes.Contains(out, []byte("0920265633L9")) {
		t.Fatal("long invoice number must not be truncated")
	}
}

func TestRenderPDFWithAdminFee(t *testing.T) {
	issued := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	inv := &store.Invoice{
		InvoiceNumber: "INV-2026-FEE",
		CustomerName:  "Budi",
		Subtotal:      100000,
		TotalAmount:   100000,
		Status:        "issued",
		DueDate:       issued.AddDate(0, 0, 7),
		IssuedAt:      &issued,
	}
	out := RenderPDF(inv, []store.InvoiceItem{
		{Description: "Paket 30Mbps", Quantity: 1, UnitPrice: 100000, Amount: 100000},
	}, RenderOptions{
		Settings: store.InvoiceSettings{CompanyName: "ISP"},
		AdminFee: 2000,
	})
	if !bytes.HasPrefix(out, []byte("%PDF-1.")) {
		t.Fatal("not a pdf")
	}
	if len(out) < 800 {
		t.Fatalf("pdf too small with admin fee: %d", len(out))
	}
}

func TestRenderPDFCustomColors(t *testing.T) {
	inv := &store.Invoice{
		InvoiceNumber: "INV-COLOR",
		CustomerName:  "Budi",
		TotalAmount:   10000,
		PaidAmount:    10000,
		Status:        "paid",
		DueDate:       time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
	}
	out := RenderPDF(inv, []store.InvoiceItem{
		{Description: "Paket 30Mbps", Quantity: 1, UnitPrice: 10000, Amount: 10000},
	}, RenderOptions{
		Settings: store.NormalizeInvoiceSettings(store.InvoiceSettings{
			CompanyName: "PT Warna",
			AccentColor: "#ff0000",
			StampColor:  "#00ff00",
		}),
		FallbackCompany: "ISP",
	})
	if !bytes.Contains(out, []byte("1.00 0.00 0.00")) {
		t.Fatal("expected custom accent red in pdf")
	}
	if !bytes.Contains(out, []byte("0.00 1.00 0.00")) {
		t.Fatal("expected custom stamp green in pdf")
	}
	if bytes.Contains(out, []byte("0.12 0.25 0.69")) {
		t.Fatal("default stamp blue must not appear when custom stamp set")
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
