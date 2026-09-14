package store

import (
	"testing"
)

func TestPaymentCategory(t *testing.T) {
	cases := map[string]string{
		"Langganan Paket Home 30Mbps - user": PaymentCategorySubscription,
		"Paket 50Mbps":                       PaymentCategorySubscription,
		"Denda keterlambatan 5%":             PaymentCategoryLateFee,
		"Biaya instalasi baru":               PaymentCategoryInstallation,
		"Pajak PPN 11%":                      PaymentCategoryTax,
		"Biaya lain-lain":                    PaymentCategoryOther,
		"":                                   PaymentCategoryOther,
	}
	for desc, want := range cases {
		if got := PaymentCategory(desc); got != want {
			t.Errorf("PaymentCategory(%q) = %q, want %q", desc, got, want)
		}
	}
}

func TestAllocatePaymentSingleItem(t *testing.T) {
	got := allocatePaymentToCategories(150000, []InvoiceItem{{Description: "Langganan Paket", Amount: 150000}})
	if len(got) != 1 || got[0].Category != PaymentCategorySubscription || got[0].Amount != 150000 {
		t.Fatalf("unexpected allocation: %+v", got)
	}
}

func TestAllocatePaymentSplitsAndKeepsTotal(t *testing.T) {
	items := []InvoiceItem{
		{Description: "Langganan Paket Home", Amount: 150000},
		{Description: "Denda keterlambatan", Amount: 5000},
	}
	got := allocatePaymentToCategories(155000, items)
	var sum int64
	byCat := map[string]int64{}
	for _, ca := range got {
		sum += ca.Amount
		byCat[ca.Category] += ca.Amount
	}
	if sum != 155000 {
		t.Fatalf("total alokasi = %d, want 155000", sum)
	}
	if byCat[PaymentCategorySubscription] != 150000 || byCat[PaymentCategoryLateFee] != 5000 {
		t.Fatalf("split salah: %+v", byCat)
	}
}

func TestAllocatePaymentRoundingKeepsTotal(t *testing.T) {
	items := []InvoiceItem{
		{Description: "Langganan", Amount: 10000},
		{Description: "Instalasi", Amount: 10000},
		{Description: "Denda", Amount: 10000},
	}
	got := allocatePaymentToCategories(100000, items)
	var sum int64
	for _, ca := range got {
		sum += ca.Amount
	}
	if sum != 100000 {
		t.Fatalf("total alokasi = %d, want 100000", sum)
	}
}

func TestAllocatePaymentNoItems(t *testing.T) {
	got := allocatePaymentToCategories(50000, nil)
	if len(got) != 1 || got[0].Category != PaymentCategoryOther || got[0].Amount != 50000 {
		t.Fatalf("unexpected allocation: %+v", got)
	}
	if n := allocatePaymentToCategories(0, nil); n != nil {
		t.Fatalf("zero payment harus kosong, got %+v", n)
	}
}
