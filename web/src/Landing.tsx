import { useEffect, useState } from "react";
import {
  ArrowRight,
  BellRing,
  CircleCheck,
  CreditCard,
  Landmark,
  Mail,
  MapPin,
  Menu,
  Phone,
  QrCode,
  ReceiptText,
  Store,
  UserRound,
  Wallet,
  X,
  Zap,
} from "lucide-react";
import { api } from "./api";
import { applyBrandingMeta, DEFAULT_BRAND_LOGO } from "./branding";
import { ChatwootWidget } from "./ChatwootWidget";
import { ThemeToggle } from "./ThemeToggle";
import { formatRp } from "./ui";
import { billingCycleLabel, planSpeedLabel } from "./PortalChangePlan";

type PublicPlan = {
  id: string;
  name: string;
  price: number;
  billing_cycle?: string;
  service_type?: string;
  download_mbps: number;
  upload_mbps: number;
  quota_gb?: number | null;
};

type PublicSite = {
  name?: string;
  app_name?: string;
  logo_url?: string | null;
  favicon_url?: string | null;
  about?: string;
  product_description?: string;
  support_email?: string;
  support_phone?: string;
  support_address?: string;
  plans?: PublicPlan[];
};

const PAY_METHODS = [
  { icon: <QrCode size={15} />, label: "QRIS" },
  { icon: <Landmark size={15} />, label: "VA Bank" },
  { icon: <Wallet size={15} />, label: "E-Wallet" },
  { icon: <Store size={15} />, label: "Retail" },
  { icon: <CreditCard size={15} />, label: "Kartu" },
];

const STEPS = [
  {
    icon: <UserRound size={18} />,
    title: "Login",
    desc: "Masuk pakai nomor HP dan password portal Anda.",
  },
  {
    icon: <ReceiptText size={18} />,
    title: "Lihat tagihan",
    desc: "Daftar tagihan berjalan, riwayat, dan status langganan.",
  },
  {
    icon: <CreditCard size={18} />,
    title: "Bayar online",
    desc: "Pilih metode favorit — lunas dalam hitungan detik.",
  },
];

