import { useQuery } from "@tanstack/react-query";
import ReactEChartsCore from "echarts-for-react/lib/core";
import echarts from "../echarts";
import { api, apiDownload } from "../api";
import { formatDate } from "../tenantTime";
import {
  IconBox,
  IconDownload,
  IconHeadset,
  IconMap,
  IconRouter,
  IconTicket,
  IconUserPlus,
  IconUsers,
} from "../icons";
import type { AdminPage } from "../admin/pages";
import { AlertsPanel } from "../AdminExtra";
import { useAppDialog } from "../confirm";
import { useChartColors } from "../theme";
import { Card, formatRp, invoiceStatusLabel, Table, Button } from "../ui";
import { toastError } from "../swal";

export function DashboardPage({
  userName,
  fieldOps,
  onNavigate,
}: {
  userName?: string;
  fieldOps?: boolean;
  onNavigate: (p: AdminPage) => void;
}) {
  const { confirm } = useAppDialog();
  const stats = useQuery({
    queryKey: ["stats", fieldOps ? "field" : "admin"],
    queryFn: () => api<Record<string, number | string>>("/api/dashboard/stats"),
  });
  const chart = useQuery({
    queryKey: ["revenue"],
    queryFn: () => api<{ month: string; revenue: number }[]>("/api/dashboard/revenue-chart?months=6"),
    enabled: !fieldOps,
  });
  const invoices = useQuery({
    queryKey: ["invoices-recent"],
    queryFn: () =>
      api<{ data: { invoice_number: string; customer_name: string; total_amount: number; status: string }[] }>(
        "/api/invoices?limit=8",
      ),
    enabled: !fieldOps,
  });
  const myTickets = useQuery({
    queryKey: ["tickets", "dash"],
    queryFn: () => api<{ data: { id: string; subject: string; status: string; priority: string; customer_name?: string }[] }>("/api/tickets?limit=8&offset=0"),
    enabled: Boolean(fieldOps),
  });
  const myLeads = useQuery({
    queryKey: ["leads", "dash"],
    queryFn: () => api<{ data: { id: string; full_name: string; status: string; phone: string }[]; total: number }>("/api/leads?limit=8&offset=0"),
    enabled: Boolean(fieldOps),
  });
  const openTicketsQ = useQuery({
    queryKey: ["tickets-open-count"],
    queryFn: () => api<{ total: number }>("/api/tickets?status=open&limit=1"),
    enabled: !fieldOps,
    retry: false,
  });
  const overdueQ = useQuery({
    queryKey: ["invoices-overdue-count"],
    queryFn: () => api<{ total: number }>("/api/invoices?status=overdue&limit=1"),
    enabled: !fieldOps,
    retry: false,
  });
  const suspendedQ = useQuery({
    queryKey: ["subs-suspended-count"],
    queryFn: () => api<{ total: number }>("/api/subscriptions?status=suspended&limit=1"),
    enabled: !fieldOps,
    retry: false,
  });
  const routersQ = useQuery({
    queryKey: ["routers"],
    queryFn: () =>
      api<{ id: string; name: string; is_active: boolean; last_seen_at?: string | null; last_error?: string | null }[]>(
        "/api/routers",
      ),
    enabled: !fieldOps,
    retry: false,
  });
  const s = stats.data ?? {};
  const series = Array.isArray(chart.data) ? chart.data : [];
  const today = formatDate(new Date(), { day: "numeric", month: "short", year: "numeric" });
  const greetName = (userName || "").trim();
  const chartColors = useChartColors();

  async function exportInvoices() {
    const ok = await confirm({
      title: "Export tagihan",
      description: "Unduh data tagihan sebagai CSV?",
      confirmLabel: "Unduh",
    });
    if (!ok) return;
    try {
      await apiDownload("/api/reports/invoices.csv", "invoices.csv");
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Export gagal");
    }
  }

  if (fieldOps) {
    return (
      <div className="space-y-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-2xl font-bold tracking-tight">
            Selamat datang{greetName ? `, ${greetName}` : ""}
          </h2>
          <span className="btn-ghost text-sm">{today}</span>
        </div>

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
          <Card title="Lead di-assign" value={Number(s.leads_assigned ?? 0)} />
          <Card title="Lead proses pasang" value={Number(s.leads_install ?? 0)} />
          <Card title="Tiket open" value={Number(s.tickets_open ?? 0)} />
          <Card title="Tiket proses" value={Number(s.tickets_in_progress ?? 0)} />
        </div>

        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <div className="panel-card overflow-hidden">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border)] px-6 py-4">
              <h3 className="eyebrow">Lead saya</h3>
              <Button type="button" variant="outline" size="sm" onClick={() => onNavigate("leads")}>
                Lihat semua
              </Button>
            </div>
            <Table
              columns={["Nama", "Telepon", "Status"]}
              rows={(myLeads.data?.data ?? []).map((l) => [
                l.full_name,
                l.phone,
                l.status === "qualified"
                  ? "Proses pasang"
                  : l.status === "contacted"
                    ? "Dihubungi"
                    : l.status === "survey"
                      ? "Survey"
                      : l.status,
              ])}
            />
          </div>
          <div className="panel-card overflow-hidden">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border)] px-6 py-4">
              <h3 className="eyebrow">Tiket saya</h3>
              <Button type="button" variant="outline" size="sm" onClick={() => onNavigate("tickets")}>
                Lihat semua
              </Button>
            </div>
            <Table
              columns={["Subjek", "Pelanggan", "Status"]}
              rows={(myTickets.data?.data ?? []).map((t) => [
                t.subject,
                t.customer_name || "—",
                t.status === "in_progress" ? "Proses" : t.status,
              ])}
            />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-2xl font-bold tracking-tight">
          Selamat datang{greetName ? `, ${greetName}` : ""}
        </h2>
        <div className="flex flex-wrap items-center gap-2">
          <span className="btn-ghost text-sm">{today}</span>
          <Button type="button" onClick={() => void exportInvoices()}>
            Export CSV
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card title="Pelanggan aktif" value={Number(s.active_customers ?? 0)} onClick={() => onNavigate("customers")} />
        <Card title="Langganan aktif" value={Number(s.active_subscriptions ?? 0)} onClick={() => onNavigate("customers")} />
        <Card title="Tagihan belum lunas" value={Number(s.unpaid_invoices ?? 0)} onClick={() => onNavigate("invoices")} />
        <Card
          title="Pendapatan bulan ini"
          value={formatRp(Number(s.monthly_revenue ?? 0))}
          onClick={() => onNavigate("payments")}
        />
      </div>

      <AttentionStrip
        openTickets={openTicketsQ.data?.total ?? 0}
        overdue={overdueQ.data?.total ?? 0}
        suspended={suspendedQ.data?.total ?? 0}
        routersOffline={routersQ.data?.filter((r) => r.is_active && (!r.last_seen_at || r.last_error)).length ?? 0}
        onNavigate={onNavigate}
      />

      <QuickActions onNavigate={onNavigate} />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="panel-card panel-card-pad lg:col-span-2">
          <div className="mb-4 flex items-center justify-between gap-2">
            <h3 className="eyebrow">Tren pendapatan</h3>
            <span className="text-xs text-[var(--muted)]">6 bulan terakhir</span>
          </div>
          <p className="mb-4 text-xs text-[var(--muted)]">
            Total bulan ini: <span className="text-xl font-bold text-[var(--text)]">{formatRp(Number(s.monthly_revenue ?? 0))}</span>
          </p>
          <ReactEChartsCore
            echarts={echarts}
            style={{ height: 280 }}
            option={{
              backgroundColor: "transparent",
              textStyle: { color: chartColors.axis, fontFamily: "Plus Jakarta Sans" },
              grid: { left: 48, right: 12, top: 16, bottom: 32 },
              xAxis: {
                type: "category",
                data: series.map((x) => x.month),
                axisLine: { lineStyle: { color: chartColors.border } },
                axisTick: { lineStyle: { color: chartColors.border } },
                axisLabel: { color: chartColors.axis },
              },
              yAxis: {
                type: "value",
                splitLine: { lineStyle: { color: chartColors.grid } },
                axisLabel: { color: chartColors.axis },
              },
              series: [
                {
                  type: "bar",
                  data: series.map((x) => x.revenue),
                  itemStyle: { color: chartColors.bar, borderRadius: [4, 4, 0, 0] },
                  barMaxWidth: 28,
                },
              ],
              tooltip: {
                trigger: "axis",
                backgroundColor: chartColors.panel,
                borderColor: chartColors.border,
                textStyle: { color: chartColors.text },
              },
            }}
          />
        </div>

        <div className="panel-card panel-card-pad flex flex-col">
          <h3 className="eyebrow mb-4">Alert & monitoring</h3>
          <div className="flex-1 overflow-auto">
            <AlertsPanel />
          </div>
        </div>
      </div>

      <div className="panel-card overflow-hidden">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border)] px-6 py-4">
          <h3 className="eyebrow">Tagihan terbaru</h3>
          <Button type="button" variant="outline" size="sm" onClick={() => onNavigate("invoices")}>
            Lihat semua
          </Button>
        </div>
        <Table
          columns={["Nomor", "Pelanggan", "Total", "Status"]}
          rows={(invoices.data?.data ?? []).map((i) => [
            i.invoice_number,
            i.customer_name,
            formatRp(i.total_amount),
            <StatusPill key={i.invoice_number} status={i.status} />,
          ])}
        />
      </div>
    </div>
  );
}

