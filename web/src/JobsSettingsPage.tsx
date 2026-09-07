import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { toastError, toastSuccess } from "./swal";
import { Button, Input, Label, Section, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Table } from "./ui";
import { Checkbox } from "@/components/ui/checkbox";

type JobSchedule = {
  billing_enabled: boolean;
  isolir_enabled: boolean;
  dunning_enabled: boolean;
  dunning_offsets: number[];
  odp_outage_enabled: boolean;
  weekly_reconcile_enabled: boolean;
  weekly_reconcile_weekday: number;
  weekly_reconcile_hour: number;
  monthly_report_enabled: boolean;
  monthly_report_day: number;
  monthly_report_hour: number;
  notify_batch_size: number;
};

type JobsResponse = {
  schedule: JobSchedule;
  defaults: JobSchedule;
  catalog: { id: string; label: string; description: string }[];
};

type JobRun = {
  id: string;
  job_name: string;
  job_key: string;
  status: string;
  created_at: string;
};

const WEEKDAYS = [
  { value: 0, label: "Minggu" },
  { value: 1, label: "Senin" },
  { value: 2, label: "Selasa" },
  { value: 3, label: "Rabu" },
  { value: 4, label: "Kamis" },
  { value: 5, label: "Jumat" },
  { value: 6, label: "Sabtu" },
];

function formatWhen(iso?: string) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("id-ID");
  } catch {
    return iso;
  }
}

function offsetsToText(offsets: number[] | undefined) {
  return (offsets ?? []).join(", ");
}

function parseOffsets(raw: string): number[] {
  return raw
    .split(/[,\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n));
}