export function Landing({ onGoLogin }: { onGoLogin: () => void }) {
  const [site, setSite] = useState<PublicSite | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      let loaded: PublicSite | null = null;
      try {
        loaded = await api<PublicSite>("/api/public/site");
      } catch {
        // Fallback: keep branding working even if the site endpoint is unavailable.
        try {
          const b = await api<PublicSite>("/api/public/branding");
          loaded = { name: b.name, app_name: b.app_name, logo_url: b.logo_url, favicon_url: b.favicon_url };
        } catch {
          /* keep defaults */
        }
      }
      if (cancelled || !loaded) return;
      setSite(loaded);
      applyBrandingMeta({
        appName: loaded.name || loaded.app_name,
        faviconUrl: loaded.favicon_url,
      });
      document.title = `${loaded.name || loaded.app_name || "Portal"} - Portal Pembayaran Internet`;
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const name = (site?.name || site?.app_name || "Delima Net").trim();
  const logo = site?.logo_url || DEFAULT_BRAND_LOGO;
  const plans = site?.plans ?? [];
  const about = (site?.about || "").trim();
  const productDescription = (site?.product_description || "").trim();
  const supportEmail = (site?.support_email || "").trim();
  const supportPhone = (site?.support_phone || "").trim();
  const supportAddress = (site?.support_address || "").trim();
  const hasSupport = Boolean(supportEmail || supportPhone || supportAddress);

  const navLinks = [
    about ? { href: "#tentang", label: "Tentang" } : null,
    plans.length > 0 ? { href: "#produk", label: "Produk & Harga" } : null,
    { href: "#cara-bayar", label: "Cara Bayar" },
    hasSupport ? { href: "#kontak", label: "Kontak" } : null,
  ].filter((l): l is { href: string; label: string } => l !== null);

  return (
    <div className="landing min-h-full">
      <ChatwootWidget />
      <div className="landing-bg" aria-hidden />
      <header className="sticky top-3 z-50 mx-auto mt-3 max-w-6xl rounded-2xl border border-[var(--border)] bg-[var(--panel)]/85 shadow-[var(--shadow-sm)] backdrop-blur-md">
        <div className="flex items-center justify-between gap-3 px-4 py-3 md:px-6">
          <a href="/" className="flex items-center gap-3 no-underline" title="Ke halaman utama" aria-label="Ke halaman utama">
            <img src={logo} alt={name} className="h-9 w-auto" />
            <span className="landing-brand text-xl">{name}</span>
          </a>
          <nav className="landing-nav" aria-label="Navigasi halaman">
            {navLinks.map((l) => (
              <a key={l.href} href={l.href}>
                {l.label}
              </a>
            ))}
          </nav>
          <div className="flex items-center gap-2">
            <ThemeToggle />
            <button type="button" className="btn landing-header-login px-5 py-2" onClick={onGoLogin}>
              Login Pelanggan
            </button>
            <button
              type="button"
              className="landing-nav-toggle"
              aria-label={menuOpen ? "Tutup menu" : "Buka menu"}
              aria-expanded={menuOpen}
              onClick={() => setMenuOpen((v) => !v)}
            >
              {menuOpen ? <X size={18} /> : <Menu size={18} />}
            </button>
          </div>
        </div>
        {menuOpen ? (
          <nav className="landing-nav-mobile" aria-label="Navigasi halaman">
            {navLinks.map((l) => (
              <a key={l.href} href={l.href} onClick={() => setMenuOpen(false)}>
                {l.label}
              </a>
            ))}
            <button
              type="button"
              className="landing-nav-mobile-login"
              onClick={() => {
                setMenuOpen(false);
                onGoLogin();
              }}
            >
              Login Pelanggan <ArrowRight size={16} />
            </button>
          </nav>
        ) : null}
      </header>

      <main className="relative z-10 mx-auto max-w-6xl px-6 pb-20 md:px-10">
        <div className="landing-hero">
          <div>
            <p className="landing-eyebrow">
              <Zap size={13} /> Portal tagihan internet
            </p>
            <h1 className="landing-title">
              Internet sat-set,
              <br />
              bayar <em>anti ribet.</em>
            </h1>
            <p className="landing-sub">
              Cek paket, lihat tagihan, dan bayar online kapan saja dari satu halaman —
              tanpa antre, tanpa ribet.
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              <button type="button" className="btn px-6 py-3 text-base" onClick={onGoLogin}>
                Login Pelanggan <ArrowRight size={17} />
              </button>
              <a className="btn-ghost inline-flex items-center px-6 py-3 text-base" href="#cara-bayar">
                Cara bayar
              </a>
            </div>
            <ul className="landing-trust">
              <li>
                <CircleCheck size={15} /> Lunas hitungan detik
              </li>
              <li>
                <CircleCheck size={15} /> Semua metode bayar
              </li>
              <li>
                <BellRing size={15} /> Notifikasi WhatsApp
              </li>
            </ul>
          </div>
          <div className="landing-visual" aria-hidden>
            <div className="landing-invoice">
              <div className="landing-invoice-sheen" aria-hidden />
              <div className="landing-invoice-head">
                <span className="landing-invoice-mark">
                  <ReceiptText size={18} />
                </span>
                <div>
                  <b>Invoice</b>
                  <span>INV-D5N-2026090001-092026</span>
                </div>
                <span className="landing-invoice-stamp">
                  <CircleCheck size={13} /> Lunas
                </span>
              </div>
              <div className="landing-invoice-meta">
                <div>
                  <span>Pelanggan</span>
                  <b>Budi Santoso</b>
                </div>
                <div>
                  <span>Jatuh tempo</span>
                  <b>10 Okt 2026</b>
                </div>
              </div>
              <div className="landing-invoice-items">
                <div>
                  <span>Paket Internet 30 Mbps</span>
                  <span>Rp 150.000</span>
                </div>
                <div>
                  <span>Biaya instalasi</span>
                  <span className="landing-invoice-free">
                    <s>Rp 50.000</s> Gratis
                  </span>
                </div>
              </div>
              <div className="landing-invoice-total">
                <span>Total</span>
                <b>Rp 150.000</b>
              </div>
              <div className="landing-invoice-bar" aria-hidden>
                <i />
              </div>
            </div>
            <div className="landing-mock">
              <div className="landing-mock-toast">
                <CircleCheck size={18} />
                <div>
                  Pembayaran diterima
                  <small>Tagihan sudah lunas — layanan aktif kembali.</small>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="landing-chips" aria-label="Metode pembayaran">
          {PAY_METHODS.map((m) => (
            <span key={m.label} className="landing-chip">
              {m.icon} {m.label}
            </span>
          ))}
        </div>

        <section id="cara-bayar" className="landing-steps">
          <h2 className="landing-section-title">Bayar dalam 3 langkah</h2>
          <div className="landing-steps-grid">
            {STEPS.map((s, i) => (
              <div key={s.title} className="landing-step">
                <span className="landing-step-no">{i + 1}</span>
                <span className="landing-step-icon">{s.icon}</span>
                <b>{s.title}</b>
                <p>{s.desc}</p>
              </div>
            ))}
          </div>
        </section>

        {about ? (
          <section id="tentang" className="landing-about">
            <h2 className="landing-section-title">Tentang {name}</h2>
            <p>{about}</p>
          </section>
        ) : null}

        <section id="produk" className="landing-plans">
          <h2 className="landing-section-title">Produk &amp; harga</h2>
          {productDescription ? <p className="landing-plans-sub">{productDescription}</p> : null}
          {plans.length > 0 ? (
            <div className="landing-plans-grid">
              {plans.map((p) => (
                <div key={p.id} className="landing-plan">
                  <span className="landing-plan-name">{p.name}</span>
                  <span className="landing-plan-meta">
                    {planSpeedLabel(p)}
                    {p.quota_gb ? ` · ${p.quota_gb} GB` : ""}
                  </span>
                  <div className="landing-plan-price">
                    {formatRp(p.price)} <small>{billingCycleLabel(p.billing_cycle)}</small>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="landing-plans-sub">
              Daftar paket belum tersedia. Hubungi kami untuk informasi produk dan harga.
            </p>
          )}
        </section>

        {hasSupport ? (
          <section id="kontak" className="landing-support">
            <h2 className="landing-section-title">Kontak dukungan</h2>
            <div className="landing-support-grid">
              {supportEmail ? (
                <div className="landing-support-item">
                  <span className="landing-support-icon">
                    <Mail size={17} />
                  </span>
                  <div>
                    <b>Email</b>
                    <a href={`mailto:${supportEmail}`}>{supportEmail}</a>
                  </div>
                </div>
              ) : null}
              {supportPhone ? (
                <div className="landing-support-item">
                  <span className="landing-support-icon">
                    <Phone size={17} />
                  </span>
                  <div>
                    <b>Telepon</b>
                    <a href={`tel:${supportPhone.replace(/[^+\d]/g, "")}`}>{supportPhone}</a>
                  </div>
                </div>
              ) : null}
              {supportAddress ? (
                <div className="landing-support-item">
                  <span className="landing-support-icon">
                    <MapPin size={17} />
                  </span>
                  <div>
                    <b>Alamat usaha</b>
                    <span>{supportAddress}</span>
                  </div>
                </div>
              ) : null}
            </div>
          </section>
        ) : null}

        <section className="landing-contact">
          <div>
            <h2>Belum punya akses portal?</h2>
            <p>
              Hubungi admin <code className="landing-code">{name}</code> untuk aktivasi akun
              dan terima password pertama Anda.
            </p>
          </div>
          <button type="button" className="btn px-6 py-3" onClick={onGoLogin}>
            Login Pelanggan <ArrowRight size={17} />
          </button>
        </section>

        <footer className="landing-footer">
          <span>
            © {new Date().getFullYear()} {name} — better connecting all.
          </span>
          <nav className="landing-footer-links">
            <a href="/terms">Syarat &amp; Ketentuan</a>
            <a href="/privacy">Kebijakan Privasi</a>
          </nav>
        </footer>
      </main>
    </div>
  );
}
