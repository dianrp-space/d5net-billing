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

	nextBill := e.NextBillDate(time.Now(), plan.BillingCycle)
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
	var suspended []xid.ID
	for _, sub := range subs {
		if err := e.store.UpdateSubscriptionStatus(ctx, tenantID, sub.ID, "suspended"); err == nil {
			suspended = append(suspended, sub.ID)
		}
	}
	return suspended, nil
}

func ProrateAmount(fullPrice int64, daysUsed, daysInPeriod int) int64 {
	if daysInPeriod <= 0 {
		return fullPrice
	}
	return int64(math.Round(float64(fullPrice) * float64(daysUsed) / float64(daysInPeriod)))
}

func ApplyLateFee(amount int64, percent float64) int64 {
	return int64(math.Round(float64(amount) * percent / 100))
}
