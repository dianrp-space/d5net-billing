package billing

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

const defaultLateFeePercent = 5.0

type Engine struct {
	store *store.Store
}

func New(st *store.Store) *Engine {
	return &Engine{store: st}
}

func daysInBillingCycle(cycle string) int {
	switch cycle {
	case "daily":
		return 1
	case "weekly":
		return 7
	case "yearly":
		return 365
	default:
		return 30
	}
}

func (e *Engine) CalculateInvoiceAmount(plan *store.Plan, prorateDays, totalDays int) (subtotal, tax int64) {
	subtotal = plan.Price
	if prorateDays > 0 && totalDays > 0 && prorateDays < totalDays {
		subtotal = int64(math.Round(float64(plan.Price) * float64(prorateDays) / float64(totalDays)))
	}
	tax = int64(math.Round(float64(subtotal) * plan.TaxPercent / 100))
	return subtotal, tax
}

func (e *Engine) GenerateInvoiceForSubscription(ctx context.Context, tenantID xid.ID, subscriptionID xid.ID) (*store.Invoice, error) {
	sub, err := e.store.GetSubscription(ctx, tenantID, subscriptionID)
	if err != nil {
		return nil, err
	}
	plan, err := e.store.GetPlan(ctx, tenantID, sub.PlanID)
	if err != nil {
		return nil, err
	}

	cust, err := e.store.GetCustomer(ctx, tenantID, sub.CustomerID)
	if err != nil {
		return nil, err
	}
	if price, err := e.store.ResolvePlanPrice(ctx, tenantID, plan.ID, cust.ClusterID); err == nil {
		plan.Price = price
	}
	if pct, err := e.store.EffectiveTaxPercent(ctx, tenantID); err == nil {
		plan.TaxPercent = pct
	}

	invNum, err := e.store.NextInvoiceNumber(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	cycleDays := daysInBillingCycle(plan.BillingCycle)
	prorateDays, periodDays := 0, 0 // 0,0 → full price in CalculateInvoiceAmount

	var priorCount int64
	_ = e.store.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM invoices WHERE tenant_id=$1 AND subscription_id=$2
	`, tenantID, subscriptionID).Scan(&priorCount)

	// First invoice after activation: charge only remaining days until next_bill_at.
	if priorCount == 0 && sub.StartedAt != nil {
		periodDays = cycleDays
		until := e.NextBillDate(*sub.StartedAt, plan.BillingCycle)
		if sub.NextBillAt != nil {
			until = *sub.NextBillAt
		}
		prorateDays = int(math.Ceil(until.Sub(*sub.StartedAt).Hours() / 24))
		if prorateDays < 1 {
			prorateDays = 1
		}
		if prorateDays > periodDays {
			prorateDays = periodDays
		}
	}

	subtotal, tax := e.CalculateInvoiceAmount(plan, prorateDays, periodDays)

	var lateFee int64
	overdue, err := e.store.SumOverdueUnpaidForSubscription(ctx, tenantID, subscriptionID)
	if err == nil && overdue > 0 {
		lateFee = ApplyLateFee(overdue, defaultLateFeePercent)
	}

	dueDate := time.Now().AddDate(0, 0, plan.GraceDays)
	inv := &store.Invoice{
		TenantID:       tenantID,
		CustomerID:     sub.CustomerID,
		SubscriptionID: &subscriptionID,
		InvoiceNumber:  invNum,
		Subtotal:       subtotal + lateFee,
		TaxAmount:      tax,
		TotalAmount:    subtotal + tax + lateFee,
		Status:         "issued",
		DueDate:        dueDate,
	}

	items := []store.InvoiceItem{{
		Description: "Langganan " + plan.Name + " - " + sub.Username,
		Quantity:    1,
		UnitPrice:   subtotal,
		Amount:      subtotal,
	}}
	if priorCount == 0 && prorateDays > 0 && periodDays > 0 && prorateDays < periodDays {
		items[0].Description += " (prorata " + strconv.Itoa(prorateDays) + "/" + strconv.Itoa(periodDays) + " hari)"
	}
	if lateFee > 0 {
		items = append(items, store.InvoiceItem{
			Description: fmt.Sprintf("Denda keterlambatan %.0f%%", defaultLateFeePercent),
			Quantity:    1,
			UnitPrice:   lateFee,
			Amount:      lateFee,
		})
	}

	if err := e.store.CreateInvoice(ctx, inv, items); err != nil {
		return nil, err
	}

	// Advance / keep billing clock.
	// First (prorated) invoice: keep next_bill_at as the period end / billing anchor.
	// Later invoices: step forward from the previous next_bill_at.
	var nextBill time.Time
	if priorCount == 0 && sub.NextBillAt != nil {
		nextBill = *sub.NextBillAt
	} else if sub.NextBillAt != nil {
		nextBill = e.NextBillDate(*sub.NextBillAt, plan.BillingCycle)
	} else {
		nextBill = e.NextBillDate(time.Now(), plan.BillingCycle)
	}
	_, _ = e.store.Pool.Exec(ctx, `UPDATE subscriptions SET next_bill_at = $3 WHERE tenant_id=$1 AND id=$2`, tenantID, subscriptionID, nextBill)
	return inv, nil
}

func (e *Engine) NextBillDate(from time.Time, cycle string) time.Time {
	switch cycle {
	case "daily":
		return from.AddDate(0, 0, 1)
	case "weekly":
		return from.AddDate(0, 0, 7)
	case "yearly":
		return from.AddDate(1, 0, 0)
	default:
		return from.AddDate(0, 1, 0)
	}
}

// NextCycleAnchor is the next occurrence of calendar day (1–28) after from.
// Activating on that day itself returns the same day next month.
func NextCycleAnchor(from time.Time, day int) time.Time {
	if day < 1 {
		day = 1
	}
	if day > 28 {
		day = 28
	}
	loc := from.Location()
	if loc == nil {
		loc = time.Local
	}
	from = time.Date(from.Year(), from.Month(), from.Day(), 12, 0, 0, 0, loc)
	candidate := time.Date(from.Year(), from.Month(), day, 12, 0, 0, 0, loc)
	if candidate.After(from) {
		return candidate
	}
	return time.Date(from.Year(), from.Month()+1, day, 12, 0, 0, 0, loc)
}

func (e *Engine) ProcessDueBilling(ctx context.Context, tenantID xid.ID) (int, error) {
	subs, err := e.store.ListDueSubscriptions(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, sub := range subs {
		if _, err := e.GenerateInvoiceForSubscription(ctx, tenantID, sub.ID); err == nil {
			count++
		}
	}
	return count, nil
}

func (e *Engine) ProcessOverdueSuspensions(ctx context.Context, tenantID xid.ID) ([]xid.ID, error) {
	subs, err := e.store.ListOverdueSubscriptions(ctx, tenantID, 0)
	if err != nil {
		return nil, err
	}
	var ids []xid.ID
	for _, sub := range subs {
		ids = append(ids, sub.ID)
	}
	return ids, nil
}

func ProrateAmount(fullPrice int64, daysUsed, daysInPeriod int) int64 {
	if daysInPeriod <= 0 {
		return fullPrice
	}
	return int64(math.Round(float64(fullPrice) * float64(daysUsed) / float64(daysInPeriod)))
}

func ApplyLateFee(amount int64, percent float64) int64 {
	if amount <= 0 || percent <= 0 {
		return 0
	}
	return int64(math.Round(float64(amount) * percent / 100))
}

// PlanChangeQuote is the mid-cycle charge when switching plans before next_bill_at.
// Formula: delta = prorata(new) − prorata(old) for remaining days in the current cycle.
type PlanChangeQuote struct {
	OldPlanID      xid.ID `json:"old_plan_id"`
	OldPlanName    string `json:"old_plan_name"`
	OldPrice       int64  `json:"old_price"`
	NewPlanID      xid.ID `json:"new_plan_id"`
	NewPlanName    string `json:"new_plan_name"`
	NewPrice       int64  `json:"new_price"`
	RemainingDays  int    `json:"remaining_days"`
	PeriodDays     int    `json:"period_days"`
	OldCredit      int64  `json:"old_credit"`
	NewCharge      int64  `json:"new_charge"`
	DeltaSubtotal  int64  `json:"delta_subtotal"`
	TaxAmount      int64  `json:"tax_amount"`
	TotalAmount    int64  `json:"total_amount"`
	Direction      string `json:"direction"` // upgrade | downgrade | same
	NextBillAt     string `json:"next_bill_at,omitempty"`
	RequiresCharge bool   `json:"requires_charge"`
}

func RemainingDaysUntil(now, until time.Time, cycleDays int) int {
	if !until.After(now) {
		return 0
	}
	days := int(math.Ceil(until.Sub(now).Hours() / 24))
	if days < 1 {
		days = 1
	}
	if cycleDays > 0 && days > cycleDays {
		days = cycleDays
	}
	return days
}

func (e *Engine) QuotePlanChange(ctx context.Context, tenantID, subscriptionID, newPlanID xid.ID, at time.Time) (*PlanChangeQuote, *store.Subscription, *store.Plan, *store.Plan, error) {
	sub, err := e.store.GetSubscription(ctx, tenantID, subscriptionID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	oldPlan, err := e.store.GetPlan(ctx, tenantID, sub.PlanID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	newPlan, err := e.store.GetPlan(ctx, tenantID, newPlanID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	cust, err := e.store.GetCustomer(ctx, tenantID, sub.CustomerID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	oldPrice := oldPlan.Price
	if p, err := e.store.ResolvePlanPrice(ctx, tenantID, oldPlan.ID, cust.ClusterID); err == nil {
		oldPrice = p
	}
	newPrice := newPlan.Price
	if p, err := e.store.ResolvePlanPrice(ctx, tenantID, newPlan.ID, cust.ClusterID); err == nil {
		newPrice = p
	}

	cycle := oldPlan.BillingCycle
	if cycle == "" {
		cycle = newPlan.BillingCycle
	}
	cycleDays := daysInBillingCycle(cycle)

	until := at.AddDate(0, 1, 0)
	if sub.NextBillAt != nil {
		until = *sub.NextBillAt
	} else if sub.StartedAt != nil {
		until = e.NextBillDate(*sub.StartedAt, cycle)
	}
	remaining := RemainingDaysUntil(at, until, cycleDays)

	oldCredit := ProrateAmount(oldPrice, remaining, cycleDays)
	newCharge := ProrateAmount(newPrice, remaining, cycleDays)
	delta := newCharge - oldCredit
	if remaining <= 0 {
		oldCredit, newCharge, delta = 0, 0, 0
	}

	dir := "same"
	switch {
	case newPrice > oldPrice:
		dir = "upgrade"
	case newPrice < oldPrice:
		dir = "downgrade"
	}

	tax := int64(0)
	total := int64(0)
	requires := false
	if delta > 0 {
		taxPct := newPlan.TaxPercent
		if pct, err := e.store.EffectiveTaxPercent(ctx, tenantID); err == nil {
			taxPct = pct
		}
		tax = int64(math.Round(float64(delta) * taxPct / 100))
		total = delta + tax
		requires = true
	}

	q := &PlanChangeQuote{
		OldPlanID:      oldPlan.ID,
		OldPlanName:    oldPlan.Name,
		OldPrice:       oldPrice,
		NewPlanID:      newPlan.ID,
		NewPlanName:    newPlan.Name,
		NewPrice:       newPrice,
		RemainingDays:  remaining,
		PeriodDays:     cycleDays,
		OldCredit:      oldCredit,
		NewCharge:      newCharge,
		DeltaSubtotal:  delta,
		TaxAmount:      tax,
		TotalAmount:    total,
		Direction:      dir,
		NextBillAt:     until.Format(time.RFC3339),
		RequiresCharge: requires,
	}
	return q, sub, oldPlan, newPlan, nil
}

// ApplyPlanChange updates the subscription plan and optionally issues a mid-cycle delta invoice.
// next_bill_at is preserved (same billing anchor). Returns quote and invoice (nil if no charge).
func (e *Engine) ApplyPlanChange(ctx context.Context, tenantID, subscriptionID, newPlanID xid.ID) (*PlanChangeQuote, *store.Invoice, error) {
	now := time.Now()
	q, sub, _, newPlan, err := e.QuotePlanChange(ctx, tenantID, subscriptionID, newPlanID, now)
	if err != nil {
		return nil, nil, err
	}
	if sub.PlanID == newPlanID {
		return q, nil, fmt.Errorf("paket sama dengan paket saat ini")
	}
	switch sub.Status {
	case "active", "suspended", "overdue":
	default:
		return nil, nil, fmt.Errorf("ganti paket hanya untuk langganan active/suspended/overdue (status: %s)", sub.Status)
	}

	svc := newPlan.ServiceType
	if svc == "" {
		svc = sub.ServiceType
	}
	sub.PlanID = newPlanID
	sub.ServiceType = svc
	if err := e.store.UpdateSubscription(ctx, tenantID, sub, false); err != nil {
		return nil, nil, err
	}

	var inv *store.Invoice
	if q.RequiresCharge && q.DeltaSubtotal > 0 {
		invNum, err := e.store.NextInvoiceNumber(ctx, tenantID)
		if err != nil {
			return nil, nil, err
		}
		due := now.AddDate(0, 0, newPlan.GraceDays)
		sid := subscriptionID
		inv = &store.Invoice{
			TenantID:       tenantID,
			CustomerID:     sub.CustomerID,
			SubscriptionID: &sid,
			InvoiceNumber:  invNum,
			Subtotal:       q.DeltaSubtotal,
			TaxAmount:      q.TaxAmount,
			TotalAmount:    q.TotalAmount,
			Status:         "issued",
			DueDate:        due,
		}
		desc := fmt.Sprintf(
			"Ganti paket %s → %s (selisih prorata %d/%d hari)",
			q.OldPlanName, q.NewPlanName, q.RemainingDays, q.PeriodDays,
		)
		items := []store.InvoiceItem{{
			Description: desc,
			Quantity:    1,
			UnitPrice:   q.DeltaSubtotal,
			Amount:      q.DeltaSubtotal,
		}}
		if err := e.store.CreateInvoice(ctx, inv, items); err != nil {
			return nil, nil, err
		}
	}
	return q, inv, nil
}
