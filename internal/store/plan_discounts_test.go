package store

import (
	"testing"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

func TestApplyPlanDiscount(t *testing.T) {
	if got := ApplyPlanDiscount(100_000, nil); got != 100_000 {
		t.Fatalf("nil = %d", got)
	}
	pct := &PlanDiscount{Kind: DiscountKindPercent, Value: 10}
	if got := ApplyPlanDiscount(150_000, pct); got != 135_000 {
		t.Fatalf("10%% of 150k = %d", got)
	}
	amt := &PlanDiscount{Kind: DiscountKindAmount, Value: 25_000}
	if got := ApplyPlanDiscount(100_000, amt); got != 75_000 {
		t.Fatalf("amount = %d", got)
	}
	if got := ApplyPlanDiscount(10_000, amt); got != 0 {
		t.Fatalf("cap at zero = %d", got)
	}
}

func TestNormalizePlanDiscount(t *testing.T) {
	d := NormalizePlanDiscount(PlanDiscount{
		Name: "  Promo  ", Kind: "PERCENT", Value: 150, Audience: "selected",
	})
	if d.Name != "Promo" || d.Kind != DiscountKindPercent || d.Value != 100 {
		t.Fatalf("got %+v", d)
	}
	cid := xid.New()
	all := NormalizePlanDiscount(PlanDiscount{Audience: "all", CustomerIDs: []xid.ID{cid}})
	if all.Audience != DiscountAudienceAll || all.CustomerIDs != nil {
		t.Fatalf("all should drop customer ids: %+v", all)
	}
}

func TestDiscountSpecificity(t *testing.T) {
	planID := xid.New()
	if n := discountSpecificity(&PlanDiscount{Audience: DiscountAudienceAll}); n != 0 {
		t.Fatalf("all = %d", n)
	}
	if n := discountSpecificity(&PlanDiscount{Audience: DiscountAudiencePick, PlanID: &planID}); n != 3 {
		t.Fatalf("selected+plan = %d", n)
	}
}
