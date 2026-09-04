package billing

import (
	"testing"
	"time"

	"github.com/dianrp/drp-billing/internal/store"
)

func TestProrateAmount(t *testing.T) {
	if ProrateAmount(300_000, 15, 30) != 150_000 {
		t.Fatal("half month")
	}
	if ProrateAmount(100, 0, 0) != 100 {
		t.Fatal("zero period")
	}
}

func TestCalculateInvoiceAmount(t *testing.T) {
	e := New(nil)
	plan := &store.Plan{Price: 100_000, TaxPercent: 11}
	sub, tax := e.CalculateInvoiceAmount(plan, 0, 0)
	if sub != 100000 || tax != 11000 {
		t.Fatalf("sub=%d tax=%d", sub, tax)
	}
	sub, _ = e.CalculateInvoiceAmount(plan, 15, 30)
	if sub != 50000 {
		t.Fatalf("prorate sub=%d", sub)
	}
}

func TestApplyLateFee(t *testing.T) {
	if ApplyLateFee(100000, 10) != 10000 {
		t.Fatal("late fee")
	}
}

func TestNextBillDate(t *testing.T) {
	e := New(nil)
	from := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if e.NextBillDate(from, "monthly").Month() != 2 {
		t.Fatal("monthly")
	}
	if e.NextBillDate(from, "daily").Day() != 16 {
		t.Fatal("daily")
	}
}
