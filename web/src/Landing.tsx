import { ThemeToggle } from "./ThemeToggle";

export function Landing({ onGoLogin }: { onGoLogin: () => void }) {
  return (
    <div className="landing min-h-full">
      <div className="landing-bg" aria-hidden />
      <header className="relative z-10 flex items-center justify-between px-6 py-5 md:px-10">
        <span className="landing-brand">drp-billing</span>
        <div className="flex items-center gap-2">
          <ThemeToggle />
          <button type="button" className="btn-ghost text-sm" onClick={onGoLogin}>
            Superadmin
          </button>
        </div>
      </header>
      <main className="relative z-10 mx-auto flex max-w-3xl flex-col px-6 pb-20 pt-16 md:px-10 md:pt-24">
        <h1 className="landing-brand text-5xl leading-none tracking-tight md:text-7xl">drp-billing</h1>
        <p className="mt-6 max-w-xl text-lg text-[#b8c4d4] md:text-xl">
          Billing ISP untuk jaringan MikroTik — pelanggan, tagihan, isolir, dan portal dalam satu platform multi-tenant.
        </p>
        <div className="mt-10 flex flex-wrap gap-3">
          <button type="button" className="btn px-5 py-2.5" onClick={onGoLogin}>
            Kelola tenant
          </button>
          <a className="btn-ghost inline-flex items-center px-5 py-2.5" href="#cara-akses">
            Cara akses
          </a>
        </div>
        <section id="cara-akses" className="mt-24 max-w-xl">
          <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-[#6b7c90]">Akses per tenant</h2>
          <ul className="mt-4 space-y-3 text-[#c5d0dc]">
            <li>
              Admin: <code className="landing-code">/&lt;slug&gt;/login</code>
            </li>
            <li>
              Pelanggan: <code className="landing-code">/&lt;slug&gt;/client/login</code>
            </li>
            <li>
              Superadmin: <code className="landing-code">/login</code>
            </li>
          </ul>
        </section>
      </main>
    </div>
  );
}
