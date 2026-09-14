package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// Kategori pendapatan untuk laporan pembayaran pelanggan. Diturunkan dari
// deskripsi item invoice karena item belum punya kolom kategori sendiri.
const (
	PaymentCategorySubscription = "Langganan"
	PaymentCategoryInstallation = "Instalasi"
	PaymentCategoryLateFee      = "Denda"
	PaymentCategoryTax          = "Pajak"
	PaymentCategoryOther        = "Lainnya"
)

// PaymentCategory mengklasifikasikan deskripsi item invoice ke bucket pendapatan.
func PaymentCategory(desc string) string {
	d := strings.ToLower(strings.TrimSpace(desc))
	switch {
	case d == "":
		return PaymentCategoryOther
	case strings.Contains(d, "denda") || strings.Contains(d, "keterlambatan") || strings.Contains(d, "late"):
		return PaymentCategoryLateFee
	case strings.Contains(d, "pajak") || strings.Contains(d, "ppn") || strings.Contains(d, "tax"):
		return PaymentCategoryTax
	case strings.Contains(d, "instalasi") || strings.Contains(d, "pemasangan") || strings.Contains(d, "aktivasi") || strings.Contains(d, "installation"):
		return PaymentCategoryInstallation
	case strings.Contains(d, "langganan") || strings.Contains(d, "paket") || strings.Contains(d, "abonemen") ||
		strings.Contains(d, "subscription") || strings.Contains(d, "internet") || strings.Contains(d, "bulanan"):
		return PaymentCategorySubscription
	default:
		return PaymentCategoryOther
	}
}

// CustomerPaymentFilter adalah parameter laporan pembayaran pelanggan.
type CustomerPaymentFilter struct {
	From           time.Time
	To             time.Time
	CustomerID     *xid.ID
	Method         string
	Category       string
	Search         string
	IncludeSandbox bool
	SandboxOnly    bool
}

// CustomerPaymentRow adalah satu baris pendapatan: bagian dari sebuah pembayaran
// yang dialokasikan ke satu kategori.
type CustomerPaymentRow struct {
	Date          time.Time `json:"date"`
	CustomerID    xid.ID    `json:"customer_id"`
	CustomerCode  string    `json:"customer_code"`
	CustomerName  string    `json:"customer_name"`
	InvoiceNumber string    `json:"invoice_number,omitempty"`
	Category      string    `json:"category"`
	Method        string    `json:"method"`
	Reference     string    `json:"reference,omitempty"`
	Sandbox       bool      `json:"sandbox"`
	Amount        int64     `json:"amount"`
}

type CustomerPaymentSummary struct {
	CustomerID   xid.ID `json:"customer_id"`
	CustomerCode string `json:"customer_code"`
	CustomerName string `json:"customer_name"`
	Count        int    `json:"count"`
	Amount       int64  `json:"amount"`
}

type CategoryPaymentSummary struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
	Amount   int64  `json:"amount"`
}

type CustomerPaymentReport struct {
	From        string                   `json:"from"`
	To          string                   `json:"to"`
	TotalCount  int                      `json:"total_count"`
	TotalAmount int64                    `json:"total_amount"`
	Data        []CustomerPaymentRow     `json:"data"`
	ByCustomer  []CustomerPaymentSummary `json:"by_customer"`
	ByCategory  []CategoryPaymentSummary `json:"by_category"`
}

// CategoryAmount adalah nominal yang dialokasikan ke satu kategori.
type CategoryAmount struct {
	Category string
	Amount   int64
}

