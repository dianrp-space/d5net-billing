import { useEffect, useState } from "react";
import {
  ArrowRight,
  BellRing,
  CircleCheck,
  CreditCard,
  Landmark,
  QrCode,
  ReceiptText,
  Store,
  UserRound,
  Wallet,
  Zap,
} from "lucide-react";
import { api } from "./api";
import { applyBrandingMeta, DEFAULT_BRAND_LOGO } from "./branding";
import { ChatwootWidget } from "./ChatwootWidget";
import { ThemeToggle } from "./ThemeToggle";

type PublicBranding = {
  name?: string;
  app_name?: string;
  logo_url?: string | null;
  favicon_url?: string | null;
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
  const [branding, setBranding] = useState<PublicBranding | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const b = await api<PublicBranding>("/api/public/branding");
        if (!cancelled) return;
        setBranding(b);
        applyBrandingMeta({
          appName: b.name || b.app_name,
          faviconUrl: b.favicon_url,
        });
        document.title = "Delima Net - Portal Pembayaran Internet";
      } catch {
        /* keep defaults */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const name = (branding?.name || branding?.app_name || "Delima Net").trim();
  const logo = branding?.logo_url || DEFAULT_BRAND_LOGO;

  return (
    <div className="landing min-h-full">
      <ChatwootWidget />
      <div className="landing-bg" aria-hidden />
      <header className="relative z-10 mx-auto flex max-w-6xl items-center justify-between px-6 py-5 md:px-10">
        <span className="flex items-center gap-3">
          <img src={logo} alt={name} className="h-9 w-auto" />
          <span className="landing-brand text-xl">{name}</span>
        </span>
        <div className="flex items-center gap-2">
          <ThemeToggle />
          <button type="button" className="btn px-5 py-2" onClick={onGoLogin}>
            Login Pelanggan
          </button>
        </div>
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
        </footer>
      </main>
    </div>
  );
}