function AttentionStrip({
  openTickets,
  overdue,
  suspended,
  routersOffline,
  onNavigate,
}: {
  openTickets: number;
  overdue: number;
  suspended: number;
  routersOffline: number;
  onNavigate: (p: AdminPage) => void;
}) {
  const items: { label: string; count: number; tone: string; page: AdminPage }[] = [];
  if (openTickets > 0) {
    items.push({ label: "Tiket open", count: openTickets, tone: "var(--warn, #b7791f)", page: "tickets" });
  }
  if (overdue > 0) {
    items.push({ label: "Tagihan overdue", count: overdue, tone: "var(--danger)", page: "invoices" });
  }
  if (suspended > 0) {
    items.push({ label: "Terisolir", count: suspended, tone: "var(--danger)", page: "customers" });
  }
  if (routersOffline > 0) {
    items.push({ label: "Router offline", count: routersOffline, tone: "var(--danger)", page: "routers" });
  }
  if (items.length === 0) return null;
  return (
    <section aria-label="Perlu perhatian">
      <h3 className="eyebrow mb-3">Perlu perhatian</h3>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {items.map((it) => (
          <button
            key={it.label}
            type="button"
            onClick={() => onNavigate(it.page)}
            className="panel-card flex items-center gap-3 p-4 text-left transition-colors hover:border-[var(--accent)]"
          >
            <span className="h-9 w-1.5 shrink-0 rounded-full" style={{ background: it.tone }} aria-hidden />
            <span className="min-w-0">
              <span className="block text-2xl font-bold leading-none">{it.count}</span>
              <span className="mt-1 block truncate text-sm text-[var(--muted)]">
                {it.label} <span aria-hidden>→</span>
              </span>
            </span>
          </button>
        ))}
      </div>
    </section>
  );
}