export function JobsSettingsPage() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["jobs-settings"],
    queryFn: () => api<JobsResponse>("/api/settings/jobs"),
  });
  const runsQ = useQuery({
    queryKey: ["jobs-runs"],
    queryFn: () => api<{ data: JobRun[] }>("/api/settings/jobs/runs?limit=30"),
  });

  const [form, setForm] = useState<JobSchedule | null>(null);
  const [offsetsText, setOffsetsText] = useState("-7, -3, 0, 1, 3");

  useEffect(() => {
    if (!q.data?.schedule) return;
    setForm({ ...q.data.schedule });
    setOffsetsText(offsetsToText(q.data.schedule.dunning_offsets));
  }, [q.data]);

  const save = useMutation({
    mutationFn: (body: JobSchedule) =>
      api<JobSchedule>("/api/settings/jobs", { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: (saved) => {
      setForm(saved);
      setOffsetsText(offsetsToText(saved.dunning_offsets));
      void qc.invalidateQueries({ queryKey: ["jobs-settings"] });
      void qc.invalidateQueries({ queryKey: ["jobs-runs"] });
      void toastSuccess("Jadwal cronjob disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const catalog = q.data?.catalog ?? [];
  const runs = runsQ.data?.data ?? [];

  function patch<K extends keyof JobSchedule>(key: K, value: JobSchedule[K]) {
    setForm((f) => (f ? { ...f, [key]: value } : f));
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!form) return;
    const body: JobSchedule = {
      ...form,
      dunning_offsets: parseOffsets(offsetsText),
      weekly_reconcile_weekday: Number(form.weekly_reconcile_weekday),
      weekly_reconcile_hour: Number(form.weekly_reconcile_hour),
      monthly_report_day: Number(form.monthly_report_day),
      monthly_report_hour: Number(form.monthly_report_hour),
      notify_batch_size: Number(form.notify_batch_size),
    };
    save.mutate(body);
  }

  return (
    <Section title="Cronjob / Worker">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Atur tugas yang dijalankan proses <code className="text-xs">drp-worker</code> untuk tenant ini.
        Worker mengecek jadwal setiap ~1 menit; tugas terjadwal (reconcile / laporan) tetap idempoten lewat{" "}
        <code className="text-xs">job_runs</code>.
      </p>

      {q.isLoading || !form ? (
        <p className="text-sm text-[var(--muted)]">Memuat pengaturan…</p>
      ) : (
        <form className="space-y-6" onSubmit={onSubmit}>
          <div className="panel-card space-y-3 p-4">
            <h3 className="text-sm font-semibold">Tugas aktif</h3>
            <label className="flex items-start gap-2 text-sm">
              <Checkbox
                checked={form.billing_enabled}
                onCheckedChange={(v) => patch("billing_enabled", v === true)}
              />
              <span>
                <strong>Generate tagihan</strong>
                <span className="mt-0.5 block text-xs text-[var(--muted)]">
                  {catalog.find((c) => c.id === "billing")?.description}
                </span>
              </span>
            </label>
            <label className="flex items-start gap-2 text-sm">
              <Checkbox checked={form.isolir_enabled} onCheckedChange={(v) => patch("isolir_enabled", v === true)} />
              <span>
                <strong>Auto isolir</strong>
                <span className="mt-0.5 block text-xs text-[var(--muted)]">
                  {catalog.find((c) => c.id === "isolir")?.description}
                </span>
              </span>
            </label>
            <label className="flex items-start gap-2 text-sm">
              <Checkbox
                checked={form.dunning_enabled}
                onCheckedChange={(v) => patch("dunning_enabled", v === true)}
              />
              <span>
                <strong>Pengingat tagihan (dunning)</strong>
                <span className="mt-0.5 block text-xs text-[var(--muted)]">
                  {catalog.find((c) => c.id === "dunning")?.description}
                </span>
              </span>
            </label>
            <div className="ml-6 max-w-md">
              <Label className="mb-1.5 block">Offset hari (negatif = sebelum jatuh tempo)</Label>
              <Input
                value={offsetsText}
                onChange={(e) => setOffsetsText(e.target.value)}
                placeholder="-7, -3, 0, 1, 3"
                disabled={!form.dunning_enabled}
              />
              <p className="mt-1 text-xs text-[var(--muted)]">Contoh: -7, -3, 0, 1, 3 → H-7, H-3, H0, H+1, H+3</p>
            </div>
            <label className="flex items-start gap-2 text-sm">
              <Checkbox
                checked={form.odp_outage_enabled}
                onCheckedChange={(v) => patch("odp_outage_enabled", v === true)}
              />
              <span>
                <strong>Deteksi gangguan ODP</strong>
                <span className="mt-0.5 block text-xs text-[var(--muted)]">
                  {catalog.find((c) => c.id === "odp_outage")?.description}
                </span>
              </span>
            </label>
          </div>

          <div className="panel-card grid gap-4 p-4 sm:grid-cols-2">
            <div className="sm:col-span-2">
              <label className="flex items-start gap-2 text-sm">
                <Checkbox
                  checked={form.weekly_reconcile_enabled}
                  onCheckedChange={(v) => patch("weekly_reconcile_enabled", v === true)}
                />
                <span>
                  <strong>Reconcile mingguan</strong>
                  <span className="mt-0.5 block text-xs text-[var(--muted)]">
                    {catalog.find((c) => c.id === "weekly_reconcile")?.description}
                  </span>
                </span>
              </label>
            </div>
            <div>
              <Label className="mb-1.5 block">Hari</Label>
              <Select
                value={String(form.weekly_reconcile_weekday)}
                onValueChange={(v) => patch("weekly_reconcile_weekday", Number(v))}
                disabled={!form.weekly_reconcile_enabled}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {WEEKDAYS.map((d) => (
                    <SelectItem key={d.value} value={String(d.value)}>
                      {d.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label className="mb-1.5 block">Jam (0–23, waktu lokal server)</Label>
              <Input
                type="number"
                min={0}
                max={23}
                value={form.weekly_reconcile_hour}
                disabled={!form.weekly_reconcile_enabled}
                onChange={(e) => patch("weekly_reconcile_hour", Number(e.target.value))}
              />
            </div>
          </div>

          <div className="panel-card grid gap-4 p-4 sm:grid-cols-2">
            <div className="sm:col-span-2">
              <label className="flex items-start gap-2 text-sm">
                <Checkbox
                  checked={form.monthly_report_enabled}
                  onCheckedChange={(v) => patch("monthly_report_enabled", v === true)}
                />
                <span>
                  <strong>Laporan bulanan (email)</strong>
                  <span className="mt-0.5 block text-xs text-[var(--muted)]">
                    {catalog.find((c) => c.id === "monthly_report")?.description}
                  </span>
                </span>
              </label>
            </div>
            <div>
              <Label className="mb-1.5 block">Tanggal (1–28)</Label>
              <Input
                type="number"
                min={1}
                max={28}
                value={form.monthly_report_day}
                disabled={!form.monthly_report_enabled}
                onChange={(e) => patch("monthly_report_day", Number(e.target.value))}
              />
            </div>
            <div>
              <Label className="mb-1.5 block">Jam (0–23)</Label>
              <Input
                type="number"
                min={0}
                max={23}
                value={form.monthly_report_hour}
                disabled={!form.monthly_report_enabled}
                onChange={(e) => patch("monthly_report_hour", Number(e.target.value))}
              />
            </div>
          </div>

          <div className="panel-card max-w-xs p-4">
            <Label className="mb-1.5 block">Batch notifikasi keluar</Label>
            <Input
              type="number"
              min={1}
              max={500}
              value={form.notify_batch_size}
              onChange={(e) => patch("notify_batch_size", Number(e.target.value))}
            />
            <p className="mt-1 text-xs text-[var(--muted)]">Maks pesan antrian diproses per siklus worker.</p>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                if (!q.data?.defaults) return;
                setForm({ ...q.data.defaults });
                setOffsetsText(offsetsToText(q.data.defaults.dunning_offsets));
              }}
            >
              Reset default
            </Button>
          </div>
        </form>
      )}

      <h3 className="mb-2 mt-8 text-sm font-semibold">Riwayat job (terbaru)</h3>
      <p className="mb-3 text-xs text-[var(--muted)]">
        Entri idempotensi (dunning, reconcile, laporan). Bukan log detail setiap tick billing.
      </p>
      <Table
        columns={["Waktu", "Job", "Key", "Status"]}
        rows={runs.map((r) => [formatWhen(r.created_at), r.job_name, r.job_key, r.status])}
      />
    </Section>
  );
}
