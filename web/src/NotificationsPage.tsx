import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { IconTrash } from "./icons";
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
    body: "Halo, ini pengingat tagihan dari kami. Silakan bayar via portal pelanggan.",
    delay_seconds: 3,
    recipients: "",
  });

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
      if (bcast.audience === "custom") {
        body.recipients = bcast.recipients
          .split(/[\n,;]+/)
          .map((s) => s.trim())
          .filter(Boolean);
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
            <Select value={bcast.channel} onValueChange={(v) => setBcast({ ...bcast, channel: v })}>
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
            <span>Audience</span>
            <Select value={bcast.audience} onValueChange={(v) => setBcast({ ...bcast, audience: v })}>
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
          ) : null}
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
          <Button type="button" onClick={() => sendBcast.mutate()} disabled={sendBcast.isPending}>
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
  if (!s) return "—";
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString("id-ID", { dateStyle: "short", timeStyle: "short" });
}

function NotificationHistoryTab() {
  const [status, setStatus] = useState("");
  const [channel, setChannel] = useState("");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, 300);
  const [page, setPage] = useState(0);
  const limit = 20;

  const q = useQuery({
    queryKey: ["notification-history", status, channel, debouncedSearch, page],
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

  return (
    <div className="grid gap-3">
      <p className="text-sm text-[var(--muted)]">
        Riwayat semua pengiriman (dunning, konfirmasi bayar, broadcast, alert ops, laporan). Pakai kolom Keterangan
        untuk menelusuri kiriman yang gagal atau ganda.
      </p>
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
      />
      <Table
        rowNumberStart={page * limit + 1}
        columns={["Waktu", "Jenis", "Channel", "Penerima", "Pesan", "Status", "Keterangan"]}
        rows={rows.map((n) => {
          const st = LOG_STATUS[n.status] ?? { label: n.status, tone: "var(--muted)" };
          const body = n.body.length > 80 ? `${n.body.slice(0, 80)}…` : n.body;
          return [
            <span key="t" title={formatDateTime(n.created_at)}>
              {formatDateTime(n.created_at)}
            </span>,
            EVENT_LABELS[n.event] || n.event || "—",
            n.channel,
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
          ];
        })}
      />
    </div>
  );
}
