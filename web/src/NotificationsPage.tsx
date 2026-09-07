import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { IconTrash } from "./icons";
import { toastError, toastSuccess } from "./swal";
import { Button, IconButton, Input, Section, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Table } from "./ui";

type NotifTemplate = {
  id: string;
  channel: string;
  event: string;
  subject?: string | null;
  body: string;
};

export function NotificationsPage() {
  const qc = useQueryClient();
  const [tab, setTab] = useState<"templates" | "broadcast">("broadcast");
  const templatesQ = useQuery({
    queryKey: ["notification-templates"],
    queryFn: () => api<NotifTemplate[]>("/api/notifications/templates"),
  });

  const [tpl, setTpl] = useState({ channel: "whatsapp", event: "broadcast", subject: "", body: "" });
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

  return (
    <Section title="Notifikasi">
      <div className="cluster-tabs mb-4">
        <button type="button" className={`cluster-tab ${tab === "broadcast" ? "is-active" : ""}`} onClick={() => setTab("broadcast")}>
          Broadcast
        </button>
        <button type="button" className={`cluster-tab ${tab === "templates" ? "is-active" : ""}`} onClick={() => setTab("templates")}>
          Template
        </button>
      </div>

      {tab === "broadcast" ? (
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
      ) : (
        <div className="grid gap-4">
          <div className="grid max-w-xl gap-3 rounded-[var(--radius-lg)] border border-[var(--border)] p-4">
            <label className="grid gap-1 text-sm">
              <span>Channel</span>
              <Select value={tpl.channel} onValueChange={(v) => setTpl({ ...tpl, channel: v })}>
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
              <span>Event key</span>
              <Input
                placeholder="invoice_reminder / payment_confirmation / broadcast / promo"
                value={tpl.event}
                onChange={(e) => setTpl({ ...tpl, event: e.target.value })}
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span>Subject (email)</span>
              <Input value={tpl.subject} onChange={(e) => setTpl({ ...tpl, subject: e.target.value })} />
            </label>
            <label className="grid gap-1 text-sm">
              <span>Body</span>
              <textarea className="input min-h-[100px]" value={tpl.body} onChange={(e) => setTpl({ ...tpl, body: e.target.value })} />
            </label>
            <Button type="button" onClick={() => saveTpl.mutate()} disabled={saveTpl.isPending}>
              Simpan template
            </Button>
          </div>

          <Table
            columns={["Channel", "Event", "Body", ""]}
            rows={(templatesQ.data || []).map((t) => [
              t.channel,
              t.event,
              <span key="b" className="line-clamp-2 max-w-md text-xs">
                {t.body}
              </span>,
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
      )}
    </Section>
  );
}
