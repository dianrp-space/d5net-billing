package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// NormalBalance mengembalikan saldo akun pada sisi normalnya:
// aset & beban = debit - kredit; liabilitas, ekuitas & pendapatan = kredit - debit.
func NormalBalance(accountType string, debit, credit int64) int64 {
	switch strings.ToLower(strings.TrimSpace(accountType)) {
	case "asset", "expense":
		return debit - credit
	default:
		return credit - debit
	}
}

type JournalLineView struct {
	AccountID   xid.ID `json:"account_id"`
	AccountCode string `json:"account_code"`
	AccountName string `json:"account_name"`
	AccountType string `json:"account_type"`
	Debit       int64  `json:"debit"`
	Credit      int64  `json:"credit"`
}

type JournalEntryView struct {
	ID          xid.ID            `json:"id"`
	EntryDate   string            `json:"entry_date"`
	Reference   string            `json:"reference,omitempty"`
	Description string            `json:"description"`
	SourceType  string            `json:"source_type,omitempty"`
	TotalDebit  int64             `json:"total_debit"`
	TotalCredit int64             `json:"total_credit"`
	Lines       []JournalLineView `json:"lines"`
}

type TrialBalanceRow struct {
	AccountID xid.ID `json:"account_id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Debit     int64  `json:"debit"`
	Credit    int64  `json:"credit"`
	Balance   int64  `json:"balance"`
}

type TrialBalance struct {
	From        string            `json:"from"`
	To          string            `json:"to"`
	Rows        []TrialBalanceRow `json:"rows"`
	TotalDebit  int64             `json:"total_debit"`
	TotalCredit int64             `json:"total_credit"`
}

type LedgerLine struct {
	EntryID     xid.ID `json:"entry_id"`
	EntryDate   string `json:"entry_date"`
	Reference   string `json:"reference,omitempty"`
	Description string `json:"description"`
	Debit       int64  `json:"debit"`
	Credit      int64  `json:"credit"`
	Balance     int64  `json:"balance"`
}

type AccountLedger struct {
	AccountID   xid.ID       `json:"account_id"`
	AccountCode string       `json:"account_code"`
	AccountName string       `json:"account_name"`
	AccountType string       `json:"account_type"`
	From        string       `json:"from"`
	To          string       `json:"to"`
	Opening     int64        `json:"opening"`
	Lines       []LedgerLine `json:"lines"`
	TotalDebit  int64        `json:"total_debit"`
	TotalCredit int64        `json:"total_credit"`
	Closing     int64        `json:"closing"`
}

type PnLLine struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}

type PnLReport struct {
	From            string    `json:"from"`
	To              string    `json:"to"`
	Period          string    `json:"period"`
	Revenue         int64     `json:"revenue"`
	Expense         int64     `json:"expense"`
	Profit          int64     `json:"profit"`
	RevenueAccounts []PnLLine `json:"revenue_accounts"`
	ExpenseAccounts []PnLLine `json:"expense_accounts"`
}

type BalanceSheetLine struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}

type BalanceSheetReport struct {
	From                   string             `json:"from"`
	To                     string             `json:"to"`
	Assets                 []BalanceSheetLine `json:"assets"`
	Liabilities            []BalanceSheetLine `json:"liabilities"`
	Equity                 []BalanceSheetLine `json:"equity"`
	TotalAssets            int64              `json:"total_assets"`
	TotalLiabilities       int64              `json:"total_liabilities"`
	TotalEquity            int64              `json:"total_equity"`
	CurrentEarnings        int64              `json:"current_earnings"`
	TotalLiabilitiesEquity int64              `json:"total_liabilities_equity"`
	Balanced               bool               `json:"balanced"`
}

func dateOnly(t time.Time) string {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Format("2006-01-02")
}

func normalizeRange(from, to time.Time) (time.Time, time.Time) {
	now := time.Now()
	if from.IsZero() {
		from = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	}
	if to.IsZero() {
		to = now
	}
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location())
	if end.Before(start) {
		start, end = end, start
	}
	return start, end
}

// accountBalances mengagregasi debit/kredit per akun dalam rentang tanggal.
func (s *Store) accountBalances(ctx context.Context, tenantID xid.ID, from, to time.Time) ([]TrialBalanceRow, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	start, end := normalizeRange(from, to)
	rows, err := s.Pool.Query(ctx, `
		SELECT a.id, a.code, a.name, a.type, COALESCE(t.debit,0), COALESCE(t.credit,0)
		FROM chart_of_accounts a
		LEFT JOIN (
			SELECT l.account_id, SUM(l.debit) AS debit, SUM(l.credit) AS credit
			FROM journal_lines l
			JOIN journal_entries e ON e.id = l.entry_id
			WHERE l.tenant_id = $1 AND e.entry_date >= $2 AND e.entry_date <= $3
			GROUP BY l.account_id
		) t ON t.account_id = a.id
		WHERE a.tenant_id = $1
		ORDER BY a.code
	`, tenantID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrialBalanceRow{}
	for rows.Next() {
		var r TrialBalanceRow
		if err := rows.Scan(&r.AccountID, &r.Code, &r.Name, &r.Type, &r.Debit, &r.Credit); err != nil {
			return nil, err
		}
		r.Balance = NormalBalance(r.Type, r.Debit, r.Credit)
		out = append(out, r)
	}
	return out, rows.Err()
}

// accountOpenings mengembalikan total debit/kredit per akun sebelum tanggal
// tertentu (saldo awal periode).
func (s *Store) accountOpenings(ctx context.Context, tenantID xid.ID, before string) (map[xid.ID][2]int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT l.account_id, COALESCE(SUM(l.debit),0), COALESCE(SUM(l.credit),0)
		FROM journal_lines l
		JOIN journal_entries e ON e.id = l.entry_id
		WHERE l.tenant_id = $1 AND e.entry_date < $2
		GROUP BY l.account_id
	`, tenantID, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[xid.ID][2]int64{}
	for rows.Next() {
		var id xid.ID
		var d, c int64
		if err := rows.Scan(&id, &d, &c); err != nil {
			return nil, err
		}
		out[id] = [2]int64{d, c}
	}
	return out, rows.Err()
}

