import { useState } from "react";
import { DEFAULT_BRAND_LOGO } from "./branding";
import { IconEye, IconEyeOff } from "./icons";
import type { PortalCustomer } from "./TenantLogin";

function identityLabel(type?: string | null): string {
  switch (String(type || "").trim().toLowerCase()) {
    case "ktp":
      return "NIK";
    case "sim":
      return "No. SIM";
    case "passport":
      return "No. Paspor";
    default:
      return "No. Identitas";
  }
}

function group4(v: string): string {
  return v.replace(/(.{4})(?=.)/g, "$1 ");
}

function maskIdentity(v: string): string {
  const raw = v.replace(/\s+/g, "");
  if (!raw) return "—";
  if (raw.length <= 4) return "••••";
  const last4 = raw.slice(-4);
  const maskedLen = raw.length - 4;
  const masked = "•".repeat(maskedLen) + last4;
  return group4(masked);
}

function statusMeta(status?: string): { label: string; tone: string } {
  switch (String(status || "").trim().toLowerCase()) {
    case "active":
      return { label: "Aktif", tone: "var(--ok, #2f7d4f)" };
    case "isolir":
      return { label: "Isolir", tone: "var(--danger)" };
    case "overdue":
      return { label: "Menunggak", tone: "var(--warn, #b7791f)" };
    case "inactive":
      return { label: "Nonaktif", tone: "var(--muted)" };
    case "dismantled":
      return { label: "Cabut", tone: "var(--muted)" };
    default:
      return { label: status || "—", tone: "var(--muted)" };
  }
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (!parts.length) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--muted)]">{label}</p>
      <p className="mt-0.5 truncate text-sm font-medium text-[var(--text)]">{value}</p>
    </div>
  );
}

/** Kartu identitas pelanggan ala mockup ID card, dengan NIK tersembunyi + toggle. */
export function ClientIdCard({
  customer,
  accounts,
  providerName,
  logoUrl,
}: {
  customer: PortalCustomer | null;
  accounts: PortalCustomer[];
  providerName: string;
  logoUrl?: string | null;
}) {
  const [showId, setShowId] = useState(false);
  const [activeId, setActiveId] = useState<string | null>(null);

  const current =
    (activeId ? accounts.find((a) => (a.id || a.customer_code) === activeId) : null) ||
    customer ||
    accounts[0] ||
    null;

  if (!current) return null;

  const st = statusMeta(current.service_status);
  const idNumber = (current.identity_number || "").trim();
  const idValue = showId ? group4(idNumber.replace(/\s+/g, "") || "—") : maskIdentity(idNumber);

  return (
    <section aria-label="Data diri pelanggan" className="overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--panel)] shadow-sm">
      {/* header pita */}
      <div
        className="flex items-center gap-3 px-4 py-3 sm:px-5"
        style={{
          background: "linear-gradient(120deg, var(--accent) 0%, color-mix(in srgb, var(--accent) 55%, transparent) 130%)",
          color: "#fff",
        }}
      >
        <img
          src={logoUrl || DEFAULT_BRAND_LOGO}
          alt=""
          className="h-9 w-9 rounded-lg bg-white/90 object-contain p-1"
        />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-bold leading-tight">{providerName}</p>
          <p className="text-[11px] uppercase tracking-[0.2em] opacity-90">Kartu Pelanggan</p>
        </div>
        <span
          className="shrink-0 rounded-full bg-white/20 px-2.5 py-1 text-[11px] font-semibold"
          style={{ color: "#fff" }}
        >
          {st.label}
        </span>
      </div>

      {/* multi akun */}
      {accounts.length > 1 ? (
        <div className="flex flex-wrap gap-1.5 border-b border-[var(--border)] px-4 py-2.5 sm:px-5">
          {accounts.map((a) => {
            const key = a.id || a.customer_code;
            const active = key === (current.id || current.customer_code);
            return (
              <button
                key={key}
                type="button"
                onClick={() => {
                  setActiveId(key);
                  setShowId(false);
                }}
                className={`rounded-full border px-2.5 py-1 text-xs transition-colors ${
                  active
                    ? "border-[var(--accent)] bg-[color-mix(in_srgb,var(--accent)_12%,transparent)] font-semibold text-[var(--text)]"
                    : "border-[var(--border)] text-[var(--muted)] hover:border-[var(--border-strong)]"
                }`}
              >
                {a.customer_code}
              </button>
            );
          })}
        </div>
      ) : null}

      {/* badan kartu */}
      <div className="flex gap-4 px-4 py-4 sm:px-5">
        <div className="flex shrink-0 flex-col items-center gap-2">
          <span
            className="flex h-16 w-16 items-center justify-center rounded-2xl text-xl font-bold text-white sm:h-20 sm:w-20 sm:text-2xl"
            style={{ background: "linear-gradient(140deg, var(--accent), color-mix(in srgb, var(--accent) 55%, #000))" }}
            aria-hidden
          >
            {initials(current.full_name)}
          </span>
          <span
            className="inline-block h-2 w-2 rounded-full"
            style={{ background: st.tone }}
            title={st.label}
          />
        </div>
        <div className="grid min-w-0 flex-1 grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
          <Field label="Nama" value={current.full_name} />
          <Field label="ID Pelanggan" value={<span className="font-mono">{current.customer_code}</span>} />
          <Field label="Telepon" value={current.phone} />
          <Field label="Email" value={current.email?.trim() || "—"} />
          <div className="min-w-0 sm:col-span-2">
            <p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--muted)]">
              {identityLabel(current.identity_type)}
            </p>
            <p className="mt-0.5 flex items-center gap-2">
              <span className="truncate font-mono text-sm font-medium tracking-wider text-[var(--text)]">
                {idNumber ? idValue : "—"}
              </span>
              {idNumber ? (
                <button
                  type="button"
                  onClick={() => setShowId((v) => !v)}
                  className="shrink-0 rounded-md border border-[var(--border)] p-1.5 text-[var(--muted)] hover:border-[var(--border-strong)] hover:text-[var(--text)]"
                  title={showId ? "Sembunyikan" : "Tampilkan"}
                  aria-label={showId ? "Sembunyikan nomor identitas" : "Tampilkan nomor identitas"}
                  aria-pressed={showId}
                >
                  {showId ? <IconEyeOff /> : <IconEye />}
                </button>
              ) : null}
            </p>
          </div>
          <Field label="Alamat" value={current.address?.trim() || "—"} />
          <Field label="Cluster / POP" value={current.cluster_name?.trim() || "—"} />
        </div>
      </div>

      {/* footer */}
      <div className="border-t border-[var(--border)] bg-[var(--panel-muted)]/40 px-4 py-2 sm:px-5">
        <p className="text-[11px] text-[var(--muted)]">
          ID <span className="font-mono font-medium text-[var(--text)]">{current.customer_code}</span>
          {" · "}Tunjukkan kartu ini saat membayar di loket atau menghubungi admin.
        </p>
      </div>
    </section>
  );
}
