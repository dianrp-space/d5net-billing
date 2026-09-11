import { useEffect, useState } from "react";
import { api, setClientSession, setToken } from "./api";
import { applyBrandingMeta } from "./branding";
import { LoginShell, SecretInput } from "./ui";
import { AuthThemeCorner } from "./ThemeToggle";

export type PortalCustomer = {
  id?: string;
  full_name: string;
  phone: string;
  customer_code: string;
  email?: string | null;
  address?: string | null;
  identity_type?: string | null;
  identity_number?: string | null;
  cluster_name?: string | null;
  service_status?: string;
  is_active?: boolean;
};

export type ClientPortalData = {
  customer?: PortalCustomer;
  customers?: PortalCustomer[];
  subscriptions?: {
    id?: string;
    username: string;
    plan_id?: string;
    plan_name: string;
    status: string;
    customer_id?: string;
    customer_name?: string;
    customer_code?: string;
    service_type?: string;
  }[];
  invoices?: {
    id: string;
    invoice_number: string;
    total_amount: number;
    paid_amount?: number;
    status: string;
    due_date?: string;
    customer_name?: string;
    customer_code?: string;
  }[];
  payments?: {
    amount: number;
    method: string;
    status: string;
    paid_at?: string;
    created_at?: string;
    customer_name?: string;
    customer_code?: string;
    invoice_number?: string;
  }[];
  wallet_balance?: number;
  tenant_slug?: string;
  tenant_name?: string;
  portal_token?: string;
};

type PublicBranding = {
  name?: string;
  app_name?: string;
  logo_url?: string | null;
  favicon_url?: string | null;
};

type LoginMode = "admin" | "client";

export function TenantLogin({
  mode,
  onAdminSuccess,
  onClientSuccess,
}: {
  mode: LoginMode;
  onAdminSuccess: () => void;
  onClientSuccess: (data: ClientPortalData) => void;
}) {
  const isAdmin = mode === "admin";
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [tenant, setTenant] = useState<PublicBranding | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const t = await api<PublicBranding>("/api/public/branding");
        if (!cancelled) setTenant(t);
      } catch {
        if (!cancelled) setErr("Provider tidak aktif.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!tenant) return;
    applyBrandingMeta({
      appName: tenant.name || tenant.app_name,
      faviconUrl: tenant.favicon_url,
      titleSuffix: isAdmin ? "Admin" : "Login Portal Pelanggan",
      separator: "-",
    });
  }, [tenant, isAdmin]);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      if (isAdmin) {
        const data = await api<{ access_token?: string; requires_totp?: boolean }>("/api/auth/login", {
          method: "POST",
          body: JSON.stringify({ email, password }),
        });
        if (data.requires_totp) {
          setErr("Akun ini memakai 2FA. Masukkan kode TOTP di versi berikutnya.");
          return;
        }
        if (data.access_token) {
          setToken(data.access_token);
          onAdminSuccess();
        } else {
          setErr("Login gagal");
        }
      } else {
        const data = await api<ClientPortalData>("/api/portal/login", {
          method: "POST",
          body: JSON.stringify({ phone, password }),
        });
        setClientSession(data);
        onClientSuccess(data);
      }
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : "Login gagal");
    } finally {
      setBusy(false);
    }
  }

  const brand = tenant?.name || tenant?.app_name || "ISP";

  return (
    <AuthThemeCorner>
      <LoginShell
        brand={brand}
        logoUrl={tenant?.logo_url}
        title={isAdmin ? "Masuk admin" : "Login Portal Pelanggan"}
        subtitle={isAdmin ? "Panel administrator" : "Cek paket & tagihan · password default = nomor HP"}
      >
        {isAdmin ? (
          <form className="flex flex-col gap-3" onSubmit={onSubmit}>
            <input className="input" placeholder="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            <SecretInput placeholder="Password" value={password} onChange={(e) => setPassword(e.target.value)} required autoComplete="current-password" />
            <button className="btn" disabled={busy || !tenant}>
              {busy ? "Masuk..." : "Masuk"}
            </button>
          </form>
        ) : (
          <form className="flex flex-col gap-3" onSubmit={onSubmit}>
            <input className="input" placeholder="Nomor telepon" value={phone} onChange={(e) => setPhone(e.target.value)} required />
            <SecretInput placeholder="Password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
            <button className="btn" disabled={busy || !tenant}>
              {busy ? "Masuk..." : "Masuk"}
            </button>
          </form>
        )}
        {err && <p className="mt-3 text-sm text-[var(--danger)]">{err}</p>}
      </LoginShell>
    </AuthThemeCorner>
  );
}
