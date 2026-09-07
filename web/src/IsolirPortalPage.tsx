import { useEffect, useState } from "react";
import { api } from "./api";
import { applyBrandingMeta } from "./branding";
import { formatRp, LoginShell, SecretInput } from "./ui";
import { AuthThemeCorner } from "./ThemeToggle";
import { QrisPayDialog, type QrisIntent } from "./QrisPayDialog";

type PublicTenant = {
  slug: string;
  name: string;
  app_name?: string;
  logo_url?: string | null;
  favicon_url?: string | null;
};

type Invoice = {
  id: string;
  invoice_number: string;
  total_amount: number;
  paid_amount: number;
  status: string;
  due_date?: string;
};

type IsolirSession = {
  customer: { full_name: string; phone: string; customer_code: string };
  invoices: Invoice[];
  tenant_slug: string;
  tenant_name: string;
  portal_token?: string;
};

function unpaid(inv: Invoice) {
  return inv.total_amount > (inv.paid_amount || 0) && inv.status !== "paid";
}

/** Public isolir portal: login singkat → daftar tagihan unpaid. */
export function IsolirPortalPage({ slug }: { slug: string }) {
  const [tenant, setTenant] = useState<PublicTenant | null>(null);
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [session, setSession] = useState<IsolirSession | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [qrisOpen, setQrisOpen] = useState(false);
  const [qrisInv, setQrisInv] = useState<Invoice | null>(null);
  const [qris, setQris] = useState<QrisIntent | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const t = await api<PublicTenant>(`/api/public/tenants/${encodeURIComponent(slug)}`);
        if (!cancelled) {
          setTenant(t);
          applyBrandingMeta({
            appName: t.name || t.app_name,
            faviconUrl: t.favicon_url,
            titleSuffix: "Isolir",
          });
        }
      } catch {
        if (!cancelled) setErr("Tenant tidak ditemukan.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [slug]);

  async function onLogin(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      const data = await api<{
        customer: IsolirSession["customer"];
        invoices: Invoice[];
        tenant_slug: string;
        tenant_name: string;
        portal_token?: string;
      }>("/api/portal/login", {
        method: "POST",
        body: JSON.stringify({ phone, password, tenant_slug: slug }),
      });
      setSession({
        customer: data.customer,
        invoices: (data.invoices || []).filter(unpaid),
        tenant_slug: data.tenant_slug || slug,
        tenant_name: data.tenant_name || tenant?.name || "",
        portal_token: data.portal_token,
      });
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : "Login gagal");
    } finally {
      setBusy(false);
    }
  }

  async function payQris(inv: Invoice) {
    if (!session?.portal_token) {
      setErr("Sesi portal tidak lengkap. Login ulang.");
      return;
    }
    setBusy(true);
    setErr("");
    try {
      const intent = await api<QrisIntent>(`/api/portal/invoices/${inv.id}/checkout`, {
        method: "POST",
        headers: { Authorization: `Bearer ${session.portal_token}` },
        body: JSON.stringify({}),
      });
      setQris(intent);
      setQrisInv(inv);
      setQrisOpen(true);
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : "Gagal membuat QRIS");
    } finally {
      setBusy(false);
    }
  }

  const appName = tenant?.name || tenant?.app_name || "Isolir";

  if (session) {
    return (
      <AuthThemeCorner>
        <div className="mx-auto flex min-h-screen max-w-lg flex-col justify-center gap-4 p-4">
          <div className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel)] p-6 shadow-sm">
            <p className="text-xs font-semibold uppercase tracking-wide text-[var(--stone)]">{appName}</p>
            <h1 className="mt-1 text-xl font-bold text-[var(--text)]">Tagihan belum lunas</h1>
            <p className="mt-1 text-sm text-[var(--muted)]">
              Halo {session.customer.full_name}. Bayar tagihan di bawah agar layanan dipulihkan.
            </p>
            <ul className="mt-4 space-y-2">
              {session.invoices.length === 0 ? (
                <li className="rounded-md bg-[var(--panel-muted)] px-3 py-3 text-sm text-[var(--muted)]">
                  Tidak ada tagihan terbuka. Hubungi admin jika internet masih terisolir.
                </li>
              ) : (
                session.invoices.map((inv) => (
                  <li
                    key={inv.id}
                    className="flex items-center justify-between gap-3 rounded-md border border-[var(--border)] bg-[var(--panel-muted)]/50 px-3 py-2.5"
                  >
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold">{inv.invoice_number}</p>
                      <p className="text-xs text-[var(--muted)]">
                        Jatuh tempo {inv.due_date ? new Date(inv.due_date).toLocaleDateString("id-ID") : "—"}
                      </p>
                    </div>
                    <div className="shrink-0 text-right">
                      <p className="text-sm font-bold">{formatRp(inv.total_amount - (inv.paid_amount || 0))}</p>
                      <button
                        type="button"
                        className="text-xs font-semibold text-[var(--accent)] underline"
                        disabled={busy}
                        onClick={() => void payQris(inv)}
                      >
                        Bayar QRIS
                      </button>
                    </div>
                  </li>
                ))
              )}
            </ul>
            <button
              type="button"
              className="mt-4 text-sm text-[var(--muted)] underline"
              onClick={() => setSession(null)}
            >
              Keluar
            </button>
            {err ? <p className="mt-2 text-sm text-[var(--danger)]">{err}</p> : null}
          </div>
        </div>
        <QrisPayDialog
          open={qrisOpen}
          invoiceNumber={qrisInv?.invoice_number || ""}
          intent={qris}
          pollPath={qrisInv ? `/api/portal/invoices/${qrisInv.id}/payment-intent` : undefined}
          cancelPath={qrisInv ? `/api/portal/invoices/${qrisInv.id}/payment-intent/cancel` : undefined}
          pollHeaders={session.portal_token ? { Authorization: `Bearer ${session.portal_token}` } : undefined}
          onClose={() => setQrisOpen(false)}
          onPaid={() => {
            if (!qrisInv) return;
            setSession({
              ...session,
              invoices: session.invoices.filter((i) => i.id !== qrisInv.id),
            });
          }}
        />
      </AuthThemeCorner>
    );
  }

  return (
    <AuthThemeCorner>
      <LoginShell
        title={appName}
        subtitle="Layanan diisolir — login untuk melihat & bayar tagihan"
        logoUrl={tenant?.logo_url}
      >
        <form className="grid gap-3" onSubmit={onLogin}>
          <label className="grid gap-1 text-sm">
            <span>No. HP / WhatsApp</span>
            <input className="input" value={phone} onChange={(e) => setPhone(e.target.value)} required autoComplete="username" />
          </label>
          <label className="grid gap-1 text-sm">
            <span>Password portal</span>
            <SecretInput value={password} onChange={(e) => setPassword(e.target.value)} required autoComplete="current-password" />
          </label>
          {err ? <p className="text-sm text-[var(--danger)]">{err}</p> : null}
          <button type="submit" className="btn" disabled={busy || !tenant}>
            {busy ? "Masuk…" : "Masuk"}
          </button>
        </form>
      </LoginShell>
    </AuthThemeCorner>
  );
}
