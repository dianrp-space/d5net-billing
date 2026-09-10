import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { IconTrash } from "./icons";
import { toastError, toastSuccess } from "./swal";
import { Button, IconButton, Input, Section, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Table } from "./ui";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
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
  invoice_number: "INV-demo-CUST-001-092026-A3F9K",
  amount: "150000",
  due_date: "10/09/2026",
  message: "(isi pesan broadcast)",
};

function renderPreview(body: string): string {
  return body.replace(/\{\{(\w+)\}\}/g, (_, key: string) => PREVIEW_SAMPLES[key] ?? `{{${key}}}`);
}

export function NotificationsPage() {
  const qc = useQueryClient();
  const [tab, setTab] = usePersistedTab("notifications", "broadcast", ["broadcast", "templates"] as const);
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
      return api<{ queued: number }>("/api/notifications/broadcast", {
        method: "POST",
        body: JSON.stringify(body),
      });
    },
    onSuccess: (r) => void toastSuccess(`${r.queued} pesan diantrekan (delay ${bcast.delay_seconds}s)`),
    onError: (e: Error) => void toastError(e.message),
  });

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
          if (v === "broadcast" || v === "templates") setTab(v);
        }}
        className="space-y-0"
      >
        <TabsList aria-label="Notifikasi">
          <TabsTrigger value="broadcast">Broadcast</TabsTrigger>
          <TabsTrigger value="templates">Template</TabsTrigger>
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
      </Tabs>
    </Section>
  );
}