// allocatePaymentToCategories membagi satu pembayaran ke kategori berdasarkan
// bobot nominal item invoice (metode largest remainder agar total tetap utuh).
// Invoice tanpa item atau item tanpa nominal dikembalikan sebagai "Lainnya".
func allocatePaymentToCategories(amount int64, items []InvoiceItem) []CategoryAmount {
	cat := func() string {
		if len(items) > 0 {
			return PaymentCategory(items[0].Description)
		}
		return PaymentCategoryOther
	}
	if amount == 0 {
		return nil
	}
	var total int64
	byCat := map[string]int64{}
	order := []string{}
	for _, it := range items {
		if it.Amount <= 0 {
			continue
		}
		c := PaymentCategory(it.Description)
		if _, ok := byCat[c]; !ok {
			order = append(order, c)
		}
		byCat[c] += it.Amount
		total += it.Amount
	}
	if amount <= 0 || total <= 0 || len(order) == 0 {
		return []CategoryAmount{{Category: cat(), Amount: amount}}
	}
	type alloc struct {
		category string
		amount   int64
		rem      float64
	}
	allocs := make([]alloc, 0, len(order))
	var assigned int64
	for _, c := range order {
		exact := float64(amount) * float64(byCat[c]) / float64(total)
		base := int64(exact)
		if exact < 0 {
			base = int64(exact - 0.5)
		}
		assigned += base
		allocs = append(allocs, alloc{category: c, amount: base, rem: exact - float64(base)})
	}
	// Distribusikan sisa pembulatan ke kategori dengan pecahan terbesar.
	leftover := amount - assigned
	for leftover != 0 {
		idx := 0
		for i := 1; i < len(allocs); i++ {
			if allocs[i].rem > allocs[idx].rem {
				idx = i
			}
		}
		step := int64(1)
		if leftover < 0 {
			step = -1
		}
		allocs[idx].amount += step
		allocs[idx].rem = 0
		leftover -= step
	}
	out := make([]CategoryAmount, 0, len(allocs))
	for _, a := range allocs {
		if a.amount != 0 {
			out = append(out, CategoryAmount{Category: a.category, Amount: a.amount})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Amount != out[j].Amount {
			return out[i].Amount > out[j].Amount
		}
		return out[i].Category < out[j].Category
	})
	return out
}

