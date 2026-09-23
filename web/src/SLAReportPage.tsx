import { useMemo, useState } from "react";
import { formatDateTime } from "./tenantTime";
import { useQuery } from "@tanstack/react-query";
import ReactEChartsCore from "echarts-for-react/lib/core";
import echarts from "./echarts";
import { api } from "./api";
import { Badge } from "./components/ui/badge";
import { Card, Label, Section, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Table } from "./ui";

type SLASample = {
  id: string;
  subject: string;
  category: string;
  priority: string;
  status: string;
  customer_name?: string;
  assignee_name?: string;
  created_at: string;
  resolved_at?: string | null;
  sla_due_at?: string | null;
  resolve_hours?: number | null;
  sla_met?: boolean | null;
  breached?: boolean;
};

type SLAReport = {
  days: number;
  from: string;
  to: string;
  total: number;
  open_count: number;
  in_progress_count: number;
  resolved_count: number;
  closed_count: number;
  outage_count: number;
  outage_pct: number;
  by_category: Record<string, number>;
  by_priority: Record<string, number>;
  by_status: Record<string, number>;
  sla_applicable: number;
  sla_met: number;
  sla_breached: number;
  sla_met_pct: number;
  open_breaching: number;
  avg_resolve_hours: number;
  median_resolve_hours: number;
  p95_resolve_hours: number;
  avg_by_category_hours: Record<string, number>;
  avg_by_priority_hours: Record<string, number>;
  slowest_resolved: SLASample[];
  open_breaching_list: SLASample[];
};

const categoryLabels: Record<string, string> = {
  general: "Umum",
  billing: "Billing",
  technical: "Teknis",
  outage: "Gangguan",
  installation: "Instalasi",
};

const priorityLabels: Record<string, string> = {
  low: "Rendah",
  normal: "Normal",
  high: "Tinggi",
  urgent: "Urgent",
};

const statusLabels: Record<string, string> = {
  open: "Open",
  in_progress: "Proses",
  resolved: "Resolved",
  closed: "Closed",
};

function formatHours(h?: number | null) {
  if (h == null || Number.isNaN(h)) return "—";
  if (h < 1) return `${Math.round(h * 60)} mnt`;
  if (h < 48) return `${h.toFixed(1)} jam`;
  return `${(h / 24).toFixed(1)} hari`;
}

function formatPct(n?: number | null) {
  if (n == null || Number.isNaN(n)) return "0%";
  return `${n.toFixed(1)}%`;
}

function formatWhen(iso?: string | null) {
  return formatDateTime(iso);
}

function entriesSorted(map: Record<string, number> | undefined) {
  return Object.entries(map || {}).sort((a, b) => b[1] - a[1]);
}

