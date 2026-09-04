import { useEffect, useState } from "react";
import { api, setPlatformToken } from "./api";
import { applyBrandingMeta } from "./branding";
import { LoginShell, SecretInput } from "./ui";
import { AuthThemeCorner } from "./ThemeToggle";

type PublicBranding = {
  app_name: string;
  logo_url?: string | null;
  favicon_url?: string | null;
};

export function PlatformLogin({ onSuccess }: { onSuccess: () => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [branding, setBranding] = useState<PublicBranding | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const b = await api<PublicBranding>("/api/public/branding");
        if (!cancelled) {
          setBranding(b);
          applyBrandingMeta({
            appName: b.app_name || "drp-billing",
            faviconUrl: b.favicon_url,
            titleSuffix: "Platform",
          });
        }
      } catch {
        /* keep defaults */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      const data = await api<{ access_token?: string; requires_totp?: boolean }>("/api/auth/platform/login", {
        method: "POST",
        body: JSON.stringify({ email, password }),
      });
      if (data.requires_totp) {
        setErr("Akun ini memakai 2FA. Masukkan kode TOTP di versi berikutnya.");
        return;
      }
      if (data.access_token) {
        setPlatformToken(data.access_token);
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

  return (
    <AuthThemeCorner>
      <LoginShell
        brand={branding?.app_name || "drp-billing"}
        logoUrl={branding?.logo_url}
        title="Superadmin"
        subtitle="Kelola tenant di platform."
      >
        <form className="flex flex-col gap-3" onSubmit={onSubmit}>
          <input className="input" placeholder="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          <SecretInput placeholder="Password" value={password} onChange={(e) => setPassword(e.target.value)} required autoComplete="current-password" />
          <button className="btn" disabled={busy}>
            {busy ? "Masuk..." : "Masuk"}
          </button>
        </form>
        {err && <p className="mt-3 text-sm text-[var(--danger)]">{err}</p>}
      </LoginShell>
    </AuthThemeCorner>
  );
}
