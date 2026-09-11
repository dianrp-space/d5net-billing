import { useEffect, useState } from "react";
import { api } from "./api";
import { applyBrandingMeta, DEFAULT_BRAND_LOGO } from "./branding";
import { ThemeToggle } from "./ThemeToggle";

type PublicBranding = {
  name?: string;
  app_name?: string;
  logo_url?: string | null;
  favicon_url?: string | null;
};

export function Landing({ onGoLogin }: { onGoLogin: () => void }) {
  const [branding, setBranding] = useState<PublicBranding | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const b = await api<PublicBranding>("/api/public/branding");
        if (cancelled) return;
        setBranding(b);
        applyBrandingMeta({ appName: b.name || b.app_name, faviconUrl: b.favicon_url });
      } catch {
        /* keep defaults */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const name = (branding?.name || branding?.app_name || "ISP").trim();
  const logo = branding?.logo_url || DEFAULT_BRAND_LOGO;

  return (
    <div className="landing min-h-full">
      <div className="landing-bg" aria-hidden />
      <header className="relative z-10 flex items-center justify-between px-6 py-5 md:px-10">
        <span className="flex items-center gap-3">
          <img src={logo} alt={name} className="h-9 w-auto" />
          <span className="landing-brand">{name}</span>
        </span>
        <ThemeToggle />
      </header>
      <main className="relative z-10 mx-auto flex max-w-3xl flex-col px-6 pb-20 pt-16 md:px-10 md:pt-24">
        <h1 className="landing-brand text-5xl leading-none tracking-tight md:text-7xl">{name}</h1>
        <p className="mt-6 max-w-xl text-lg text-[#b8c4d4] md:text-xl">
          Portal pembayaran tagihan internet. Cek paket, lihat tagihan, dan bayar online kapan saja
          dari satu halaman.
        </p>
        <div className="mt-10 flex flex-wrap gap-3">
          <button type="button" className="btn px-5 py-2.5" onClick={onGoLogin}>
            Login Pelanggan
          </button>
          <a className="btn-ghost inline-flex items-center px-5 py-2.5" href="#cara-bayar">
            Cara bayar
          </a>
        </div>
        <section id="cara-bayar" className="mt-24 max-w-xl">
          <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-[#6b7c90]">
            Bayar tagihan
          </h2>
          <ul className="mt-4 space-y-3 text-[#c5d0dc]">
            <li>1. Login pakai nomor HP dan password portal Anda.</li>
            <li>2. Lihat daftar tagihan yang belum lunas.</li>
            <li>3. Bayar via payment gateway yang tersedia.</li>
            <li>
              Belum punya akses? Hubungi admin{" "}
              <code className="landing-code">{name}</code>.
            </li>
          </ul>
        </section>
      </main>
    </div>
  );
}
