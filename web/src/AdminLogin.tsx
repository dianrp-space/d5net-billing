import { useEffect, useState } from "react";
import { api, setAdminSlug, setToken } from "./api";
import { applyBrandingMeta } from "./branding";
import { LoginShell, SecretInput } from "./ui";
import { AuthThemeCorner } from "./ThemeToggle";

type PublicTenant = {
  slug: string;
  name: string;
  is_active: boolean;
  app_name?: string;
  logo_url?: string | null;
  favicon_url?: string | null;
};

export function AdminLogin({
  slug,
  onSuccess,
}: {
  slug: string;
  onSuccess: () => void;
}) {
  const [email, setEmail] = useState("");
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
            appName: t.name || t.app_name,
            faviconUrl: t.favicon_url,
            titleSuffix: "Admin",
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
      const data = await api<{ access_token?: string; requires_totp?: boolean }>("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, password, tenant_slug: slug }),
      });
      if (data.requires_totp) {
        setErr("Akun ini memakai 2FA. Masukkan kode TOTP di versi berikutnya.");
        return;
      }
      if (data.access_token) {
        setToken(data.access_token);
        setAdminSlug(slug);
        onSuccess();
      } else {
        setErr("Login gagal");
      }
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : "Login gagal");
    } finally {
      setBusy(false);
    }
  }

  const brand = tenant?.name || tenant?.app_name || slug;

  return (
    <AuthThemeCorner>
      <LoginShell
        brand={brand}
        logoUrl={tenant?.logo_url}
        title="Masuk admin"
        subtitle={`Panel tenant /admin/${slug}`}
      >
        <form className="flex flex-col gap-3" onSubmit={onSubmit}>
          <input className="input" placeholder="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          <SecretInput placeholder="Password" value={password} onChange={(e) => setPassword(e.target.value)} required autoComplete="current-password" />
          <button className="btn" disabled={busy || !tenant}>
            {busy ? "Masuk..." : "Masuk"}
          </button>
        </form>
        {err && <p className="mt-3 text-sm text-[var(--danger)]">{err}</p>}
      </LoginShell>
    </AuthThemeCorner>
  );
}
