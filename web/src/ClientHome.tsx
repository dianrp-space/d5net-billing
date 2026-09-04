import { clearClientSession } from "./api";
import type { ClientPortalData } from "./ClientLogin";
import { ThemeToggle } from "./ThemeToggle";
import { Card, formatRp, Section, Table } from "./ui";

export function ClientHome({
  data,
  onLogout,
}: {
  data: ClientPortalData;
  onLogout: () => void;
}) {
  return (
    <div className="mx-auto max-w-3xl p-6">
      <header className="mb-6 flex items-center justify-between">
        <div>
          <div className="text-sm text-[var(--muted)]">{data.tenant_name || data.tenant_slug || "drp-billing"}</div>
          <h1 className="text-xl font-semibold">Halo, {data.customer?.full_name ?? ""}</h1>
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
      <Card title="Saldo" value={formatRp(data.wallet_balance ?? 0)} />
      <div className="mt-6">
        <Section title="Paket / Langganan">
          <Table
            columns={["Username", "Paket", "Status"]}
            rows={(data.subscriptions ?? []).map((s) => [s.username, s.plan_name, s.status])}
          />
        </Section>
      </div>
      <div className="mt-6">
        <Section title="Tagihan">
          <Table
            columns={["Nomor", "Total", "Jatuh tempo", "Status"]}
            rows={(data.invoices ?? []).map((i) => [
              i.invoice_number,
              formatRp(i.total_amount),
              i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—",
              i.status,
            ])}
          />
        </Section>
      </div>
      <div className="mt-6">
        <Section title="Riwayat pembayaran">
          <Table
            columns={["Tanggal", "Jumlah", "Metode", "Status"]}
            rows={(data.payments ?? []).map((p) => [
              p.paid_at || p.created_at
                ? new Date(p.paid_at || p.created_at!).toLocaleString("id-ID")
                : "—",
              formatRp(p.amount),
              p.method,
              p.status,
            ])}
          />
        </Section>
      </div>
      <p className="mt-6 text-xs text-[var(--muted)]">
        Grafik usage traffic per paket akan tampil setelah poller monitoring mencatat sample (Fase 5).
      </p>
    </div>
  );
}
