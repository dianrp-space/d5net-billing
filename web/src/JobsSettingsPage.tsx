import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { useConfirm } from "./confirm";
import { IconRefresh } from "./icons";
import { toastError, toastSuccess } from "./swal";
import { Button, Input, Label, Section, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Table } from "./ui";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";

type JobSchedule = {
  billing_enabled: boolean;
  isolir_enabled: boolean;
  dunning_enabled: boolean;
  dunning_offsets: number[];
  weekly_reconcile_enabled: boolean;
  weekly_reconcile_weekday: number;
  weekly_reconcile_hour: number;
  monthly_report_enabled: boolean;
  monthly_report_day: number;
  monthly_report_hour: number;
  notify_batch_size: number;
  cycle_interval_seconds: number;
  poller_interval_seconds: number;
};

type JobsResponse = {
  schedule: JobSchedule;
  defaults: JobSchedule;
  catalog: { id: string; label: string; description: string }[];
  isolir_grace_days?: number;
};

type JobsSavePayload = JobSchedule & { isolir_grace_days: number };

type JobRun = {
  id: string;
  job_name: string;
  job_key: string;
  status: string;
  created_at: string;
};

type JobRunResult = {
  invoices: number;
  isolir: number;
  late_fees: number;
  notify: number;
  routers_polled?: number;
  routers_failed?: number;
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

const INTERVAL_MINUTES = [1, 2, 5, 10, 15, 30, 60];
const HOURS = Array.from({ length: 24 }, (_, h) => h);
const MONTH_DAYS = Array.from({ length: 28 }, (_, i) => i + 1);

function formatWhen(iso?: string) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("id-ID");
  } catch {
    return iso;
  }
}

function secondsToMinutes(sec: number | undefined) {
  const n = Number(sec);
  if (!Number.isFinite(n) || n < 60) return 1;
  return Math.max(1, Math.min(60, Math.round(n / 60)));
}