// CustomerPaymentReport menyusun laporan detail pembayaran pelanggan per
// kategori pemasukan, lengkap dengan subtotal per pelanggan dan per kategori.
func (s *Store) CustomerPaymentReport(ctx context.Context, tenantID xid.ID, f CustomerPaymentFilter) (*CustomerPaymentReport, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	now := time.Now()
	from := f.From
	if from.IsZero() {
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}
	to := f.To
	if to.IsZero() {
		to = now
	}
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location()).AddDate(0, 0, 1)
	if end.Before(start) {
		start, end = end.AddDate(0, 0, -1), start.AddDate(0, 0, 1)
	}

	where := `WHERE p.tenant_id=$1 AND p.deleted_at IS NULL
		AND LOWER(p.status) IN ('paid','success')
		AND COALESCE(p.paid_at, p.created_at) >= $2
		AND COALESCE(p.paid_at, p.created_at) < $3`
	args := []any{tenantID, start, end}
	if f.SandboxOnly {
		where += ` AND p.sandbox = true`
	} else if !f.IncludeSandbox {
		where += ` AND p.sandbox = false`
	}
	if f.CustomerID != nil && !xid.IsNil(*f.CustomerID) {
		args = append(args, *f.CustomerID)
		where += fmt.Sprintf(` AND p.customer_id = $%d`, len(args))
	}
	if m := strings.TrimSpace(f.Method); m != "" {
		args = append(args, m)
		where += fmt.Sprintf(` AND LOWER(p.method) = LOWER($%d)`, len(args))
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		args = append(args, "%"+q+"%")
		n := len(args)
		where += fmt.Sprintf(` AND (c.full_name ILIKE $%[1]d OR c.customer_code ILIKE $%[1]d
			OR COALESCE(i.invoice_number,'') ILIKE $%[1]d OR COALESCE(p.reference,'') ILIKE $%[1]d)`, n)
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT p.customer_id, p.amount, p.method, COALESCE(p.reference,''), p.sandbox,
		       COALESCE(p.paid_at, p.created_at),
		       COALESCE(c.customer_code,''), COALESCE(c.full_name,''),
		       i.id, COALESCE(i.invoice_number,'')
		FROM payments p
		LEFT JOIN customers c ON c.id = p.customer_id AND c.tenant_id = p.tenant_id
		LEFT JOIN invoices i ON i.id = p.invoice_id AND i.tenant_id = p.tenant_id
		`+where+`
		ORDER BY COALESCE(p.paid_at, p.created_at) DESC, p.id DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rawPayment struct {
		customerID    xid.ID
		amount        int64
		method        string
		reference     string
		sandbox       bool
		date          time.Time
		customerCode  string
		customerName  string
		invoiceID     *xid.ID
		invoiceNumber string
	}
	var raw []rawPayment
	invoiceIDs := make([]xid.ID, 0)
	for rows.Next() {
		var rp rawPayment
		if err := rows.Scan(&rp.customerID, &rp.amount, &rp.method, &rp.reference, &rp.sandbox, &rp.date,
			&rp.customerCode, &rp.customerName, &rp.invoiceID, &rp.invoiceNumber); err != nil {
			return nil, err
		}
		raw = append(raw, rp)
		if rp.invoiceID != nil && !xid.IsNil(*rp.invoiceID) {
			invoiceIDs = append(invoiceIDs, *rp.invoiceID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	itemsMap, err := s.InvoiceItemsMap(ctx, tenantID, invoiceIDs)
	if err != nil {
		return nil, err
	}

	report := &CustomerPaymentReport{
		From:       start.Format("2006-01-02"),
		To:         time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location()).Format("2006-01-02"),
		Data:       []CustomerPaymentRow{},
		ByCustomer: []CustomerPaymentSummary{},
		ByCategory: []CategoryPaymentSummary{},
	}
	categoryFilter := strings.TrimSpace(f.Category)
	seenCustomer := map[xid.ID]int{}
	seenCategory := map[string]int{}

	for _, rp := range raw {
		var items []InvoiceItem
		if rp.invoiceID != nil {
			items = itemsMap[*rp.invoiceID]
		}
		for _, ca := range allocatePaymentToCategories(rp.amount, items) {
			cat := ca.Category
			amt := ca.Amount
			if categoryFilter != "" && !strings.EqualFold(cat, categoryFilter) {
				continue
			}
			report.Data = append(report.Data, CustomerPaymentRow{
				Date:          rp.date,
				CustomerID:    rp.customerID,
				CustomerCode:  rp.customerCode,
				CustomerName:  rp.customerName,
				InvoiceNumber: rp.invoiceNumber,
				Category:      cat,
				Method:        rp.method,
				Reference:     rp.reference,
				Sandbox:       rp.sandbox,
				Amount:        amt,
			})
			report.TotalCount++
			report.TotalAmount += amt
			if i, ok := seenCustomer[rp.customerID]; ok {
				report.ByCustomer[i].Count++
				report.ByCustomer[i].Amount += amt
			} else {
				seenCustomer[rp.customerID] = len(report.ByCustomer)
				report.ByCustomer = append(report.ByCustomer, CustomerPaymentSummary{
					CustomerID:   rp.customerID,
					CustomerCode: rp.customerCode,
					CustomerName: rp.customerName,
					Count:        1,
					Amount:       amt,
				})
			}
			if i, ok := seenCategory[cat]; ok {
				report.ByCategory[i].Count++
				report.ByCategory[i].Amount += amt
			} else {
				seenCategory[cat] = len(report.ByCategory)
				report.ByCategory = append(report.ByCategory, CategoryPaymentSummary{
					Category: cat,
					Count:    1,
					Amount:   amt,
				})
			}
		}
	}

	sort.SliceStable(report.ByCustomer, func(i, j int) bool {
		if report.ByCustomer[i].Amount != report.ByCustomer[j].Amount {
			return report.ByCustomer[i].Amount > report.ByCustomer[j].Amount
		}
		return report.ByCustomer[i].CustomerName < report.ByCustomer[j].CustomerName
	})
	sort.SliceStable(report.ByCategory, func(i, j int) bool {
		return report.ByCategory[i].Amount > report.ByCategory[j].Amount
	})
	return report, nil
}
