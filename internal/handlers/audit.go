package handlers

import (
	"context"

	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/tenant"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// auditEvent mencatat aksi penting ke audit_logs tanpa menggagalkan request.
// Aman dipanggil dengan Deps parsial (test): diam bila Audit belum di-wire.
func auditEvent(ctx context.Context, d *Deps, action, entityType string, entityID *xid.ID, meta map[string]any) {
	if d == nil || d.Audit == nil {
		return
	}
	var tid, uid *xid.ID
	if t, ok := tenant.FromContext(ctx); ok {
		tid = &t.ID
		if !xid.IsNil(t.UserID) {
			uid = &t.UserID
		}
	}
	_ = d.Audit.Log(ctx, tid, uid, action, entityType, entityID, meta, httpx.IPFromContext(ctx))
}

// auditAction constants — dipakai UI untuk label.
const (
	AuditAuthLogin        = "auth.login"
	AuditAuthLoginFailed  = "auth.login_failed"
	AuditPortalLogin      = "portal.login"
	AuditInvoicePay       = "invoice.pay"
	AuditInvoiceIssue     = "invoice.issue_manual"
	AuditInvoiceDelete    = "invoice.delete"
	AuditInvoiceRestore   = "invoice.restore"
	AuditInvoicePurge     = "invoice.purge"
	AuditCustomerStatus   = "customer.batch_status"
	AuditCustomerDelete   = "customer.delete"
	AuditCustomerDismantle = "customer.dismantle"
	AuditBroadcast        = "notification.broadcast"
	AuditRoleCreate       = "role.create"
	AuditRoleUpdate       = "role.update"
	AuditRoleDelete       = "role.delete"
	AuditUserCreate       = "user.create"
	AuditUserDelete       = "user.delete"
)
