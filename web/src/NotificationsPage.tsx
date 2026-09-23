import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { formatDateTimeShort } from "./tenantTime";
import { useConfirm } from "./confirm";
import { IconRefresh, IconTrash } from "./icons";
import { toastError, toastSuccess } from "./swal";
import { Button, IconButton, Input, Section, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Table } from "./ui";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ListToolbar, useDebouncedValue } from "./ListToolbar";
import { usePersistedTab } from "./navPersist";

type NotifTemplate = {
  id: string;
  channel: string;
  event: string;
  subject?: string | null;
  body: string;
};

type TemplateEvent = {
  event: string;
  label: string;
  description: string;
  channels: string[];
  variables: { name: string; desc: string }[];
  default_subject: string;
  default_body: string;
};

const PREVIEW_SAMPLES: Record<string, string> = {
  customer_name: "Budi Santoso",
  phone: "0812-3456-7890",
  plan_name: "Home 20 Mbps",
  item_name: "Tes 3",
  invoice_number: "INV-D5N-2026090001-092026A3F9K2",
  amount: "150000",
  due_date: "10/09/2026",
  message: "(isi pesan broadcast)",
};

function renderPreview(body: string): string {
  return body.replace(/\{\{(\w+)\}\}/g, (_, key: string) => PREVIEW_SAMPLES[key] ?? `{{${key}}}`);
}

/** Variabel yang benar-benar diisi saat broadcast dikirim (per penerima). */
const SUPPORTED_BCAST_VARS = new Set([
  "customer_name",
  "phone",
  "plan_name",
  "item_name",
  "invoice_number",
  "amount",
  "due_date",
]);

/** Daftar placeholder {{nama}} unik persis seperti yang dikenali backend. */
function findPlaceholders(s: string): string[] {
  const out: string[] = [];
  const re = /\{\{(\w+)\}\}/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(s)) !== null) {
    if (!out.includes(m[1])) out.push(m[1]);
  }
  return out;
}