export function SLAReportPage() {
  const [days, setDays] = useState("30");
  const q = useQuery({
    queryKey: ["sla-report", days],
    queryFn: () => api<SLAReport>(`/api/reports/sla?days=${encodeURIComponent(days)}`),
  });
  const r = q.data;

  const categoryChart = useMemo(() => {
    const rows = entriesSorted(r?.by_category);
    return {
      labels: rows.map(([k]) => categoryLabels[k] || k),
      values: rows.map(([, v]) => v),
    };
  }, [r?.by_category]);

  const resolveChart = useMemo(() => {
    const rows = entriesSorted(r?.avg_by_category_hours).map(([k, v]) => ({
      name: categoryLabels[k] || k,
      value: Number(v.toFixed(2)),
    }));
    return rows;
  }, [r?.avg_by_category_hours]);

  return (
    <Section
      title="Laporan SLA"
      actions={
        <div className="min-w-[160px]">
          <Label className="mb-1.5 block text-xs">Periode</Label>
          <Select value={days} onValueChange={setDays}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="7">7 hari</SelectItem>
              <SelectItem value="30">30 hari</SelectItem>
              <SelectItem value="90">90 hari</SelectItem>
              <SelectItem value="180">180 hari</SelectItem>
              <SelectItem value="365">1 tahun</SelectItem>
            </SelectContent>
          </Select>
        </div>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Evaluasi manajemen: kepatuhan SLA, proporsi gangguan, dan waktu penyelesaian tiket sampai resolved.
        {r ? ` Periode ${formatWhen(r.from)} – ${formatWhen(r.to)}.` : ""}
      </p>

      <div className="mb-6 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/40 p-4 text-sm">
        <h3 className="mb-2 text-sm font-semibold text-[var(--text)]">Apa arti istilah di laporan ini?</h3>
        <ul className="grid gap-2 text-[var(--muted)] sm:grid-cols-2">
          <li>
            <span className="font-medium text-[var(--text)]">SLA</span> — batas waktu target penyelesaian tiket sejak dibuat.
            Urgent/tinggi: <strong className="text-[var(--text)]">4 jam</strong>; prioritas lain:{" "}
            <strong className="text-[var(--text)]">24 jam</strong>.
          </li>
          <li>
            <span className="font-medium text-[var(--text)]">Breach / melanggar SLA</span> — tiket diselesaikan{" "}
            <em>setelah</em> batas SLA, atau masih open/proses padahal batas SLA sudah lewat.
          </li>
          <li>
            <span className="font-medium text-[var(--text)]">SLA terpenuhi</span> — tiket resolved/closed sebelum atau tepat
            pada batas SLA.
          </li>
          <li>
            <span className="font-medium text-[var(--text)]">% Gangguan</span> — porsi tiket kategori{" "}
            <em>Gangguan (outage)</em> dari total tiket di periode.
          </li>
          <li>
            <span className="font-medium text-[var(--text)]">Rata-rata / median / P95 resolve</span> — lama dari dibuat sampai
            resolved. Median = nilai tengah; P95 = 95% tiket selesai lebih cepat dari angka ini.
          </li>
          <li>
            <span className="font-medium text-[var(--text)]">Open lewat SLA</span> — tiket masih open/proses dan sudah
            melewati batas SLA (butuh perhatian segera).
          </li>
        </ul>
      </div>

      {q.isLoading ? <p className="text-sm text-[var(--muted)]">Memuat laporan…</p> : null}
      {q.isError ? <p className="text-sm text-[var(--danger)]">{(q.error as Error).message}</p> : null}

      {r ? (
        <div className="space-y-6">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Card title="Total tiket" value={r.total} hint="Semua tiket yang dibuat dalam periode." />
            <Card
              title="% Gangguan"
              value={formatPct(r.outage_pct)}
              hint={`${r.outage_count} tiket kategori Gangguan dari ${r.total} tiket.`}
            />
            <Card
              title="SLA terpenuhi"
              value={formatPct(r.sla_met_pct)}
              hint={`${r.sla_met} dari ${r.sla_applicable} tiket yang dinilai SLA selesai tepat waktu.`}
            />
            <Card
              title="Rata-rata resolve"
              value={formatHours(r.avg_resolve_hours)}
              hint="Rata-rata waktu dari tiket dibuat sampai status resolved/closed."
            />
          </div>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Card
              title="Median resolve"
              value={formatHours(r.median_resolve_hours)}
              hint="Separuh tiket selesai lebih cepat, separuh lebih lama dari angka ini."
            />
            <Card
              title="P95 resolve"
              value={formatHours(r.p95_resolve_hours)}
              hint="Hampir semua tiket (95%) selesai dalam waktu ini atau lebih cepat."
            />
            <Card
              title="Melanggar SLA"
              value={r.sla_breached}
              hint="Jumlah breach: resolved terlambat + open/proses yang sudah lewat batas SLA."
            />
            <Card
              title="Open lewat SLA"
              value={r.open_breaching}
              hint="Subset breach: tiket yang masih berjalan dan sudah lewat batas SLA."
            />
          </div>

          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div className="panel-card panel-card-pad">
              <h3 className="eyebrow mb-1">Komposisi kategori</h3>
              <p className="mb-4 text-xs text-[var(--muted)]">
                Gangguan: {r.outage_count} dari {r.total} tiket ({formatPct(r.outage_pct)})
              </p>
              <ReactEChartsCore
                echarts={echarts}
                style={{ height: 260 }}
                option={{
                  backgroundColor: "transparent",
                  textStyle: { color: "var(--chart-axis)", fontFamily: "Plus Jakarta Sans" },
                  tooltip: { trigger: "item" },
                  series: [
                    {
                      type: "pie",
                      radius: ["42%", "68%"],
                      data: categoryChart.labels.map((name, i) => ({
                        name,
                        value: categoryChart.values[i],
                      })),
                      label: { color: "var(--text)" },
                    },
                  ],
                }}
              />
            </div>

            <div className="panel-card panel-card-pad">
              <h3 className="eyebrow mb-1">Rata-rata waktu resolve</h3>
              <p className="mb-4 text-xs text-[var(--muted)]">Per kategori (jam)</p>
              <ReactEChartsCore
                echarts={echarts}
                style={{ height: 260 }}
                option={{
                  backgroundColor: "transparent",
                  textStyle: { color: "var(--chart-axis)", fontFamily: "Plus Jakarta Sans" },
                  grid: { left: 48, right: 12, top: 16, bottom: 48 },
                  xAxis: {
                    type: "category",
                    data: resolveChart.map((x) => x.name),
                    axisLabel: { rotate: 20 },
                    axisLine: { lineStyle: { color: "var(--border)" } },
                  },
                  yAxis: {
                    type: "value",
                    name: "jam",
                    splitLine: { lineStyle: { color: "var(--chart-grid)" } },
                  },
                  series: [
                    {
                      type: "bar",
                      data: resolveChart.map((x) => x.value),
                      itemStyle: { color: "var(--chart-bar)", borderRadius: [4, 4, 0, 0] },
                      barMaxWidth: 36,
                    },
                  ],
                  tooltip: { trigger: "axis" },
                }}
              />
            </div>
          </div>

          <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div className="panel-card panel-card-pad">
              <h3 className="eyebrow mb-3">Status</h3>
              <ul className="space-y-2 text-sm">
                {entriesSorted(r.by_status).map(([k, v]) => (
                  <li key={k} className="flex justify-between gap-2">
                    <span>{statusLabels[k] || k}</span>
                    <span className="font-semibold">{v}</span>
                  </li>
                ))}
                {entriesSorted(r.by_status).length === 0 ? (
                  <li className="text-[var(--muted)]">Tidak ada data</li>
                ) : null}
              </ul>
            </div>
            <div className="panel-card panel-card-pad">
              <h3 className="eyebrow mb-3">Prioritas</h3>
              <ul className="space-y-2 text-sm">
                {entriesSorted(r.by_priority).map(([k, v]) => (
                  <li key={k} className="flex justify-between gap-2">
                    <span>{priorityLabels[k] || k}</span>
                    <span className="font-semibold">
                      {v}
                      {r.avg_by_priority_hours?.[k] != null ? (
                        <span className="ml-2 text-xs font-normal text-[var(--muted)]">
                          avg {formatHours(r.avg_by_priority_hours[k])}
                        </span>
                      ) : null}
                    </span>
                  </li>
                ))}
                {entriesSorted(r.by_priority).length === 0 ? (
                  <li className="text-[var(--muted)]">Tidak ada data</li>
                ) : null}
              </ul>
            </div>
            <div className="panel-card panel-card-pad">
              <h3 className="eyebrow mb-1">Ringkasan SLA</h3>
              <p className="mb-3 text-xs text-[var(--muted)]">
                Breach = melanggar batas waktu. Dihitung dari tiket yang punya target SLA.
              </p>
              <ul className="space-y-2 text-sm">
                <li className="flex justify-between gap-2">
                  <span title="Tiket resolved/closed yang punya batas SLA, plus open yang sudah lewat SLA">
                    Dinilai SLA
                  </span>
                  <span className="font-semibold">{r.sla_applicable}</span>
                </li>
                <li className="flex justify-between gap-2">
                  <span title="Selesai sebelum atau tepat pada batas SLA">Terpenuhi</span>
                  <span className="font-semibold text-[var(--ok)]">{r.sla_met}</span>
                </li>
                <li className="flex justify-between gap-2">
                  <span title="Resolved terlambat atau masih open setelah batas SLA">Melanggar (breach)</span>
                  <span className="font-semibold text-[var(--danger)]">{r.sla_breached}</span>
                </li>
                <li className="flex justify-between gap-2">
                  <span>Resolved / Closed</span>
                  <span className="font-semibold">{r.resolved_count + r.closed_count}</span>
                </li>
              </ul>
              <p className="mt-3 text-xs text-[var(--muted)]">
                Target SLA sistem: urgent/tinggi = 4 jam, prioritas lain = 24 jam sejak tiket dibuat.
              </p>
            </div>
          </div>

          <div className="panel-card overflow-hidden">
            <div className="border-b border-[var(--border)] px-6 py-4">
              <h3 className="eyebrow">Tiket open/proses lewat SLA</h3>
              <p className="mt-1 text-xs text-[var(--muted)]">
                Daftar breach yang masih berjalan — belum resolved padahal batas waktu sudah lewat.
              </p>
            </div>
            <Table
              columns={["Subjek", "Kategori", "Prioritas", "Pelanggan", "Assignee", "Umur", "Batas SLA"]}
              rows={(r.open_breaching_list || []).map((t) => [
                t.subject,
                categoryLabels[t.category] || t.category,
                <Badge key={`${t.id}-p`} variant={t.priority === "urgent" || t.priority === "high" ? "danger" : "outline"}>
                  {priorityLabels[t.priority] || t.priority}
                </Badge>,
                t.customer_name || "—",
                t.assignee_name || "—",
                formatHours(t.resolve_hours),
                formatWhen(t.sla_due_at),
              ])}
            />
          </div>

          <div className="panel-card overflow-hidden">
            <div className="border-b border-[var(--border)] px-6 py-4">
              <h3 className="eyebrow">Resolved paling lama</h3>
              <p className="mt-1 text-xs text-[var(--muted)]">
                Tiket yang sudah selesai, diurut dari durasi terlama. Badge{" "}
                <span className="font-medium text-[var(--danger)]">Lewat SLA</span> = selesai setelah batas waktu
                (breach); <span className="font-medium text-[var(--ok)]">Tepat waktu</span> = dalam SLA.
              </p>
            </div>
            <Table
              columns={["Subjek", "Kategori", "Prioritas", "Durasi", "SLA", "Resolved"]}
              rows={(r.slowest_resolved || []).map((t) => [
                <div key={`${t.id}-s`}>
                  <p className="font-medium">{t.subject}</p>
                  <p className="text-xs text-[var(--muted)]">{t.customer_name || "—"}</p>
                </div>,
                categoryLabels[t.category] || t.category,
                priorityLabels[t.priority] || t.priority,
                formatHours(t.resolve_hours),
                t.sla_met == null ? (
                  "—"
                ) : t.sla_met ? (
                  <Badge key={`${t.id}-ok`} variant="success">
                    Tepat waktu
                  </Badge>
                ) : (
                  <Badge key={`${t.id}-bad`} variant="danger">
                    Lewat SLA
                  </Badge>
                ),
                formatWhen(t.resolved_at),
              ])}
            />
          </div>
        </div>
      ) : null}
    </Section>
  );
}
