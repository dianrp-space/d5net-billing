import { useEffect, useState } from "react";
import { api, clearClientSession } from "./api";
import { applyBrandingMeta } from "./branding";
import type { ClientPortalData } from "./ClientLogin";
import { toastError, toastSuccess } from "./swal";
import { ThemeToggle } from "./ThemeToggle";
import { Card, formatRp, Section, SecretInput, Table } from "./ui";

export function ClientHome({
  data,
  onLogout,
}: {
  data: ClientPortalData;
  onLogout: () => void;
}) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [formErr, setFormErr] = useState("");

  // Session restore renders here directly (bypassing ClientLogin),
  // so ensure the tab title too.
  useEffect(() => {
    applyBrandingMeta({
      appName: data.tenant_name || data.tenant_slug,
      titleSuffix: "Portal Pelanggan",
      separator: "-",
    });
  }, [data.tenant_name, data.tenant_slug]);

  const accounts = data.customers?.length
    ? data.customers
    : data.customer
      ? [{ id: data.customer.id || "", full_name: data.customer.full_name, phone: data.customer.phone, customer_code: data.customer.customer_code }]
      : [];
  const multi = accounts.length > 1;
  const [pwAccount, setPwAccount] = useState("");
  const pwAccountId = pwAccount || accounts[0]?.id || "";
  const accountLabel = (code?: string, name?: string) =>
    code ? `${code}${name ? ` · ${name}` : ""}` : name || "—";

  async function onChangePassword(e: React.FormEvent) {
    e.preventDefault();
    setFormErr("");
    const phone = data.customer?.phone?.trim() || "";
    const slug = data.tenant_slug?.trim() || "";
    if (!phone || !slug) {
      setFormErr("Sesi portal tidak lengkap. Silakan login ulang.");
      return;
    }
    if (multi && !pwAccountId) {
      setFormErr("Pilih akun yang passwordnya diubah.");
      return;
    }
    if (newPassword.length < 6) {
      setFormErr("Password baru minimal 6 karakter.");
      return;
    }
    if (newPassword !== confirmPassword) {
      setFormErr("Konfirmasi password tidak cocok.");
      return;
    }
    setBusy(true);
    try {
      await api("/api/portal/change-password", {
        method: "POST",
        body: JSON.stringify({
          tenant_slug: slug,
          phone,
          current_password: currentPassword,
          new_password: newPassword,
          ...(multi && pwAccountId ? { customer_id: pwAccountId } : {}),
        }),
      });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      void toastSuccess("Password berhasil diubah");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Gagal ganti password";
      setFormErr(msg);
      void toastError(msg);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-3xl p-6">
      <header className="mb-6 flex items-center justify-between">
        <div>
          <div className="text-sm text-[var(--muted)]">{data.tenant_name || data.tenant_slug || "drp-billing"}</div>
          <h1 className="text-xl font-semibold">Halo, {data.customer?.full_name ?? ""}</h1>
          {data.customer?.phone ? (
            <p className="text-xs text-[var(--muted)]">Login: {data.customer.phone}</p>
          ) : null}
          {multi ? (
            <div className="mt-2 flex flex-wrap gap-1.5">
              {accounts.map((a) => (
                <span
                  key={a.id || a.customer_code}
                  className="rounded-full border border-[var(--border)] bg-[var(--panel)] px-2 py-0.5 text-xs text-[var(--muted)]"
                >
                  {accountLabel(a.customer_code, a.full_name)}
                </span>
              ))}
            </div>
          ) : null}
        </div>
        <div className="flex items-center gap-2">
          <ThemeToggle />
          <button
            className="btn-ghost"
            onClick={() => {
              clearClientSession();
              onLogout();
            }}
          >
            Keluar
          </button>
        </div>
      </header>
      <Card title={multi ? `Saldo gabungan (${accounts.length} akun)` : "Saldo"} value={formatRp(data.wallet_balance ?? 0)} />
      <div className="mt-6">
        <Section title="Paket / Langganan">
          <Table
            columns={multi ? ["Akun", "Username", "Paket", "Status"] : ["Username", "Paket", "Status"]}
            rows={(data.subscriptions ?? []).map((s) =>
              multi
                ? [accountLabel(s.customer_code, s.customer_name), s.username, s.plan_name, s.status]
                : [s.username, s.plan_name, s.status],
            )}
          />
        </Section>
      </div>
      <div className="mt-6">
        <Section title="Tagihan">
          <Table
            columns={multi ? ["Akun", "Nomor", "Total", "Jatuh tempo", "Status"] : ["Nomor", "Total", "Jatuh tempo", "Status"]}
            rows={(data.invoices ?? []).map((i) =>
              multi
                ? [
                    accountLabel(i.customer_code, i.customer_name),
                    i.invoice_number,
                    formatRp(i.total_amount),
                    i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—",
                    i.status,
                  ]
                : [
                    i.invoice_number,
                    formatRp(i.total_amount),
                    i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—",
                    i.status,
                  ],
            )}
          />
        </Section>
      </div>
      <div className="mt-6">
        <Section title="Riwayat pembayaran">
          <Table
            columns={multi ? ["Akun", "Tanggal", "Jumlah", "Metode", "Status"] : ["Tanggal", "Jumlah", "Metode", "Status"]}
            rows={(data.payments ?? []).map((p) =>
              multi
                ? [
                    accountLabel(p.customer_code, p.customer_name),
                    p.paid_at || p.created_at
                      ? new Date(p.paid_at || p.created_at!).toLocaleString("id-ID")
                      : "—",
                    formatRp(p.amount),
                    p.method,
                    p.status,
                  ]
                : [
                    p.paid_at || p.created_at
                      ? new Date(p.paid_at || p.created_at!).toLocaleString("id-ID")
                      : "—",
                    formatRp(p.amount),
                    p.method,
                    p.status,
                  ],
            )}
          />
        </Section>
      </div>
      <div className="mt-6">
        <Section title="Ganti password">
          <form className="grid max-w-md gap-3" onSubmit={onChangePassword}>
            {multi ? (
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Akun</span>
                <select
                  className="input"
                  value={pwAccountId}
                  onChange={(e) => setPwAccount(e.target.value)}
                >
                  {accounts.map((a) => (
                    <option key={a.id || a.customer_code} value={a.id}>
                      {accountLabel(a.customer_code, a.full_name)}
                    </option>
                  ))}
                </select>
              </label>
            ) : null}
            <SecretInput
              placeholder="Password saat ini"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
              autoComplete="current-password"
            />
            <SecretInput
              placeholder="Password baru (min 6)"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              minLength={6}
              autoComplete="new-password"
            />
            <SecretInput
              placeholder="Ulangi password baru"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              minLength={6}
              autoComplete="new-password"
            />
            <button className="btn w-fit" disabled={busy}>
              {busy ? "Menyimpan..." : "Simpan password"}
            </button>
            {formErr && <p className="text-sm text-[var(--danger)]">{formErr}</p>}
          </form>
        </Section>
      </div>
    </div>
  );
}