export function NotificationsPage() {
  const qc = useQueryClient();
  const [tab, setTab] = usePersistedTab("notifications", "broadcast", ["broadcast", "templates", "history"] as const);
  const templatesQ = useQuery({
    queryKey: ["notification-templates"],
    queryFn: () => api<NotifTemplate[]>("/api/notifications/templates"),
  });

  const catalogQ = useQuery({
    queryKey: ["notification-template-events"],
    queryFn: () => api<TemplateEvent[]>("/api/notifications/templates/catalog"),
  });

  const [tpl, setTpl] = useState({ channel: "whatsapp", event: "invoice_reminder", subject: "", body: "" });
  const [bcast, setBcast] = useState({
    channel: "whatsapp",
    audience: "overdue",
    cluster_id: "",
    odp_id: "",
    subject: "",
    template_event: "",
    body: "Halo {{customer_name}}, ini pengingat tagihan dari kami. Silakan bayar via portal pelanggan.",
    delay_seconds: 3,
    recipients: "",
  });

  type ClusterOpt = { id: string; name: string; code: string; is_active: boolean };
  type OdpOpt = { id: string; name: string; code: string; cluster_id?: string | null };
  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterOpt[]>("/api/clusters"),
    retry: false,
  });
  const odpsQ = useQuery({
    queryKey: ["odps"],
    queryFn: () => api<OdpOpt[]>("/api/odps"),
    retry: false,
  });
  const clusters = (Array.isArray(clustersQ.data) ? clustersQ.data : []).filter((c) => c.is_active);
  const odps = Array.isArray(odpsQ.data) ? odpsQ.data : [];
  const odpOptions = bcast.cluster_id ? odps.filter((o) => o.cluster_id === bcast.cluster_id) : odps;

  const previewQ = useQuery({
    queryKey: ["broadcast-preview", bcast.audience, bcast.cluster_id, bcast.odp_id],
    queryFn: () => {
      const params = new URLSearchParams({ audience: bcast.audience });
      if (bcast.cluster_id) params.set("cluster_id", bcast.cluster_id);
      if (bcast.odp_id) params.set("odp_id", bcast.odp_id);
      return api<{ audience: string; count: number }>(`/api/notifications/broadcast/preview?${params}`);
    },
    enabled: bcast.audience !== "custom",
    retry: false,
    staleTime: 10_000,
  });
  const previewCount = previewQ.data?.count;

  const saveTpl = useMutation({
    mutationFn: () =>
      api("/api/notifications/templates", {
        method: "PUT",
        body: JSON.stringify({
          channel: tpl.channel,
          event: tpl.event.trim(),
          subject: tpl.subject.trim() || null,
          body: tpl.body,
        }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["notification-templates"] });
      void toastSuccess("Template disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const delTpl = useMutation({
    mutationFn: (id: string) => api(`/api/notifications/templates/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["notification-templates"] });
      void toastSuccess("Template dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const sendBcast = useMutation({
    mutationFn: () => {
      const body: Record<string, unknown> = {
        channel: bcast.channel,
        audience: bcast.audience,
        body: bcast.body,
        delay_seconds: bcast.delay_seconds,
      };
      if (bcast.subject.trim()) body.subject = bcast.subject.trim();
      if (bcast.template_event) body.template_event = bcast.template_event;
      if (bcast.audience === "custom") {
        body.recipients = bcast.recipients
          .split(/[\n,;]+/)
          .map((s) => s.trim())
          .filter(Boolean);
      } else {
        if (bcast.cluster_id) body.cluster_id = bcast.cluster_id;
        if (bcast.odp_id) body.odp_id = bcast.odp_id;
      }
      return api<{ queued: number; batch_id: string }>("/api/notifications/broadcast", {
        method: "POST",
        body: JSON.stringify(body),
      });
    },
    onSuccess: (r) => {
      setBatchId(r.batch_id);
      void toastSuccess(`${r.queued} pesan diantrekan — memantau pengiriman…`);
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const [batchId, setBatchId] = useState("");
  type BroadcastProgress = {
    total: number;
    pending: number;
    sent: number;
    failed: number;
    failures: { recipient: string; error: string }[];
  };
  const progressQ = useQuery({
    queryKey: ["broadcast-progress", batchId],
    queryFn: () => api<BroadcastProgress>(`/api/notifications/broadcast/${batchId}`),
    enabled: Boolean(batchId),
    refetchInterval: (query) => {
      const d = query.state.data as BroadcastProgress | undefined;
      return d && d.pending === 0 ? false : 2000;
    },
  });
  const progress = progressQ.data;
  const progressDone = progress ? progress.pending === 0 : false;
  const progressPct = progress && progress.total > 0 ? Math.round(((progress.sent + progress.failed) / progress.total) * 100) : 0;

  const catalog = catalogQ.data ?? [];
  const templates = templatesQ.data ?? [];
  const selectedEvent = catalog.find((e) => e.event === tpl.event);
  const broadcastEvent = catalog.find((e) => e.event === "broadcast");
  const bcastVars =
    broadcastEvent && broadcastEvent.variables.length
      ? broadcastEvent.variables
      : [
          { name: "customer_name", desc: "Nama pelanggan" },
          { name: "phone", desc: "Nomor penerima" },
          { name: "plan_name", desc: "Paket langganan (terbaru)" },
          { name: "item_name", desc: "Item tagihan acuan" },
          { name: "invoice_number", desc: "Nomor tagihan acuan" },
          { name: "amount", desc: "Sisa tagihan acuan (angka)" },
          { name: "due_date", desc: "Jatuh tempo tagihan acuan" },
        ];
  // Template tersimpan untuk channel yang sedang dipilih.
  const bcastChannelTemplates = templates.filter((t) => t.channel === bcast.channel);
  function bcastTemplateLabel(t: NotifTemplate): string {
    return catalog.find((e) => e.event === t.event)?.label || t.event;
  }
  function pickBcastTemplate(event: string) {
    if (!event) {
      setBcast((b) => ({ ...b, template_event: "" }));
      return;
    }
    const saved = templates.find((t) => t.channel === bcast.channel && t.event === event);
    if (!saved) return;
    setBcast((b) => ({ ...b, template_event: event, subject: saved.subject || "", body: saved.body }));
  }
  // Placeholder yang tidak akan terisi saat kirim → cegah terkirim mentah.
  const unsupportedVars = findPlaceholders(`${bcast.body}\n${bcast.subject}`).filter(
    (v) => bcast.audience === "custom" || !SUPPORTED_BCAST_VARS.has(v),
  );

  function insertBcastVar(name: string) {
    setBcast((b) => ({
      ...b,
      body: `${b.body}${b.body === "" || b.body.endsWith(" ") ? "" : " "}{{${name}}}`,
    }));
  }

  function findSaved(channel: string, event: string) {
    return templates.find((t) => t.channel === channel && t.event === event);
  }

  function loadTemplate(channel: string, event: string, forceDefault = false) {
    const saved = findSaved(channel, event);
    const def = catalog.find((e) => e.event === event);
    if (saved && !forceDefault) {
      setTpl({ channel, event, subject: saved.subject || "", body: saved.body });
    } else {
      setTpl({ channel, event, subject: def?.default_subject || "", body: def?.default_body || "" });
    }
  }

  function insertVar(name: string) {
    setTpl((t) => ({
      ...t,
      body: `${t.body}${t.body === "" || t.body.endsWith(" ") ? "" : " "}{{${name}}}`,
    }));
  }

  // Prefill the editor once the catalog loads (saved template wins, else default).
  useEffect(() => {
    if (!catalog.length) return;
    setTpl((t) => {
      if (t.body) return t;
      const saved = templates.find((x) => x.channel === t.channel && x.event === t.event);
      const def = catalog.find((e) => e.event === t.event);
      return { ...t, subject: saved?.subject || def?.default_subject || "", body: saved?.body || def?.default_body || "" };
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [catalog.length, templates.length]);

  return (
    <Section title="Notifikasi">
      <Tabs
        value={tab}
        onValueChange={(v) => {
          if (v === "broadcast" || v === "templates" || v === "history") setTab(v);
        }}
        className="space-y-0"
      >
        <TabsList aria-label="Notifikasi">
          <TabsTrigger value="broadcast">Broadcast</TabsTrigger>
          <TabsTrigger value="templates">Template</TabsTrigger>
          <TabsTrigger value="history">Riwayat</TabsTrigger>
        </TabsList>
        <TabsContent value="broadcast">
          <div className="grid max-w-xl gap-3">
          <p className="text-sm text-[var(--muted)]">
            Kirim pesan massal (dunning / promo). Delay antar penerima dipakai sebagai rate limit antrian.
          </p>
          <label className="grid gap-1 text-sm">
            <span>Channel</span>
            <Select
              value={bcast.channel}
              onValueChange={(v) => setBcast((b) => ({ ...b, channel: v, template_event: "" }))}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="whatsapp">WhatsApp</SelectItem>
                <SelectItem value="telegram">Telegram</SelectItem>
                <SelectItem value="email">Email</SelectItem>
              </SelectContent>
            </Select>
          </label>
          <label className="grid gap-1 text-sm">
            <span>Pakai template tersimpan (opsional)</span>
            <Select value={bcast.template_event || "__manual__"} onValueChange={pickBcastTemplate}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__manual__">Tulis manual</SelectItem>
                {bcastChannelTemplates.map((t) => (
                  <SelectItem key={t.id} value={t.event}>
                    {bcastTemplateLabel(t)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <span className="text-[11px] text-[var(--muted)]">
              {bcastChannelTemplates.length
                ? "Memilih template mengisi subject & isi pesan di bawah (masih bisa diubah)."
                : "Belum ada template untuk channel ini — buat di tab Template."}
            </span>
          </label>
          <label className="grid gap-1 text-sm">
            <span>Audience</span>
            <Select
              value={bcast.audience}
              onValueChange={(v) => setBcast({ ...bcast, audience: v, cluster_id: "", odp_id: "" })}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="overdue">Pelanggan tunggakan</SelectItem>
                <SelectItem value="active">Semua pelanggan aktif (punya HP)</SelectItem>
                <SelectItem value="custom">Custom (paste nomor)</SelectItem>
              </SelectContent>
            </Select>
          </label>
          {bcast.audience === "custom" ? (
            <label className="grid gap-1 text-sm">
              <span>Nomor (satu per baris)</span>
              <textarea
                className="input min-h-[100px]"
                value={bcast.recipients}
                onChange={(e) => setBcast({ ...bcast, recipients: e.target.value })}
              />
            </label>
          ) : (
            <>
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="grid gap-1 text-sm">
                  <span>Cluster / area (opsional)</span>
                  <Select
                    value={bcast.cluster_id || "__all__"}
                    onValueChange={(v) => {
                      const next = v === "__all__" ? "" : v;
                      setBcast((b) => ({
                        ...b,
                        cluster_id: next,
                        // ODP yang tidak masuk cluster baru ikut direset.
                        odp_id: next && b.odp_id && !odps.some((o) => o.id === b.odp_id && o.cluster_id === next) ? "" : b.odp_id,
                      }));
                    }}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__all__">Semua cluster</SelectItem>
                      {clusters.map((c) => (
                        <SelectItem key={c.id} value={c.id}>
                          {c.name} ({c.code})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <span className="text-[11px] text-[var(--muted)]">
                    Batasi ke pelanggan di satu POP/area — mis. area terdampak gangguan.
                  </span>
                </label>
                <label className="grid gap-1 text-sm">
                  <span>ODP (opsional)</span>
                  <Select
                    value={bcast.odp_id || "__all__"}
                    onValueChange={(v) => setBcast({ ...bcast, odp_id: v === "__all__" ? "" : v })}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__all__">Semua ODP</SelectItem>
                      {odpOptions.map((o) => (
                        <SelectItem key={o.id} value={o.id}>
                          {o.code} · {o.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <span className="text-[11px] text-[var(--muted)]">
                    Batasi ke pelanggan yang langganannya menancap di ODP ini.
                  </span>
                </label>
              </div>
              <p className="text-xs text-[var(--muted)]" aria-live="polite">
                {previewQ.isLoading
                  ? "Menghitung penerima…"
                  : previewQ.isError
                    ? "Gagal menghitung penerima. Cek koneksi lalu ubah filter untuk mencoba lagi."
                    : previewCount != null
                      ? `${previewCount} penerima akan dikirimi pesan.`
                      : null}
              </p>
            </>
          )}
          <label className="grid gap-1 text-sm">
            <span>Delay antar pesan (detik)</span>
            <Input
              type="number"
              min={1}
              max={60}
              value={bcast.delay_seconds}
              onChange={(e) => setBcast({ ...bcast, delay_seconds: Math.min(60, Math.max(1, Number(e.target.value) || 2)) })}
            />
          </label>
          <label className="grid gap-1 text-sm">
            <span>Isi pesan</span>
            <textarea
              className="input min-h-[120px]"
              value={bcast.body}
              onChange={(e) => setBcast({ ...bcast, body: e.target.value })}
              required
            />
          </label>
          {bcast.channel === "email" ? (
            <label className="grid gap-1 text-sm">
              <span>Subject (email)</span>
              <Input
                value={bcast.subject}
                onChange={(e) => setBcast({ ...bcast, subject: e.target.value })}
                placeholder="Judul email"
              />
            </label>
          ) : null}
          {unsupportedVars.length > 0 ? (
            <p className="rounded-lg border border-[var(--danger)]/40 bg-[var(--danger)]/5 p-3 text-xs leading-relaxed text-[var(--danger)]">
              Variabel {unsupportedVars.map((v) => `{{${v}}}`).join(", ")} tidak didukung broadcast
              {bcast.audience === "custom"
                ? " custom (nomor paste-an tanpa data pelanggan)"
                : " (yang terisi otomatis: customer_name, phone, plan_name, item_name, invoice_number, amount, due_date)"}
              {" "}dan akan terkirim mentah. Hapus atau ganti sebelum mengirim.
            </p>
          ) : null}
          {bcast.audience !== "custom" ? (
            <>
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-xs text-[var(--muted)]">Sisipkan variabel:</span>
                {bcastVars.map((v) => (
                  <button
                    key={v.name}
                    type="button"
                    className="btn-ghost"
                    style={{ padding: "0.2rem 0.5rem", fontSize: "0.7rem", lineHeight: 1.4 }}
                    title={v.desc}
                    onClick={() => insertBcastVar(v.name)}
                  >
                    {`{{${v.name}}}`}
                  </button>
                ))}
              </div>
              <div className="rounded-lg border border-[var(--border)] bg-[var(--panel-muted)]/40 p-3">
                <p className="mb-1 text-xs font-medium text-[var(--muted)]">Pratinjau</p>
                {bcast.channel === "email" && bcast.subject.trim() ? (
                  <p className="mb-1 text-sm font-semibold">{renderPreview(bcast.subject) || "—"}</p>
                ) : null}
                <p className="whitespace-pre-wrap text-sm">{renderPreview(bcast.body) || "—"}</p>
              </div>
            </>
          ) : (
            <p className="text-[11px] text-[var(--muted)]">
              Audience custom berisi nomor paste-an tanpa data pelanggan, jadi variabel tidak tersedia.
            </p>
          )}
          <Button
            type="button"
            onClick={() => sendBcast.mutate()}
            disabled={
              sendBcast.isPending ||
              unsupportedVars.length > 0 ||
              (bcast.audience !== "custom" && previewCount === 0)
            }
          >
            {sendBcast.isPending ? "Mengantre…" : "Kirim broadcast"}
          </Button>
          {batchId && progress ? (
            <div className="grid gap-2 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel)] p-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-sm font-semibold">
                  {progressDone ? "Pengiriman selesai" : "Mengirim…"} ({progress.sent + progress.failed}/{progress.total})
                </h3>
                <Button type="button" variant="outline" size="sm" onClick={() => setBatchId("")}>
                  Tutup
                </Button>
              </div>
              <div
                className="h-2.5 overflow-hidden rounded-full bg-[var(--panel-muted)]"
                role="progressbar"
                aria-valuenow={progressPct}
                aria-valuemin={0}
                aria-valuemax={100}
              >
                <div
                  className="h-full rounded-full bg-[var(--accent)] transition-all"
                  style={{ width: `${progressPct}%` }}
                />
              </div>
              <div className="flex flex-wrap gap-2 text-xs">
                <span className="rounded-full border border-[var(--border)] px-2 py-0.5">
                  ⏳ Menunggu: {progress.pending}
                </span>
                <span className="rounded-full border border-[var(--border)] px-2 py-0.5">
                  ✅ Terkirim: {progress.sent}
                </span>
                {progress.failed > 0 ? (
                  <span className="rounded-full border border-[var(--danger)] px-2 py-0.5 text-[var(--danger)]">
                    ❌ Gagal: {progress.failed}
                  </span>
                ) : null}
              </div>
              {!progressDone ? (
                <p className="text-xs text-[var(--muted)]">Memantau otomatis tiap 2 detik. Worker mengirim tiap ~15 detik.</p>
              ) : progress.failed === 0 ? (
                <p className="text-xs text-[var(--muted)]">Semua pesan terkirim.</p>
              ) : null}
              {progress.failures.length > 0 ? (
                <ul className="grid gap-1 text-xs">
                  {progress.failures.map((f) => (
                    <li
                      key={f.recipient}
                      className="rounded-md border border-[var(--danger)]/40 px-2 py-1.5"
                    >
                      <span className="font-semibold">{f.recipient}</span>
                      <span className="block text-[var(--muted)]">{f.error || "gagal tanpa keterangan"}</span>
                    </li>
                  ))}
                </ul>
              ) : null}
            </div>
          ) : null}
          </div>
        </TabsContent>
        <TabsContent value="templates">
          <div className="grid gap-4 lg:grid-cols-2 lg:items-start">
            <div className="grid gap-3 rounded-[var(--radius-lg)] border border-[var(--border)] p-4">
              <div>
                <h3 className="text-sm font-semibold">Editor template</h3>
                <p className="mt-0.5 text-xs text-[var(--muted)]">
                  Ubah isi notifikasi otomatis (dunning, konfirmasi pembayaran, broadcast). Pilih jenis lalu sunting
                  body-nya.
                </p>
              </div>
              <label className="grid gap-1 text-sm">
                <span>Jenis notifikasi</span>
                <Select value={tpl.event} onValueChange={(v) => loadTemplate(tpl.channel, v)}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {catalog.map((e) => (
                      <SelectItem key={e.event} value={e.event}>
                        {e.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {selectedEvent ? <span className="text-xs text-[var(--muted)]">{selectedEvent.description}</span> : null}
              </label>
              <label className="grid gap-1 text-sm">
                <span>Channel</span>
                <Select value={tpl.channel} onValueChange={(v) => loadTemplate(v, tpl.event)}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="whatsapp">WhatsApp</SelectItem>
                    <SelectItem value="telegram">Telegram</SelectItem>
                    <SelectItem value="email">Email</SelectItem>
                  </SelectContent>
                </Select>
                {selectedEvent && !selectedEvent.channels.includes(tpl.channel) ? (
                  <span className="text-xs text-[var(--warn)]">
                    Jenis ini biasanya dikirim lewat {selectedEvent.channels.join(", ")}.
                  </span>
                ) : null}
              </label>
              <label className="grid gap-1 text-sm">
                <span>Subject (email)</span>
                <Input value={tpl.subject} onChange={(e) => setTpl({ ...tpl, subject: e.target.value })} />
              </label>
              <label className="grid gap-1 text-sm">
                <span>Body</span>
                <textarea
                  className="input min-h-[120px]"
                  value={tpl.body}
                  onChange={(e) => setTpl({ ...tpl, body: e.target.value })}
                />
              </label>
              {selectedEvent?.variables.length ? (
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="text-xs text-[var(--muted)]">Sisipkan variabel:</span>
                  {selectedEvent.variables.map((v) => (
                    <button
                      key={v.name}
                      type="button"
                      className="btn-ghost"
                      style={{ padding: "0.2rem 0.5rem", fontSize: "0.7rem", lineHeight: 1.4 }}
                      title={v.desc}
                      onClick={() => insertVar(v.name)}
                    >
                      {`{{${v.name}}}`}
                    </button>
                  ))}
                </div>
              ) : null}
              <div className="rounded-lg border border-[var(--border)] bg-[var(--panel-muted)]/40 p-3">
                <p className="mb-1 text-xs font-medium text-[var(--muted)]">Pratinjau</p>
                <p className="whitespace-pre-wrap text-sm">{renderPreview(tpl.body) || "—"}</p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button type="button" onClick={() => saveTpl.mutate()} disabled={saveTpl.isPending}>
                  {saveTpl.isPending ? "Menyimpan…" : "Simpan template"}
                </Button>
                <Button type="button" variant="ghost" onClick={() => loadTemplate(tpl.channel, tpl.event, true)}>
                  Isi bawaan
                </Button>
              </div>
            </div>

            <div className="grid gap-3">
              <div>
                <h3 className="text-sm font-semibold">Template tersimpan</h3>
                <p className="mt-0.5 text-xs text-[var(--muted)]">
                  Klik body untuk memuat ke editor. Hapus untuk kembali ke bawaan.
                </p>
              </div>
              <Table
                columns={["Channel", "Jenis", "Body", ""]}
                rows={templates.map((t) => [
                  t.channel,
                  catalog.find((e) => e.event === t.event)?.label || t.event,
                  <button
                    key="b"
                    type="button"
                    className="line-clamp-2 max-w-md text-left text-xs hover:underline"
                    title="Muat ke editor"
                    onClick={() => loadTemplate(t.channel, t.event)}
                  >
                    {t.body}
                  </button>,
                  <IconButton
                    key="d"
                    label="Hapus"
                    title="Hapus"
                    aria-label="Hapus template"
                    onClick={() => delTpl.mutate(t.id)}
                  >
                    <IconTrash />
                  </IconButton>,
                ])}
              />
            </div>
          </div>
        </TabsContent>
        <TabsContent value="history">
          <NotificationHistoryTab />
        </TabsContent>
      </Tabs>
    </Section>
  );
}

type NotifLog = {
  id: string;
  channel: string;
  event: string;
  recipient: string;
  sender?: string | null;
  subject?: string | null;
  body: string;
  status: string;
  attempts: number;
  error?: string | null;
  batch_id?: string | null;
  scheduled_at: string;
  sent_at?: string | null;
  created_at: string;
};

const EVENT_LABELS: Record<string, string> = {
  broadcast: "Broadcast",
  invoice_issued: "Tagihan baru",
  invoice_reminder: "Pengingat tagihan",
  payment_confirmation: "Konfirmasi bayar",
  ops_telegram: "Alert ops",
  monthly_report: "Laporan bulanan",
};

const LOG_STATUS: Record<string, { label: string; tone: string }> = {
  pending: { label: "Menunggu", tone: "var(--warn, #b7791f)" },
  sent: { label: "Terkirim", tone: "var(--ok)" },
  failed: { label: "Gagal", tone: "var(--danger)" },
};

function formatDateTime(s?: string | null): string {
  return formatDateTimeShort(s);
}

function NotificationHistoryTab() {
  const qc = useQueryClient();
  const confirm = useConfirm();
  const [status, setStatus] = useState("");
  const [channel, setChannel] = useState("");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, 300);
  const [page, setPage] = useState(0);
  const [limit, setLimit] = useState(20);
  function setPageSize(n: number) {
    setLimit(n);
    setPage(0);
  }

  const q = useQuery({
    queryKey: ["notification-history", status, channel, debouncedSearch, page, limit],
    queryFn: () => {
      const params = new URLSearchParams({ limit: String(limit), offset: String(page * limit) });
      if (status) params.set("status", status);
      if (channel) params.set("channel", channel);
      if (debouncedSearch.trim()) params.set("search", debouncedSearch.trim());
      return api<{ data: NotifLog[]; total: number; pending: number; sent: number; failed: number }>(
        `/api/notifications/history?${params}`,
      );
    },
    refetchInterval: 10000,
  });

  const rows = q.data?.data ?? [];
  const total = q.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / limit));

  function clampRetentionDays(n: unknown): number {
    const v = Math.floor(Number(n));
    if (!Number.isFinite(v) || v < 0) return 0;
    return Math.min(365, v);
  }

  const retentionQ = useQuery({
    queryKey: ["notification-retention"],
    queryFn: () => api<{ retention_days: number }>("/api/notifications/retention"),
  });
  const [retentionDays, setRetentionDays] = useState(0);
  useEffect(() => {
    if (retentionQ.data) setRetentionDays(clampRetentionDays(retentionQ.data.retention_days));
  }, [retentionQ.data]);

  const saveRetention = useMutation({
    mutationFn: (days: number) =>
      api<{ retention_days: number }>("/api/notifications/retention", {
        method: "PUT",
        body: JSON.stringify({ retention_days: days }),
      }),
    onSuccess: (r) => {
      setRetentionDays(clampRetentionDays(r.retention_days));
      void qc.invalidateQueries({ queryKey: ["notification-retention"] });
      void toastSuccess(
        clampRetentionDays(r.retention_days) === 0
          ? "Retensi otomatis dimatikan"
          : `Log otomatis dihapus setelah ${clampRetentionDays(r.retention_days)} hari`,
      );
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const purgeNow = useMutation({
    mutationFn: (days: number) =>
      api<{ deleted: number; retention_days: number }>("/api/notifications/history/purge", {
        method: "POST",
        body: JSON.stringify({ retention_days: days }),
      }),
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: ["notification-history"] });
      void toastSuccess(`${r.deleted} log lebih dari ${r.retention_days} hari dihapus`);
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const resend = useMutation({
    mutationFn: (id: string) => api<{ status: string }>(`/api/notifications/history/${id}/resend`, { method: "POST" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["notification-history"] });
      void toastSuccess("Dimasukkan antrean, worker mengirim ±15 detik lagi");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  async function onResend(n: NotifLog) {
    const ok = await confirm({
      title: "Kirim ulang pesan?",
      description: `Kirim ulang ke ${n.recipient || "penerima"}? Isi pesan sama seperti sebelumnya.`,
      confirmLabel: "Kirim ulang",
    });
    if (!ok) return;
    resend.mutate(n.id);
  }

  async function onPurgeNow() {
    const days = clampRetentionDays(retentionDays);
    if (days < 1) {
      void toastError("Isi retensi minimal 1 hari untuk hapus manual.");
      return;
    }
    const ok = await confirm({
      title: `Hapus log Terkirim/Gagal lebih dari ${days} hari?`,
      description: "Log Menunggu tidak ikut dihapus. Aksi ini permanen dan tercatat di audit.",
      confirmLabel: "Hapus",
      danger: true,
    });
    if (!ok) return;
    purgeNow.mutate(days);
  }

  return (
    <div className="grid gap-3">
      <p className="text-sm text-[var(--muted)]">
        Riwayat semua pengiriman (dunning, konfirmasi bayar, broadcast, alert ops, laporan). Pakai kolom Keterangan
        untuk menelusuri kiriman yang gagal atau ganda.
      </p>
      <div className="grid gap-2 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel)] p-4">
        <h3 className="text-sm font-semibold">Retensi log otomatis</h3>
        <p className="text-xs text-[var(--muted)]">
          Worker menghapus otomatis tiap hari untuk log Terkirim/Gagal yang lebih tua dari retensi. Log Menunggu
          tidak pernah dihapus. Isi 0 untuk menonaktifkan hapus otomatis.
          {retentionQ.data ? (
            <>
              {" "}Status:{" "}
              <strong>
                {clampRetentionDays(retentionQ.data.retention_days) === 0
                  ? "nonaktif"
                  : `hapus log > ${clampRetentionDays(retentionQ.data.retention_days)} hari`}
              </strong>
            </>
          ) : null}
        </p>
        <div className="flex flex-wrap items-end gap-2">
          <label className="grid gap-1 text-sm">
            <span>Simpan log (hari)</span>
            <Input
              type="number"
              min={0}
              max={365}
              className="w-28"
              value={retentionDays}
              onChange={(e) => setRetentionDays(clampRetentionDays(e.target.value))}
            />
          </label>
          <Button
            type="button"
            onClick={() => saveRetention.mutate(clampRetentionDays(retentionDays))}
            disabled={saveRetention.isPending}
          >
            {saveRetention.isPending ? "Menyimpan…" : "Simpan retensi"}
          </Button>
          <Button type="button" variant="outline" onClick={() => void onPurgeNow()} disabled={purgeNow.isPending}>
            {purgeNow.isPending ? "Menghapus…" : "Hapus sekarang"}
          </Button>
        </div>
      </div>
      <div className="flex flex-wrap gap-2 text-xs">
        <span className="rounded-full border border-[var(--border)] px-2.5 py-1">
          ⏳ Menunggu: <strong>{q.data?.pending ?? 0}</strong>
        </span>
        <span className="rounded-full border border-[var(--border)] px-2.5 py-1">
          ✅ Terkirim: <strong>{q.data?.sent ?? 0}</strong>
        </span>
        <span
          className="rounded-full border px-2.5 py-1"
          style={{
            borderColor: (q.data?.failed ?? 0) > 0 ? "var(--danger)" : "var(--border)",
            color: (q.data?.failed ?? 0) > 0 ? "var(--danger)" : undefined,
          }}
        >
          ❌ Gagal: <strong>{q.data?.failed ?? 0}</strong>
        </span>
      </div>
      <ListToolbar
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(0);
        }}
        searchPlaceholder="Nomor, isi pesan…"
        filters={[
          {
            key: "channel",
            label: "Channel",
            value: channel,
            onChange: (v) => {
              setChannel(v);
              setPage(0);
            },
            options: [
              { value: "whatsapp", label: "WhatsApp" },
              { value: "telegram", label: "Telegram" },
              { value: "email", label: "Email" },
            ],
          },
          {
            key: "status",
            label: "Status",
            value: status,
            onChange: (v) => {
              setStatus(v);
              setPage(0);
            },
            options: [
              { value: "pending", label: "Menunggu" },
              { value: "sent", label: "Terkirim" },
              { value: "failed", label: "Gagal" },
            ],
          },
        ]}
        page={page}
        pageCount={pages}
        onPageChange={setPage}
        total={total}
        pageSize={limit}
        onPageSizeChange={setPageSize}
      />
      <Table
        rowNumberStart={page * limit + 1}
        columns={["Waktu", "Jenis", "Channel", "Pengirim", "Penerima", "Pesan", "Status", "Keterangan", "Aksi"]}
        rows={rows.map((n) => {
          const st = LOG_STATUS[n.status] ?? { label: n.status, tone: "var(--muted)" };
          const body = n.body.length > 80 ? `${n.body.slice(0, 80)}…` : n.body;
          return [
            <span key="t" title={formatDateTime(n.created_at)}>
              {formatDateTime(n.created_at)}
            </span>,
            EVENT_LABELS[n.event] || n.event || "—",
            n.channel,
            n.sender ? (
              <span key="from" title={`Dikirim via ${n.sender}`}>
                {n.sender}
              </span>
            ) : (
              <span key="from" className="text-[var(--muted)]">
                —
              </span>
            ),
            n.recipient,
            <span key="b" title={n.body}>
              {body}
            </span>,
            <span key="s" style={{ color: st.tone, fontWeight: 600 }}>
              {st.label}
            </span>,
            n.error ? (
              <span key="e" className="text-[var(--danger)]" title={n.error}>
                {n.error.length > 60 ? `${n.error.slice(0, 60)}…` : n.error}
              </span>
            ) : n.status === "sent" ? (
              <span key="e" className="text-[var(--muted)]" title={n.sent_at ?? undefined}>
                terkirim {formatDateTime(n.sent_at)}
              </span>
            ) : n.attempts > 0 ? (
              <span key="e" className="text-[var(--muted)]">percobaan ke-{n.attempts + 1}</span>
            ) : (
              <span key="e" className="text-[var(--muted)]">—</span>
            ),
            n.status === "failed" ? (
              <IconButton
                key="r"
                label="Kirim ulang"
                disabled={resend.isPending}
                onClick={() => void onResend(n)}
              >
                <IconRefresh />
              </IconButton>
            ) : (
              <span key="r" className="text-[var(--muted)]">
                —
              </span>
            ),
          ];
        })}
      />
    </div>
  );
}
