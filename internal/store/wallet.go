package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

// PaymentMethodWallet adalah metode pembayaran untuk pelunasan dari saldo.
const PaymentMethodWallet = "saldo"

type WalletTxn struct {
	ID          xid.ID    `json:"id"`
	Amount      int64     `json:"amount"`
	Type        string    `json:"type"`
	Reference   string    `json:"reference,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) ListWalletTxns(ctx context.Context, tenantID, customerID xid.ID, limit, offset int) ([]WalletTxn, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM wallet_transactions wt
		JOIN wallets w ON w.id = wt.wallet_id
		WHERE wt.tenant_id = $1 AND w.customer_id = $2
	`, tenantID, customerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT wt.id, wt.amount, wt.type, COALESCE(wt.reference,''), COALESCE(wt.description,''), wt.created_at
		FROM wallet_transactions wt
		JOIN wallets w ON w.id = wt.wallet_id
		WHERE wt.tenant_id = $1 AND w.customer_id = $2
		ORDER BY wt.created_at DESC, wt.id DESC
		LIMIT $3 OFFSET $4
	`, tenantID, customerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []WalletTxn{}
	for rows.Next() {
		var t WalletTxn
		if err := rows.Scan(&t.ID, &t.Amount, &t.Type, &t.Reference, &t.Description, &t.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, t)
	}
	return list, total, rows.Err()
}

// OutstandingInvoiceIDs mengembalikan tagihan belum lunas milik pelanggan,
// terurut dari yang paling lama (due_date, lalu terbit).
func (s *Store) OutstandingInvoiceIDs(ctx context.Context, tenantID, customerID xid.ID) ([]xid.ID, error) {
	return s.outstandingInvoiceIDs(ctx, tenantID, customerID, false)
}

// OutstandingSubscriptionInvoiceIDs sama seperti OutstandingInvoiceIDs tetapi
// hanya tagihan langganan (punya subscription_id). Dipakai auto-pay saldo agar
// tagihan manual tidak ikut terpotong otomatis.
func (s *Store) OutstandingSubscriptionInvoiceIDs(ctx context.Context, tenantID, customerID xid.ID) ([]xid.ID, error) {
	return s.outstandingInvoiceIDs(ctx, tenantID, customerID, true)
}

func (s *Store) outstandingInvoiceIDs(ctx context.Context, tenantID, customerID xid.ID, subscriptionOnly bool) ([]xid.ID, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	q := `
		SELECT id FROM invoices
		WHERE tenant_id=$1 AND customer_id=$2 AND deleted_at IS NULL
		  AND status IN ('issued','partial','overdue')
		  AND total_amount > paid_amount`
	if subscriptionOnly {
		q += ` AND subscription_id IS NOT NULL`
	}
	q += ` ORDER BY due_date ASC, issued_at ASC NULLS FIRST`
	rows, err := s.Pool.Query(ctx, q, tenantID, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []xid.ID{}
	for rows.Next() {
		var id xid.ID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func lookupAccountTx(ctx context.Context, tx pgx.Tx, tenantID xid.ID, typ, code string) xid.ID {
	var id xid.ID
	_ = tx.QueryRow(ctx, `
		SELECT id FROM chart_of_accounts
		WHERE tenant_id=$1 AND is_active=true AND (LOWER(type)=LOWER($2) OR code=$3)
		ORDER BY CASE WHEN code=$3 THEN 0 ELSE 1 END, code
		LIMIT 1
	`, tenantID, typ, code).Scan(&id)
	return id
}

func insertWalletTxnTx(ctx context.Context, tx pgx.Tx, tenantID, walletID xid.ID, amount int64, typ, ref, desc string) error {
	var refArg, descArg any
	if strings.TrimSpace(ref) != "" {
		refArg = ref
	}
	if strings.TrimSpace(desc) != "" {
		descArg = desc
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO wallet_transactions (tenant_id, wallet_id, amount, type, reference, description)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, tenantID, walletID, amount, typ, refArg, descArg)
	return err
}

// postWalletJournalTx mencatat jurnal dalam transaksi yang sama (best-effort
// bila akun tidak tersedia: jurnal dilewati agar saldo tetap konsisten).
func postWalletJournalTx(ctx context.Context, tx pgx.Tx, tenantID xid.ID, description, reference, sourceType string, sourceID *xid.ID, debitAccount, creditAccount xid.ID, amount int64) error {
	if amount <= 0 || xid.IsNil(debitAccount) || xid.IsNil(creditAccount) {
		return nil
	}
	var refArg, sourceTypeArg, sourceIDArg any
	if strings.TrimSpace(reference) != "" {
		refArg = reference
	}
	if strings.TrimSpace(sourceType) != "" {
		sourceTypeArg = sourceType
	}
	if sourceID != nil && !xid.IsNil(*sourceID) {
		sourceIDArg = *sourceID
	}
	var entryID xid.ID
	if err := tx.QueryRow(ctx, `
		INSERT INTO journal_entries (tenant_id, entry_date, reference, description, source_type, source_id)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id
	`, tenantID, time.Now().Format("2006-01-02"), refArg, description, sourceTypeArg, sourceIDArg).Scan(&entryID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO journal_lines (tenant_id, entry_id, account_id, debit, credit)
		VALUES ($1,$2,$3,$4,0), ($1,$2,$5,0,$4)
	`, tenantID, entryID, debitAccount, amount, creditAccount)
	return err
}