function clampIsolirGrace(n: number | undefined) {
  const v = Math.floor(Number(n) || 0);
  if (!Number.isFinite(v) || v < 0) return 0;
  return Math.min(30, v);
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

function catalogHint(catalog: JobsResponse["catalog"], id: string, fallback: string) {
  return catalog.find((c) => c.id === id)?.description || fallback;
}

function JobRow({
  checked,
  onCheckedChange,
  title,
  hint,
  children,
}: {
  checked: boolean;
  onCheckedChange: (v: boolean) => void;
  title: string;
  hint: string;
  children?: ReactNode;
}) {
  return (
    <div className="rounded-[var(--radius-md,0.65rem)] border border-[var(--border)] bg-[var(--panel)] p-3">
      <label className="flex items-start gap-3 text-sm">
        <Checkbox className="mt-0.5" checked={checked} onCheckedChange={(v) => onCheckedChange(v === true)} />
        <span className="min-w-0 flex-1">
          <span className="block font-semibold">{title}</span>
          <span className="mt-0.5 block text-xs leading-relaxed text-[var(--muted)]">{hint}</span>
        </span>
      </label>
      {children ? <div className="mt-3 pl-8">{children}</div> : null}
    </div>
  );
}

export function JobsSettingsPage() {
  const qc = useQueryClient();
  const confirm = useConfirm();
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
  const [isolirGraceDays, setIsolirGraceDays] = useState(0);
  // Cakupan run manual ("Jalankan sekarang").
  const [runPollRouters, setRunPollRouters] = useState(true);
  const [runForceScheduled, setRunForceScheduled] = useState(false);

  useEffect(() => {
    if (!q.data?.schedule) return;
    setForm({ ...q.data.schedule });
    setOffsetsText(offsetsToText(q.data.schedule.dunning_offsets));
    setIsolirGraceDays(clampIsolirGrace(q.data.isolir_grace_days));
  }, [q.data]);

  const save = useMutation({
    mutationFn: (body: JobsSavePayload) =>
      api<JobsSavePayload>("/api/settings/jobs", { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: (saved) => {
      const { isolir_grace_days, ...sched } = saved;
      setForm(sched);
      setOffsetsText(offsetsToText(sched.dunning_offsets));
      setIsolirGraceDays(clampIsolirGrace(isolir_grace_days));
      void qc.invalidateQueries({ queryKey: ["jobs-settings"] });
      void qc.invalidateQueries({ queryKey: ["jobs-runs"] });
      void qc.invalidateQueries({ queryKey: ["settings-branding"] });
      void toastSuccess("Jadwal cronjob disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const runNow = useMutation({
    mutationFn: (body: { poll_routers: boolean; force_scheduled: boolean }) =>
      api<JobRunResult>("/api/settings/jobs/run", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: (res) => {
      void qc.invalidateQueries({ queryKey: ["jobs-runs"] });
      const parts = [
        `Tagihan baru ${res.invoices}`,
        `Isolir ${res.isolir}`,
        `Denda ${res.late_fees}`,
        `Notifikasi ${res.notify}`,
      ];
      if (res.routers_polled || res.routers_failed) {
        parts.push(`Router ${res.routers_polled ?? 0} ok${res.routers_failed ? `, ${res.routers_failed} gagal` : ""}`);
      }
      void toastSuccess(`Siklus selesai. ${parts.join(" · ")}`);
    },
    onError: (e: Error) => void toastError(e.message || "Gagal menjalankan worker"),
  });

  const catalog = q.data?.catalog ?? [];
  const runs = runsQ.data?.data ?? [];
  const intervalMins = form ? secondsToMinutes(form.cycle_interval_seconds) : 1;
  const pollerMins = form ? secondsToMinutes(form.poller_interval_seconds || 300) : 5;
  const intervalOptions = INTERVAL_MINUTES.includes(intervalMins)
    ? INTERVAL_MINUTES
    : [...INTERVAL_MINUTES, intervalMins].sort((a, b) => a - b);
  const pollerOptions = INTERVAL_MINUTES.includes(pollerMins)
    ? INTERVAL_MINUTES
    : [...INTERVAL_MINUTES, pollerMins].sort((a, b) => a - b);

  function patch<K extends keyof JobSchedule>(key: K, value: JobSchedule[K]) {
    setForm((f) => (f ? { ...f, [key]: value } : f));
  }

  function scheduleBody(src: JobSchedule): JobSchedule {
    return {
      ...src,
      dunning_offsets: parseOffsets(offsetsText),
      weekly_reconcile_weekday: Number(src.weekly_reconcile_weekday),
      weekly_reconcile_hour: Number(src.weekly_reconcile_hour),
      monthly_report_day: Number(src.monthly_report_day),
      monthly_report_hour: Number(src.monthly_report_hour),
      notify_batch_size: Number(src.notify_batch_size),
      cycle_interval_seconds: secondsToMinutes(src.cycle_interval_seconds) * 60,
      poller_interval_seconds: secondsToMinutes(src.poller_interval_seconds || 300) * 60,
    };
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!form) return;
    save.mutate({
      ...scheduleBody(form),
      isolir_grace_days: clampIsolirGrace(isolirGraceDays),
    });
  }

  async function onRunNow() {
    const extras: string[] = [];
    if (runPollRouters) extras.push("sampling router (sesi/metrik/traffic)");
    if (runForceScheduled) extras.push("paksa reconcile + laporan walau di luar jadwal");
    const ok = await confirm({
      title: "Jalankan worker sekarang?",
      description:
        "Tagihan jatuh tempo, auto isolir, pengingat, dan antrian notifikasi akan diproses segera — tidak menunggu interval." +
        (extras.length ? ` Termasuk: ${extras.join("; ")}.` : ""),
      confirmLabel: "Jalankan",
    });
    if (!ok) return;
    runNow.mutate({ poll_routers: runPollRouters, force_scheduled: runForceScheduled });
  }

  function resetDefaults() {
    if (!q.data?.defaults) return;
    setForm({ ...q.data.defaults });
    setOffsetsText(offsetsToText(q.data.defaults.dunning_offsets));
    setIsolirGraceDays(clampIsolirGrace(q.data.isolir_grace_days));
  }

  return (
    <Section
      title="Cronjob"
      actions={
        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" variant="outline" onClick={onRunNow} disabled={runNow.isPending || !form}>
            <IconRefresh />
            {runNow.isPending ? "Menjalankan…" : "Jalankan sekarang"}
          </Button>
          <Button type="button" variant="ghost" onClick={resetDefaults} disabled={!form}>
            Reset
          </Button>
          <Button type="submit" form="jobs-form" disabled={save.isPending || !form}>
            {save.isPending ? "Menyimpan…" : "Simpan"}
          </Button>
        </div>
      }
    >
      <p className="mb-3 max-w-2xl text-sm text-[var(--muted)]">
        Atur interval worker (tagihan/isolir) dan poller router (login API MikroTik untuk metrik). Reconcile dan
        laporan tetap sekali per jadwal (idempoten) kecuali opsi paksa di bawah dicentang.
      </p>
      <div className="mb-5 flex max-w-2xl flex-wrap gap-x-6 gap-y-2 rounded-[var(--radius-md,0.65rem)] border border-[var(--border)] bg-[var(--panel)] p-3">
        <span className="w-full text-xs font-semibold tracking-wide text-[var(--muted)]">CAKUPAN RUN MANUAL</span>
        <label className="flex cursor-pointer items-start gap-2 text-sm">
          <Checkbox
            className="mt-0.5"
            checked={runPollRouters}
            onCheckedChange={(v) => setRunPollRouters(v === true)}
          />
          <span>
            <span className="block font-medium">Sertakan sampling router</span>
            <span className="block text-xs text-[var(--muted)]">Sesi, CPU/memori, dan traffic ikut dibaca sekarang.</span>
          </span>
        </label>
        <label className="flex cursor-pointer items-start gap-2 text-sm">
          <Checkbox
            className="mt-0.5"
            checked={runForceScheduled}
            onCheckedChange={(v) => setRunForceScheduled(v === true)}
          />
          <span>
            <span className="block font-medium">Paksa reconcile + laporan</span>
            <span className="block text-xs text-[var(--muted)]">
              Abaikan jadwal hari/jam (tetap maks. 1x sehari / 1x sebulan).
            </span>
          </span>
        </label>
      </div>

      {q.isLoading || !form ? (
        <p className="text-sm text-[var(--muted)]">Memuat pengaturan…</p>
      ) : (
        <form id="jobs-form" className="grid gap-5" onSubmit={onSubmit}>
          <div className="panel-card p-4">
            <div className="mb-3">
              <h3 className="text-sm font-semibold">Interval &amp; antrian</h3>
              <p className="mt-0.5 text-xs text-[var(--muted)]">
                Seberapa sering worker dan poller router berjalan, serta ukuran batch notifikasi.
              </p>
            </div>
            <div className="grid gap-4 sm:grid-cols-3">
              <div>
                <Label className="mb-1.5 block">Interval worker</Label>
                <Select
                  value={String(intervalMins)}
                  onValueChange={(v) => patch("cycle_interval_seconds", Number(v) * 60)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {intervalOptions.map((m) => (
                      <SelectItem key={m} value={String(m)}>
                        Setiap {m} menit
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="mt-1 text-xs text-[var(--muted)]">Tagihan, isolir, pengingat.</p>
              </div>
              <div>
                <Label className="mb-1.5 block">Interval poller router</Label>
                <Select
                  value={String(pollerMins)}
                  onValueChange={(v) => patch("poller_interval_seconds", Number(v) * 60)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {pollerOptions.map((m) => (
                      <SelectItem key={m} value={String(m)}>
                        Setiap {m} menit
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="mt-1 text-xs text-[var(--muted)]">Login API MikroTik (sesi &amp; CPU). Default 5 menit.</p>
              </div>
              <div>
                <Label className="mb-1.5 block">Batch notifikasi</Label>
                <Input
                  type="number"
                  min={1}
                  max={500}
                  value={form.notify_batch_size}
                  onChange={(e) => patch("notify_batch_size", Number(e.target.value))}
                />
                <p className="mt-1 text-xs text-[var(--muted)]">Jumlah pesan per pemrosesan antrian.</p>
              </div>
            </div>
          </div>

          <div className="grid gap-5 lg:grid-cols-2 lg:items-start">
            <div className="panel-card space-y-3 p-4">
              <div>
                <h3 className="text-sm font-semibold">Setiap siklus</h3>
                <p className="mt-0.5 text-xs text-[var(--muted)]">
                  Jalan otomatis sesuai interval, atau lewat tombol di atas.
                </p>
              </div>
              <JobRow
                checked={form.billing_enabled}
                onCheckedChange={(v) => patch("billing_enabled", v)}
                title="Generate tagihan"
                hint={catalogHint(catalog, "billing", "Invoice untuk langganan yang jatuh tempo.")}
              />
              <JobRow
                checked={form.isolir_enabled}
                onCheckedChange={(v) => patch("isolir_enabled", v)}
                title="Auto isolir"
                hint={catalogHint(
                  catalog,
                  "isolir",
                  "Suspend langganan lewat jatuh tempo + masa tenggang isolir, lalu retry resume.",
                )}
              >
                <Label className="mb-1.5 block">Masa tenggang isolir (hari)</Label>
                <Input
                  type="number"
                  min={0}
                  max={30}
                  step={1}
                  value={isolirGraceDays}
                  onChange={(e) => setIsolirGraceDays(clampIsolirGrace(Number(e.target.value)))}
                />
                <p className="mt-1 text-xs text-[var(--muted)]">
                  Hari setelah jatuh tempo invoice. 0 = isolir pada tanggal jatuh tempo. Maks. 30. Sama dengan
                  Pengaturan → Umum.
                </p>
              </JobRow>
              <JobRow
                checked={form.dunning_enabled}
                onCheckedChange={(v) => patch("dunning_enabled", v)}
                title="Pengingat tagihan"
                hint={catalogHint(catalog, "dunning", "Reminder WhatsApp/email pada offset hari relatif jatuh tempo.")}
              >
                <Label className="mb-1.5 block">Offset hari</Label>
                <Input
                  value={offsetsText}
                  onChange={(e) => setOffsetsText(e.target.value)}
                  placeholder="-7, -3, 0, 1, 3"
                  disabled={!form.dunning_enabled}
                />
                <p className="mt-1 text-xs text-[var(--muted)]">Negatif = sebelum jatuh tempo. Contoh: -7, -3, 0, 1, 3</p>
              </JobRow>
            </div>

            <div className="panel-card space-y-3 p-4">
              <div>
                <h3 className="text-sm font-semibold">Jadwal</h3>
                <p className="mt-0.5 text-xs text-[var(--muted)]">Sekali per slot waktu (tidak diulang tiap siklus).</p>
              </div>
              <JobRow
                checked={form.weekly_reconcile_enabled}
                onCheckedChange={(v) => patch("weekly_reconcile_enabled", v)}
                title="Reconcile mingguan"
                hint={catalogHint(catalog, "weekly_reconcile", "Dry-run drift RouterOS vs billing.")}
              >
                <div className="grid gap-3 sm:grid-cols-2">
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
                    <Label className="mb-1.5 block">Jam</Label>
                    <Select
                      value={String(form.weekly_reconcile_hour)}
                      onValueChange={(v) => patch("weekly_reconcile_hour", Number(v))}
                      disabled={!form.weekly_reconcile_enabled}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {HOURS.map((h) => (
                          <SelectItem key={h} value={String(h)}>
                            {String(h).padStart(2, "0")}:00
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </JobRow>
              <JobRow
                checked={form.monthly_report_enabled}
                onCheckedChange={(v) => patch("monthly_report_enabled", v)}
                title="Laporan bulanan"
                hint={catalogHint(catalog, "monthly_report", "Email ringkas statistik bisnis.")}
              >
                <div className="grid gap-3 sm:grid-cols-2">
                  <div>
                    <Label className="mb-1.5 block">Tanggal</Label>
                    <Select
                      value={String(form.monthly_report_day)}
                      onValueChange={(v) => patch("monthly_report_day", Number(v))}
                      disabled={!form.monthly_report_enabled}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {MONTH_DAYS.map((d) => (
                          <SelectItem key={d} value={String(d)}>
                            {d}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div>
                    <Label className="mb-1.5 block">Jam</Label>
                    <Select
                      value={String(form.monthly_report_hour)}
                      onValueChange={(v) => patch("monthly_report_hour", Number(v))}
                      disabled={!form.monthly_report_enabled}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {HOURS.map((h) => (
                          <SelectItem key={h} value={String(h)}>
                            {String(h).padStart(2, "0")}:00
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </JobRow>
            </div>
          </div>
        </form>
      )}

      <div className="panel-card mt-5 p-4">
        <h3 className="text-sm font-semibold">Riwayat</h3>
        <p className="mb-3 mt-0.5 text-xs text-[var(--muted)]">
          Idempotensi dunning, reconcile, dan laporan — bukan setiap tick billing/isolir.
        </p>
        <Table
          columns={["Waktu", "Job", "Key", "Status"]}
          rows={runs.map((r) => [
            formatWhen(r.created_at),
            r.job_name,
            r.job_key,
            <Badge
              key={r.id}
              variant={r.status === "done" ? "success" : r.status === "failed" ? "danger" : "outline"}
            >
              {r.status}
            </Badge>,
          ])}
        />
      </div>
    </Section>
  );
}