func (s *Store) TrialBalance(ctx context.Context, tenantID xid.ID, from, to time.Time) (*TrialBalance, error) {
	start, end := normalizeRange(from, to)
	rows, err := s.accountBalances(ctx, tenantID, start, end)
	if err != nil {
		return nil, err
	}
	openings, err := s.accountOpenings(ctx, tenantID, dateOnly(start))
	if err != nil {
		return nil, err
	}
	tb := &TrialBalance{From: dateOnly(start), To: dateOnly(end), Rows: []TrialBalanceRow{}}
	for i := range rows {
		op := openings[rows[i].AccountID]
		totalDebit := rows[i].Debit + op[0]
		totalCredit := rows[i].Credit + op[1]
		if totalDebit == 0 && totalCredit == 0 {
			continue
		}
		rows[i].Balance = NormalBalance(rows[i].Type, totalDebit, totalCredit)
		tb.TotalDebit += rows[i].Debit
		tb.TotalCredit += rows[i].Credit
		tb.Rows = append(tb.Rows, rows[i])
	}
	return tb, nil
}

func (s *Store) ProfitAndLoss(ctx context.Context, tenantID xid.ID, from, to time.Time) (*PnLReport, error) {
	start, end := normalizeRange(from, to)
	rows, err := s.accountBalances(ctx, tenantID, start, end)
	if err != nil {
		return nil, err
	}
	rep := &PnLReport{
		From:            start.Format("2006-01-02"),
		To:              end.Format("2006-01-02"),
		Period:          start.Format("2006-01"),
		RevenueAccounts: []PnLLine{},
		ExpenseAccounts: []PnLLine{},
	}
	for _, r := range rows {
		if r.Debit == 0 && r.Credit == 0 {
			continue
		}
		switch strings.ToLower(r.Type) {
		case "revenue", "income":
			rep.Revenue += r.Balance
			rep.RevenueAccounts = append(rep.RevenueAccounts, PnLLine{Code: r.Code, Name: r.Name, Amount: r.Balance})
		case "expense":
			rep.Expense += r.Balance
			rep.ExpenseAccounts = append(rep.ExpenseAccounts, PnLLine{Code: r.Code, Name: r.Name, Amount: r.Balance})
		}
	}
	rep.Profit = rep.Revenue - rep.Expense
	return rep, nil
}