// TopupWallet menambah saldo pelanggan dan mencatat jurnal (debit Kas, kredit
// Saldo pelanggan). Mengembalikan saldo baru.
func (s *Store) TopupWallet(ctx context.Context, tenantID, customerID xid.ID, amount int64, txType, ref, desc string) (int64, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("nominal topup harus > 0")
	}
	if strings.TrimSpace(txType) == "" {
		txType = "topup"
	}
	var newBalance int64
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var walletID xid.ID
		if err := tx.QueryRow(ctx, `
			INSERT INTO wallets (tenant_id, customer_id, balance) VALUES ($1,$2,$3)
			ON CONFLICT (tenant_id, customer_id) DO UPDATE SET balance = wallets.balance + $3, updated_at = NOW()
			RETURNING id, balance
		`, tenantID, customerID, amount).Scan(&walletID, &newBalance); err != nil {
			return err
		}
		if err := insertWalletTxnTx(ctx, tx, tenantID, walletID, amount, txType, ref, desc); err != nil {
			return err
		}
		cash := lookupAccountTx(ctx, tx, tenantID, "asset", "1110")
		liability := lookupAccountTx(ctx, tx, tenantID, "liability", "2100")
		return postWalletJournalTx(ctx, tx, tenantID, "Topup saldo", ref, "wallet_topup", &customerID, cash, liability, amount)
	})
	return newBalance, err
}

// PayInvoiceFromWallet melunasi satu tagihan dari saldo secara atomik.
// paid=false (tanpa error) bila saldo tidak cukup atau tagihan sudah lunas.
func (s *Store) PayInvoiceFromWallet(ctx context.Context, tenantID, customerID, invoiceID xid.ID) (*Payment, bool, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, false, err
	}
	var payment Payment
	paid := false
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var total, paidAmount int64
		var status, invNum string
		if err := tx.QueryRow(ctx, `
			SELECT total_amount, paid_amount, status, invoice_number
			FROM invoices
			WHERE tenant_id=$1 AND customer_id=$2 AND id=$3 AND deleted_at IS NULL
			FOR UPDATE
		`, tenantID, customerID, invoiceID).Scan(&total, &paidAmount, &status, &invNum); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "paid", "void", "cancelled", "canceled":
			return nil
		}
		remaining := total - paidAmount
		if remaining <= 0 {
			return nil
		}
		var walletID xid.ID
		var balance int64
		if err := tx.QueryRow(ctx, `
			SELECT id, balance FROM wallets WHERE tenant_id=$1 AND customer_id=$2 FOR UPDATE
		`, tenantID, customerID).Scan(&walletID, &balance); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // belum punya saldo
			}
			return err
		}
		if balance < remaining {
			return nil // saldo kurang; tidak dipotong
		}
		if _, err := tx.Exec(ctx, `
			UPDATE wallets SET balance = balance - $3, updated_at = NOW() WHERE tenant_id=$1 AND id=$2
		`, tenantID, walletID, remaining); err != nil {
			return err
		}
		if err := insertWalletTxnTx(ctx, tx, tenantID, walletID, -remaining, "invoice_payment", invNum, "Bayar tagihan "+invNum); err != nil {
			return err
		}
		now := time.Now()
		if err := tx.QueryRow(ctx, `
			INSERT INTO payments (tenant_id, customer_id, invoice_id, amount, method, reference, status, paid_at)
			VALUES ($1,$2,$3,$4,$5,$6,'paid',$7)
			RETURNING id, created_at
		`, tenantID, customerID, invoiceID, remaining, PaymentMethodWallet, invNum, now).Scan(&payment.ID, &payment.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE invoices SET
				paid_amount = paid_amount + $3,
				status = CASE WHEN paid_amount + $3 >= total_amount THEN 'paid' ELSE 'partial' END,
				paid_at = CASE WHEN paid_amount + $3 >= total_amount THEN $4 ELSE paid_at END,
				updated_at = NOW()
			WHERE tenant_id=$1 AND id=$2
		`, tenantID, invoiceID, remaining, now); err != nil {
			return err
		}
		payment.TenantID = tenantID
		payment.CustomerID = customerID
		payment.InvoiceID = &invoiceID
		payment.Amount = remaining
		payment.Method = PaymentMethodWallet
		payment.Status = "paid"
		payment.Reference = &invNum
		payment.PaidAt = &now
		payment.InvoiceNumber = invNum

		liability := lookupAccountTx(ctx, tx, tenantID, "liability", "2100")
		revenue := lookupAccountTx(ctx, tx, tenantID, "revenue", "4100")
		if err := postWalletJournalTx(ctx, tx, tenantID, "Pembayaran "+invNum, invNum, "wallet_payment", &invoiceID, liability, revenue, remaining); err != nil {
			return err
		}
		paid = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if !paid {
		return nil, false, nil
	}
	return &payment, true, nil
}
