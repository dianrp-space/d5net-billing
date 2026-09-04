import { useEffect, useState } from "react";
import { api, setClientSession } from "./api";
import { applyBrandingMeta } from "./branding";
import { LoginShell, SecretInput } from "./ui";
import { AuthThemeCorner } from "./ThemeToggle";

export type ClientPortalData = {
  customer?: { full_name: string; phone: string; customer_code: string };
  subscriptions?: { username: string; plan_name: string; status: string }[];
  invoices?: { invoice_number: string; total_amount: number; status: string; due_date?: string }[];
  payments?: { amount: number; method: string; status: string; paid_at?: string; created_at?: string }[];
  wallet_balance?: number;
  tenant_slug?: string;
  tenant_name?: string;
};

type PublicTenant = {
  slug: string;
  name: string;
  is_active: boolean;
  app_name?: string;
  logo_url?: string | null;
  favicon_url?: string | null;
};

export function ClientLogin({
  slug,
  onSuccess,
}: {
  slug: string;
  onSuccess: (data: ClientPortalData) => void;
}) {
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [tenant, setTenant] = useState<PublicTenant | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const t = await api<PublicTenant>(`/api/public/tenants/${encodeURIComponent(slug)}`);
        if (!cancelled) {
          setTenant(t);
          applyBrandingMeta({
            appName: t.app_name || t.name,
            faviconUrl: t.favicon_url,
            titleSuffix: "Portal",
          });
        }
      } catch {
        if (!cancelled) setErr("Tenant tidak ditemukan atau nonaktif.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [slug]);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      const data = await api<ClientPortalData>("/api/portal/login", {
        method: "POST",
        body: JSON.stringify({ phone, password, tenant_slug: slug }),
      });
      setClientSession(data);
      onSuccess(data);
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : "Login gagal");
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthThemeCorner>
      <LoginShell
        brand={tenant?.app_name || tenant?.name || slug}
        logoUrl={tenant?.logo_url}
        title="Portal pelanggan"
        subtitle={`Cek paket & tagihan · password default = nomor HP`}
      >
        <form className="flex flex-col gap-3" onSubmit={onSubmit}>
          <input className="input" placeholder="Nomor telepon" value={phone} onChange={(e) => setPhone(e.target.value)} required />
          <SecretInput placeholder="Password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
          <button className="btn" disabled={busy || !tenant}>
            {busy ? "Masuk..." : "Masuk"}
          </button>
        </form>
        {err && <p className="mt-3 text-sm text-[var(--danger)]">{err}</p>}
      </LoginShell>
    </AuthThemeCorner>
  );
}