func (s *Store) BalanceSheet(ctx context.Context, tenantID xid.ID, from, to time.Time) (*BalanceSheetReport, error) {
	// Neraca adalah saldo kumulatif s/d tanggal "to" (bukan pergerakan periode).
	_, end := normalizeRange(from, to)
	allFrom := time.Date(1800, 1, 1, 0, 0, 0, 0, end.Location())
	rows, err := s.accountBalances(ctx, tenantID, allFrom, end)
	if err != nil {
		return nil, err
	}
	rep := &BalanceSheetReport{
		To:          dateOnly(end),
		Assets:      []BalanceSheetLine{},
		Liabilities: []BalanceSheetLine{},
		Equity:      []BalanceSheetLine{},
	}
	for _, r := range rows {
		if r.Debit == 0 && r.Credit == 0 {
			continue
		}
		line := BalanceSheetLine{Code: r.Code, Name: r.Name, Amount: r.Balance}
		switch strings.ToLower(r.Type) {
		case "asset":
			rep.Assets = append(rep.Assets, line)
			rep.TotalAssets += r.Balance
		case "liability":
			rep.Liabilities = append(rep.Liabilities, line)
			rep.TotalLiabilities += r.Balance
		case "equity":
			rep.Equity = append(rep.Equity, line)
			rep.TotalEquity += r.Balance
		case "revenue", "income":
			rep.CurrentEarnings += r.Balance
		case "expense":
			rep.CurrentEarnings -= r.Balance
		}
	}
	rep.TotalLiabilitiesEquity = rep.TotalLiabilities + rep.TotalEquity + rep.CurrentEarnings
	rep.Balanced = rep.TotalAssets == rep.TotalLiabilitiesEquity
	return rep, nil
}

