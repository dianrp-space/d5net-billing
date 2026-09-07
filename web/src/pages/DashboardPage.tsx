import { useQuery } from "@tanstack/react-query";
import ReactECharts from "echarts-for-react";
import { api, apiDownload } from "../api";
import { IconTicket, IconUsers } from "../icons";
import type { AdminPage } from "../admin/pages";
import { AlertsPanel } from "../AdminExtra";
import { Card, formatRp, Table, Button } from "../ui";
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
  const s = stats.data ?? {};
  const series = Array.isArray(chart.data) ? chart.data : [];
  const today = new Date().toLocaleDateString("id-ID", { day: "numeric", month: "short", year: "numeric" });
  const greetName = (userName || "").trim();

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
          <Button
            type="button"
            onClick={() =>
              void apiDownload("/api/reports/invoices.csv", "invoices.csv").catch((e: Error) =>
                toastError(e.message || "Export gagal"),
              )
            }
          >
            Export CSV
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card title="Pelanggan aktif" value={Number(s.active_customers ?? 0)} />
        <Card title="Langganan aktif" value={Number(s.active_subscriptions ?? 0)} />
        <Card title="Tagihan belum lunas" value={Number(s.unpaid_invoices ?? 0)} />
        <Card title="Pendapatan bulan ini" value={formatRp(Number(s.monthly_revenue ?? 0))} />
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="panel-card panel-card-pad lg:col-span-2">
          <div className="mb-4 flex items-center justify-between gap-2">
            <h3 className="eyebrow">Tren pendapatan</h3>
            <span className="text-xs text-[var(--muted)]">6 bulan terakhir</span>
          </div>
          <p className="mb-4 text-xs text-[var(--muted)]">
            Total bulan ini: <span className="text-xl font-bold text-[var(--text)]">{formatRp(Number(s.monthly_revenue ?? 0))}</span>
          </p>
          <ReactECharts
            style={{ height: 280 }}
            option={{
              backgroundColor: "transparent",
              textStyle: { color: "var(--chart-axis)", fontFamily: "Plus Jakarta Sans" },
              grid: { left: 48, right: 12, top: 16, bottom: 32 },
              xAxis: {
                type: "category",
                data: series.map((x) => x.month),
                axisLine: { lineStyle: { color: "var(--border)" } },
              },
              yAxis: {
                type: "value",
                splitLine: { lineStyle: { color: "var(--chart-grid)" } },
              },
              series: [
                {
                  type: "bar",
                  data: series.map((x) => x.revenue),
                  itemStyle: { color: "var(--chart-bar)", borderRadius: [4, 4, 0, 0] },
                  barMaxWidth: 28,
                },
              ],
              tooltip: { trigger: "axis" },
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

function StatusPill({ status }: { status: string }) {
  const tone =
    status === "paid" || status === "success"
      ? "bg-[rgba(43,154,102,0.1)] text-[var(--ok)]"
      : status === "overdue" || status === "canceled"
        ? "bg-[rgba(220,38,38,0.1)] text-[var(--danger)]"
        : "bg-[rgba(245,158,11,0.12)] text-[var(--warn)]";
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-1 text-[10px] font-bold ${tone}`}>
      {status}
    </span>
  );
}