function QuickActions({ onNavigate }: { onNavigate: (p: AdminPage) => void }) {
  const actions: { label: string; icon: React.ReactNode; page: AdminPage }[] = [
    { label: "Pelanggan", icon: <IconUsers />, page: "customers" },
    { label: "Paket", icon: <IconBox />, page: "plans" },
    { label: "Router", icon: <IconRouter />, page: "routers" },
    { label: "Tiket", icon: <IconHeadset />, page: "tickets" },
    { label: "Lead", icon: <IconUserPlus />, page: "leads" },
    { label: "MAP FTTH", icon: <IconMap />, page: "odp" },
    { label: "Voucher", icon: <IconTicket />, page: "vouchers" },
    { label: "Backup", icon: <IconDownload />, page: "backup" },
  ];
  return (
    <section aria-label="Aksi cepat">
      <h3 className="eyebrow mb-3">Aksi cepat</h3>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {actions.map((a) => (
          <button
            key={a.page}
            type="button"
            onClick={() => onNavigate(a.page)}
            className="group flex cursor-pointer items-center gap-3 rounded-[var(--radius-lg)] border-2 border-[var(--border)] bg-[var(--panel)] p-3.5 text-left shadow-[var(--shadow-sm)] transition-all hover:-translate-y-0.5 hover:border-[var(--accent)] hover:shadow-md active:translate-y-0 active:scale-[0.98]"
          >
            <span
              className="grid h-10 w-10 shrink-0 place-items-center rounded-xl text-[var(--accent)] transition-colors group-hover:bg-[var(--accent)] group-hover:text-white"
              style={{ background: "color-mix(in srgb, var(--accent) 14%, transparent)" }}
              aria-hidden
            >
              {a.icon}
            </span>
            <span className="min-w-0 flex-1 truncate text-sm font-semibold">{a.label}</span>
            <span
              className="shrink-0 text-[var(--muted)] transition-all group-hover:translate-x-0.5 group-hover:text-[var(--accent)]"
              aria-hidden
            >
              →
            </span>
          </button>
        ))}
      </div>
    </section>
  );
}

function StatusPill({ status }: { status: string }) {
  const tone =
    status === "paid" || status === "success"
      ? "bg-[rgba(43,154,102,0.1)] text-[var(--ok)]"
      : status === "overdue" || status === "canceled" || status === "cancelled" || status === "void"
        ? "bg-[rgba(220,38,38,0.1)] text-[var(--danger)]"
        : "bg-[rgba(245,158,11,0.12)] text-[var(--warn)]";
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-1 text-[10px] font-bold ${tone}`}>
      {invoiceStatusLabel(status)}
    </span>
  );
}