func (s *Store) GeneralJournal(ctx context.Context, tenantID xid.ID, from, to time.Time, accountID *xid.ID, search string, limit, offset int) ([]JournalEntryView, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	start, end := normalizeRange(from, to)
	if limit <= 0 {
		limit = 50
	}
	where := `WHERE e.tenant_id=$1 AND e.entry_date >= $2 AND e.entry_date <= $3`
	args := []any{tenantID, start.Format("2006-01-02"), end.Format("2006-01-02")}
	if accountID != nil && !xid.IsNil(*accountID) {
		args = append(args, *accountID)
		where += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM journal_lines l WHERE l.entry_id=e.id AND l.account_id=$%d)`, len(args))
	}
	if q := strings.TrimSpace(search); q != "" {
		args = append(args, "%"+q+"%")
		n := len(args)
		where += fmt.Sprintf(` AND (e.description ILIKE $%d OR COALESCE(e.reference,'') ILIKE $%d)`, n, n)
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM journal_entries e `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.Pool.Query(ctx, `
		SELECT e.id, e.entry_date, COALESCE(e.reference,''), e.description, COALESCE(e.source_type,'')
		FROM journal_entries e `+where+
		fmt.Sprintf(` ORDER BY e.entry_date DESC, e.created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	entries := []JournalEntryView{}
	ids := []xid.ID{}
	index := map[xid.ID]int{}
	for rows.Next() {
		var e JournalEntryView
		if err := rows.Scan(&e.ID, &e.EntryDate, &e.Reference, &e.Description, &e.SourceType); err != nil {
			rows.Close()
			return nil, 0, err
		}
		e.Lines = []JournalLineView{}
		index[e.ID] = len(entries)
		ids = append(ids, e.ID)
		entries = append(entries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(ids) == 0 {
		return entries, total, nil
	}
	lines, err := s.journalLinesForEntries(ctx, tenantID, ids)
	if err != nil {
		return nil, 0, err
	}
	for _, ln := range lines {
		i, ok := index[ln.entryID]
		if !ok {
			continue
		}
		entries[i].Lines = append(entries[i].Lines, ln.line)
		entries[i].TotalDebit += ln.line.Debit
		entries[i].TotalCredit += ln.line.Credit
	}
	return entries, total, nil
}

type journalLineWithEntry struct {
	entryID xid.ID
	line    JournalLineView
}

func (s *Store) journalLinesForEntries(ctx context.Context, tenantID xid.ID, ids []xid.ID) ([]journalLineWithEntry, error) {
	args := []any{tenantID}
	placeholders := make([]string, 0, len(ids))
	for i, id := range ids {
		args = append(args, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+2))
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT l.entry_id, a.id, a.code, a.name, a.type, l.debit, l.credit
		FROM journal_lines l
		JOIN chart_of_accounts a ON a.id = l.account_id
		WHERE l.tenant_id = $1 AND l.entry_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY l.debit DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []journalLineWithEntry{}
	for rows.Next() {
		var lw journalLineWithEntry
		if err := rows.Scan(&lw.entryID, &lw.line.AccountID, &lw.line.AccountCode, &lw.line.AccountName, &lw.line.AccountType, &lw.line.Debit, &lw.line.Credit); err != nil {
			return nil, err
		}
		out = append(out, lw)
	}
	return out, rows.Err()
}

func (s *Store) AccountLedger(ctx context.Context, tenantID xid.ID, accountID xid.ID, from, to time.Time) (*AccountLedger, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	start, end := normalizeRange(from, to)
	var acc AccountLedger
	if err := s.Pool.QueryRow(ctx, `
		SELECT id, code, name, type FROM chart_of_accounts WHERE tenant_id=$1 AND id=$2
	`, tenantID, accountID).Scan(&acc.AccountID, &acc.AccountCode, &acc.AccountName, &acc.AccountType); err != nil {
		return nil, err
	}
	var oDebit, oCredit int64
	_ = s.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(l.debit),0), COALESCE(SUM(l.credit),0)
		FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id
		WHERE l.tenant_id=$1 AND l.account_id=$2 AND e.entry_date < $3
	`, tenantID, accountID, start.Format("2006-01-02")).Scan(&oDebit, &oCredit)
	acc.From = dateOnly(start)
	acc.To = dateOnly(end)
	acc.Opening = NormalBalance(acc.AccountType, oDebit, oCredit)
	acc.Lines = []LedgerLine{}

	rows, err := s.Pool.Query(ctx, `
		SELECT e.id, e.entry_date, COALESCE(e.reference,''), e.description, l.debit, l.credit
		FROM journal_lines l JOIN journal_entries e ON e.id=l.entry_id
		WHERE l.tenant_id=$1 AND l.account_id=$2 AND e.entry_date >= $3 AND e.entry_date <= $4
		ORDER BY e.entry_date, e.created_at
	`, tenantID, accountID, dateOnly(start), dateOnly(end))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	balance := acc.Opening
	for rows.Next() {
		var ln LedgerLine
		if err := rows.Scan(&ln.EntryID, &ln.EntryDate, &ln.Reference, &ln.Description, &ln.Debit, &ln.Credit); err != nil {
			return nil, err
		}
		balance += NormalBalance(acc.AccountType, ln.Debit, ln.Credit)
		ln.Balance = balance
		acc.TotalDebit += ln.Debit
		acc.TotalCredit += ln.Credit
		acc.Lines = append(acc.Lines, ln)
	}
	acc.Closing = balance
	return &acc, rows.Err()
}
