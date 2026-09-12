import { useEffect, useState } from "react";
import {
  ArrowLeft,
  BellRing,
  CircleAlert,
  CircleCheck,
  Gauge,
  Headset,
  Loader2,
  LockKeyhole,
  Mail,
  Phone,
  ShieldCheck,
  UserRound,
  Wifi,
  Zap,
} from "lucide-react";
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
  photo_url?: string | null;
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
    paid_at?: string | null;
    issued_at?: string | null;
    customer_name?: string;
    customer_code?: string;
    items_summary?: string;
    items?: { description: string; quantity?: number; unit_price?: number; amount?: number }[];
  }[];
  payments?: {
    id?: string;
    invoice_id?: string;
    amount: number;
    method: string;
    status: string;
    paid_at?: string;
    created_at?: string;
    customer_name?: string;
    customer_code?: string;
    invoice_number?: string;
    items_summary?: string;
    items?: { description: string; quantity?: number; unit_price?: number; amount?: number }[];
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

function LoginVisual({ mode, brand }: { mode: LoginMode; brand: string }) {
  const isAdmin = mode === "admin";
  const feats = isAdmin
    ? [
        { icon: <Gauge size={17} />, title: "Satu panel operasional", desc: "Pelanggan, tagihan, isolir & tiket dalam satu dasbor." },
        { icon: <Wifi size={17} />, title: "Monitoring jaringan", desc: "Router MikroTik, sesi, dan MAP FTTH real-time." },
        { icon: <Headset size={17} />, title: "Respon gangguan cepat", desc: "Tiket & work order teknisi tercatat rapi." },
      ]
    : [
        { icon: <Zap size={17} />, title: "Bayar online kapan saja", desc: "VA bank, e-wallet, retail, sampai QRIS." },
        { icon: <BellRing size={17} />, title: "Pengingat otomatis", desc: "Info tagihan & isolir via WhatsApp." },
        { icon: <UserRound size={17} />, title: "Portal mandiri", desc: "Cek paket, tagihan & riwayat bayar sendiri." },
      ];
  return (
    <>
      <span className="auth-visual-brand">
        <img src="/d5net.webp" alt={brand} />
      </span>
      <h2 className="auth-visual-title">
        {isAdmin ? (
          <>Satu panel untuk <em>seluruh jaringan.</em></>
        ) : (
          <>Internet lancar, <em>bayar sat-set.</em></>
        )}
      </h2>
      <p className="auth-visual-sub">
        {isAdmin
          ? `Kelola pelanggan, tagihan, dan perangkat ${brand} dari mana saja.`
          : `Portal pembayaran tagihan ${brand} — cepat, aman, tanpa antre.`}
      </p>
      <div className="auth-visual-feats">
        {feats.map((f) => (
          <div key={f.title} className="auth-visual-feat">
            <span className="auth-visual-feat-icon">{f.icon}</span>
            <div>
              <b>{f.title}</b>
              <span>{f.desc}</span>
            </div>
          </div>
        ))}
      </div>
      <div className="auth-mock">
        <div className="auth-mock-row">
          <span className="auth-mock-no">INV-D5N-2026090001-092026</span>
          <span className="auth-mock-paid">Lunas</span>
        </div>
        <div className="auth-mock-amount">Rp 150.000</div>
        <div className="auth-mock-bar" aria-hidden>
          <i />
        </div>
      </div>
      <div className="auth-mock">
        <div className="auth-mock-toast">
          <CircleCheck size={18} />
          <div>
            Pembayaran diterima
            <small>Tagihan sudah lunas — layanan aktif kembali.</small>
          </div>
        </div>
      </div>
    </>
  );
}

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
      titleSuffix: isAdmin ? "Login Portal Admin" : "Login Portal Pelanggan",
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
        visual={<LoginVisual mode={mode} brand={brand} />}
        badge={
          isAdmin ? (
            <>
              <ShieldCheck size={12} /> Administrator
            </>
          ) : (
            <>
              <UserRound size={12} /> Pelanggan
            </>
          )
        }
        footer={
          isAdmin ? (
            <>Area khusus administrator. Akses tercatat.</>
          ) : (
            <a href="/">
              <ArrowLeft size={14} /> Kembali ke beranda
            </a>
          )
        }
      >
        {isAdmin ? (
          <form className="auth-form" onSubmit={onSubmit}>
            <label className="auth-label">
              Email
              <span className="auth-field">
                <Mail size={16} className="auth-field-icon" />
                <input
                  className="input"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                  autoComplete="username"
                />
              </span>
            </label>
            <label className="auth-label">
              Password
              <span className="auth-field">
                <LockKeyhole size={16} className="auth-field-icon" />
                <SecretInput value={password} onChange={(e) => setPassword(e.target.value)} required autoComplete="current-password" />
              </span>
            </label>
            <button className="btn auth-submit" disabled={busy || !tenant}>
              {busy ? (
                <>
                  <Loader2 size={16} className="animate-spin" /> Masuk...
                </>
              ) : (
                "Masuk"
              )}
            </button>
          </form>
        ) : (
          <form className="auth-form" onSubmit={onSubmit}>
            <label className="auth-label">
              Nomor telepon
              <span className="auth-field">
                <Phone size={16} className="auth-field-icon" />
                <input
                  className="input"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  required
                  autoComplete="username"
                  inputMode="tel"
                />
              </span>
            </label>
            <label className="auth-label">
              Password
              <span className="auth-field">
                <LockKeyhole size={16} className="auth-field-icon" />
                <SecretInput value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
              </span>
            </label>
            <button className="btn auth-submit" disabled={busy || !tenant}>
              {busy ? (
                <>
                  <Loader2 size={16} className="animate-spin" /> Masuk...
                </>
              ) : (
                "Masuk"
              )}
            </button>
          </form>
        )}
        {err && (
          <p className="auth-error" role="alert">
            <CircleAlert size={16} /> <span>{err}</span>
          </p>
        )}
      </LoginShell>
    </AuthThemeCorner>
  );
}
