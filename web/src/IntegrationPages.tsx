import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api } from "./api";
import { useAppDialog } from "./confirm";
import { IconCopy, IconMail, IconSend, IconTrash, IconWhatsApp } from "./icons";
import { swalAlert, toastError, toastSuccess } from "./swal";
import { usePersistedTab } from "./navPersist";
import { FormDialog, IconButton, Section, SecretInput, Table } from "./ui";

type OutboundWebhook = {
  id: string;
  url: string;
  secret?: string | null;
  events: string[];
  is_active: boolean;
};

type PaymentIntegration = {
  configured: boolean;
  enabled: boolean;
  base_url: string;
  method: string;
  provider: string;
  env_fallback: boolean;
  api_key?: string;
  webhook_secret?: string;
  expires_in_minutes: number;
  webhook_path: string;
  webhook_url: string;
  webhook_base_hint: string;
};

type DuitkuIntegration = {
  configured: boolean;
  enabled: boolean;
  sandbox: boolean;
  merchant_code: string;
  api_key?: string;
  expires_in_minutes: number;
  webhook_path: string;
  webhook_url: string;
  webhook_base_hint: string;
};

function ttlHint(minutes: number) {
  const n = Number.isFinite(minutes) && minutes > 0 ? minutes : 15;
  if (n >= 1440) return "24 jam";
  if (n % 60 === 0) return n === 60 ? "1 jam" : `${n / 60} jam`;
  if (n > 60) {
    const h = Math.floor(n / 60);
    const m = n % 60;
    return `${h} jam ${m} menit`;
  }
  return `${n} menit`;
}

const TTL_PRESETS = [
  { minutes: 15, label: "15 menit" },
  { minutes: 60, label: "1 jam" },
  { minutes: 360, label: "6 jam" },
  { minutes: 1440, label: "24 jam" },
];

function paymentWebhookDisplayURL(path?: string, fallback = "/api/webhooks/payment/drp") {
  const raw = path || fallback;
  if (/^https?:\/\//i.test(raw)) return raw;
  const normalized = raw.startsWith("/") ? raw : `/${raw}`;
  if (typeof window === "undefined") return normalized;
  return `${window.location.origin}${normalized}`;
}

const EVENT_PRESETS = ["payment.paid", "invoice.created", "subscription.suspended", "subscription.activated"];

/** Sends a one-off messaging-gateway test message (whatsapp|telegram|email). */
function sendMessagingTest(payload: { channel: string; recipient?: string; subject?: string; body?: string }) {
  return api<{ status: string; channel: string }>("/api/integrations/messaging/test", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function WebhooksIntegrationPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const list = useQuery({
    queryKey: ["outbound-webhooks"],
    queryFn: () => api<OutboundWebhook[]>("/api/outbound-webhooks"),
  });
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ url: "", secret: "", events: ["payment.paid"] as string[] });
  const [err, setErr] = useState("");

  const create = useMutation({
    mutationFn: () =>
      api("/api/outbound-webhooks", {
        method: "POST",
        body: JSON.stringify({
          url: form.url.trim(),
          secret: form.secret.trim() || null,
          events: form.events,
        }),
      }),
    onSuccess: () => {
      setOpen(false);
      setForm({ url: "", secret: "", events: ["payment.paid"] });
      void qc.invalidateQueries({ queryKey: ["outbound-webhooks"] });
      void toastSuccess("Webhook ditambahkan");
    },
    onError: (e: Error) => {
      setErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/outbound-webhooks/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["outbound-webhooks"] });
      void toastSuccess("Webhook dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const rows = (Array.isArray(list.data) ? list.data : []).map((w) => [
    w.url,
    (w.events || []).join(", ") || "—",
    w.is_active ? "aktif" : "nonaktif",
    <IconButton
      key={w.id}
      label="Hapus webhook"
      danger
      onClick={async () => {
        const ok = await confirm({
          title: "Hapus webhook",
          description: `Hapus endpoint ${w.url}?`,
          confirmLabel: "Hapus",
        });
        if (ok) remove.mutate(w.id);
      }}
    >
      <IconTrash />
    </IconButton>,
  ]);

  function toggleEvent(ev: string) {
    const set = new Set(form.events);
    if (set.has(ev)) set.delete(ev);
    else set.add(ev);
    setForm({ ...form, events: [...set] });
  }

  return (
    <>
      <Section
        title="Outbound webhook"
        actions={
          <button
            type="button"
            className="btn"
            onClick={() => {
              setErr("");
              setOpen(true);
            }}
          >
            + Webhook
          </button>
        }
      >
        <p className="mb-3 text-sm text-[var(--muted)]">
          Kirim event tenant ke URL eksternal (mis. otomasi n8n / Zapier). Secret opsional untuk HMAC.
        </p>
        {list.isLoading ? (
          <p className="text-[var(--muted)]">Memuat...</p>
        ) : (
          <Table columns={["URL", "Events", "Status", "Aksi"]} rows={rows} />
        )}
      </Section>

      <FormDialog open={open} title="Tambah webhook" onClose={() => setOpen(false)} wide>
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <input
            className="input"
            placeholder="https://example.com/hooks/drp"
            value={form.url}
            onChange={(e) => setForm({ ...form, url: e.target.value })}
            required
            type="url"
          />
          <SecretInput
            placeholder="Secret (opsional)"
            value={form.secret}
            onChange={(e) => setForm({ ...form, secret: e.target.value })}
            autoComplete="off"
          />
          <div className="grid gap-2 sm:grid-cols-2">
            {EVENT_PRESETS.map((ev) => (
              <label key={ev} className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={form.events.includes(ev)} onChange={() => toggleEvent(ev)} />
                <code className="text-xs">{ev}</code>
              </label>
            ))}
          </div>
          <div className="flex gap-2">
            <button className="btn" disabled={create.isPending}>
              Simpan
            </button>
            <button type="button" className="btn-ghost" onClick={() => setOpen(false)}>
              Batal
            </button>
          </div>
          {err && <p className="text-sm text-[var(--danger)]">{err}</p>}
        </form>
      </FormDialog>
    </>
  );
}

export function PaymentGWPage() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["integration-payment"],
    queryFn: () => api<PaymentIntegration>("/api/integrations/payment"),
  });
  const duitkuQ = useQuery({
    queryKey: ["integration-duitku"],
    queryFn: () => api<DuitkuIntegration>("/api/integrations/duitku"),
  });
  const [form, setForm] = useState({
    enabled: false,
    base_url: "",
    api_key: "",
    webhook_secret: "",
    expires_in_minutes: 15,
  });
  const [duitkuForm, setDuitkuForm] = useState({
    enabled: false,
    sandbox: true,
    merchant_code: "",
    api_key: "",
    expires_in_minutes: 60,
  });

  useEffect(() => {
    if (!q.data) return;
    setForm({
      enabled: q.data.enabled,
      base_url: q.data.base_url || "",
      api_key: q.data.api_key || "",
      webhook_secret: q.data.webhook_secret || "",
      expires_in_minutes: q.data.expires_in_minutes || 15,
    });
  }, [q.data]);

  useEffect(() => {
    if (!duitkuQ.data) return;
    setDuitkuForm({
      enabled: duitkuQ.data.enabled,
      sandbox: duitkuQ.data.configured ? duitkuQ.data.sandbox : true,
      merchant_code: duitkuQ.data.merchant_code || "",
      api_key: duitkuQ.data.api_key || "",
      expires_in_minutes: duitkuQ.data.expires_in_minutes || 60,
    });
  }, [duitkuQ.data]);

  const save = useMutation({
    mutationFn: () =>
      api<PaymentIntegration>("/api/integrations/payment", {
        method: "PUT",
        body: JSON.stringify({
          enabled: form.enabled,
          base_url: form.base_url.trim(),
          api_key: form.api_key.trim() || undefined,
          webhook_secret: form.webhook_secret.trim() || undefined,
          expires_in_minutes: Math.min(1440, Math.max(1, form.expires_in_minutes || 15)),
        }),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["integration-payment"], data);
      setForm({
        enabled: data.enabled,
        base_url: data.base_url || "",
        api_key: data.api_key || form.api_key,
        webhook_secret: data.webhook_secret || form.webhook_secret,
        expires_in_minutes: data.expires_in_minutes || form.expires_in_minutes,
      });
      void qc.invalidateQueries({ queryKey: ["integration-payment"] });
      void toastSuccess("DRP Payment disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const saveDuitku = useMutation({
    mutationFn: () =>
      api<DuitkuIntegration>("/api/integrations/duitku", {
        method: "PUT",
        body: JSON.stringify({
          enabled: duitkuForm.enabled,
          sandbox: duitkuForm.sandbox,
          merchant_code: duitkuForm.merchant_code.trim(),
          api_key: duitkuForm.api_key.trim() || undefined,
          expires_in_minutes: Math.min(1440, Math.max(1, duitkuForm.expires_in_minutes || 60)),
        }),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["integration-duitku"], data);
      setDuitkuForm({
        enabled: data.enabled,
        sandbox: data.sandbox,
        merchant_code: data.merchant_code || "",
        api_key: data.api_key || duitkuForm.api_key,
        expires_in_minutes: data.expires_in_minutes || duitkuForm.expires_in_minutes,
      });
      void qc.invalidateQueries({ queryKey: ["integration-duitku"] });
      void toastSuccess("Duitku disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const webhookURL = paymentWebhookDisplayURL(q.data?.webhook_url || q.data?.webhook_path);
  const duitkuWebhookURL = paymentWebhookDisplayURL(
    duitkuQ.data?.webhook_url || duitkuQ.data?.webhook_path,
    "/api/webhooks/payment/duitku",
  );

  async function copyWebhook(url: string) {
    try {
      await navigator.clipboard.writeText(url);
      void toastSuccess("Callback URL disalin");
    } catch {
      void toastError("Gagal menyalin. Salin manual dari kolom URL.");
    }
  }

  const loading = q.isLoading || duitkuQ.isLoading;

  return (
    <Section title="Payment Gateway">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Aktifkan gateway per tenant. <strong>DRP Payment</strong> untuk QRIS di aplikasi,{" "}
        <strong>Duitku</strong> untuk redirect ke halaman bayar Duitku. Kredensial disimpan terenkripsi.
      </p>
      {loading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <Accordion type="multiple" className="grid max-w-xl gap-3">
          <ProviderAccordionItem
            value="drp"
            title="DRP Payment · QRIS"
            enabled={form.enabled}
            configured={Boolean(q.data?.configured)}
            onToggle={(v) => setForm({ ...form, enabled: v })}
          >
            <p className="text-[11px] leading-relaxed text-[var(--muted)]">
              Setiap QRIS mendapat <strong>kode unik 3 digit</strong> yang ditambahkan ke nominal tagihan. Pelanggan harus
              bayar <strong>tepat</strong> jumlah itu supaya konfirmasi otomatis (webhook) bisa mencocokkan pembayaran.
            </p>
            <p className="text-[11px] leading-relaxed text-[var(--muted)]">
              Gunakan <strong>kredensial DRP milik tenant ini</strong> (API key &amp; webhook secret sendiri). Kredensial
              platform/env tidak dipakai untuk tenant.
            </p>
            {q.data?.env_fallback ? (
              <p className="text-[11px] text-[var(--muted)]">Kunci platform (env) aktif. Isi field di bawah untuk override per tenant.</p>
            ) : null}
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">API base URL</span>
              <input
                className="input"
                placeholder="https://payment.dianrp.com"
                value={form.base_url}
                onChange={(e) => setForm({ ...form, base_url: e.target.value })}
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">API key</span>
              <SecretInput
                name="drp-payment-api-key"
                placeholder="API key (drp_live_…)"
                value={form.api_key}
                onChange={(e) => setForm({ ...form, api_key: e.target.value })}
                autoComplete="new-password"
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Webhook secret</span>
              <SecretInput
                name="drp-payment-webhook-secret"
                placeholder="Webhook secret"
                value={form.webhook_secret}
                onChange={(e) => setForm({ ...form, webhook_secret: e.target.value })}
                autoComplete="new-password"
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Masa berlaku QRIS (TTL)</span>
              <input
                className="input"
                type="number"
                min={1}
                max={1440}
                step={1}
                value={form.expires_in_minutes}
                onChange={(e) =>
                  setForm({
                    ...form,
                    expires_in_minutes: Number(e.target.value) || 15,
                  })
                }
              />
              <div className="flex flex-wrap gap-1">
                {TTL_PRESETS.map((p) => (
                  <button
                    key={p.minutes}
                    type="button"
                    className="btn-ghost px-2 py-1 text-[11px]"
                    onClick={() => setForm({ ...form, expires_in_minutes: p.minutes })}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
              <span className="text-[11px] text-[var(--muted)]">
                QR baru berlaku {ttlHint(form.expires_in_minutes)} (1–1440 menit). QR yang sudah terbit tetap dipakai
                sampai waktu kadaluwarsanya sendiri.
              </span>
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Webhook URL DRP · /api/webhooks/payment/drp?tenant=…</span>
              <div className="flex gap-2">
                <input className="input min-w-0 flex-1 font-mono text-xs" readOnly value={webhookURL} />
                <IconButton label="Salin webhook URL" onClick={() => void copyWebhook(webhookURL)}>
                  <IconCopy />
                </IconButton>
              </div>
              <span className="text-[11px] text-[var(--muted)]">
                Khusus DRP Payment / QRIS, terpisah dari Duitku. Tempel di dashboard merchant. Domain harus publik.
              </span>
            </label>
            <button type="button" className="btn w-fit" disabled={save.isPending} onClick={() => save.mutate()}>
              {save.isPending ? "Menyimpan..." : "Simpan"}
            </button>
          </ProviderAccordionItem>

          <ProviderAccordionItem
            value="duitku"
            title="Duitku"
            enabled={duitkuForm.enabled}
            configured={Boolean(duitkuQ.data?.configured)}
            onToggle={(v) => setDuitkuForm({ ...duitkuForm, enabled: v })}
          >
            <p className="text-[11px] leading-relaxed text-[var(--muted)]">
              Pelanggan diarahkan ke <strong>halaman bayar Duitku</strong> (bukan API v2 / MD5). Callback memakai HMAC-SHA256.
              Isi callback URL di bawah ke dashboard Duitku.
            </p>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={duitkuForm.sandbox}
                onChange={(e) => setDuitkuForm({ ...duitkuForm, sandbox: e.target.checked })}
              />
              Sandbox (api-sandbox.duitku.com)
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Merchant code</span>
              <input
                className="input"
                placeholder="Dxxxxx"
                value={duitkuForm.merchant_code}
                onChange={(e) => setDuitkuForm({ ...duitkuForm, merchant_code: e.target.value })}
                autoComplete="off"
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">API key</span>
              <SecretInput
                name="duitku-api-key"
                placeholder="API key Duitku"
                value={duitkuForm.api_key}
                onChange={(e) => setDuitkuForm({ ...duitkuForm, api_key: e.target.value })}
                autoComplete="new-password"
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Masa berlaku invoice (TTL)</span>
              <input
                className="input"
                type="number"
                min={1}
                max={1440}
                step={1}
                value={duitkuForm.expires_in_minutes}
                onChange={(e) =>
                  setDuitkuForm({
                    ...duitkuForm,
                    expires_in_minutes: Number(e.target.value) || 60,
                  })
                }
              />
              <div className="flex flex-wrap gap-1">
                {TTL_PRESETS.map((p) => (
                  <button
                    key={`duitku-${p.minutes}`}
                    type="button"
                    className="btn-ghost px-2 py-1 text-[11px]"
                    onClick={() => setDuitkuForm({ ...duitkuForm, expires_in_minutes: p.minutes })}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
              <span className="text-[11px] text-[var(--muted)]">
                Invoice Duitku berlaku {ttlHint(duitkuForm.expires_in_minutes)}.
              </span>
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Callback URL Duitku · /api/webhooks/payment/duitku?tenant=…</span>
              <div className="flex gap-2">
                <input className="input min-w-0 flex-1 font-mono text-xs" readOnly value={duitkuWebhookURL} />
                <IconButton label="Salin callback URL" onClick={() => void copyWebhook(duitkuWebhookURL)}>
                  <IconCopy />
                </IconButton>
              </div>
              <span className="text-[11px] text-[var(--muted)]">
                Khusus Duitku, terpisah dari QRIS. Tempel di dashboard Duitku. Domain harus publik; callback berupa form POST.
              </span>
            </label>
            <button type="button" className="btn w-fit" disabled={saveDuitku.isPending} onClick={() => saveDuitku.mutate()}>
              {saveDuitku.isPending ? "Menyimpan..." : "Simpan"}
            </button>
          </ProviderAccordionItem>
        </Accordion>
      )}
    </Section>
  );
}

function ProviderAccordionItem({
  value,
  title,
  enabled,
  configured,
  onToggle,
  children,
}: {
  value: string;
  title: string;
  enabled: boolean;
  configured: boolean;
  onToggle: (v: boolean) => void;
  children: ReactNode;
}) {
  return (
    <AccordionItem value={value} className="rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3">
      <div className="flex items-center gap-2">
        <AccordionTrigger className="min-w-0 flex-1 py-3 hover:no-underline">
          <span className="min-w-0 flex-1">
            <span className="block text-sm font-medium">{title}</span>
            <span className="mt-0.5 block text-[10px] text-[var(--muted)]">
              {configured ? "kunci tersimpan" : "belum dikonfigurasi"}
              {enabled ? " · aktif" : " · nonaktif"}
            </span>
          </span>
        </AccordionTrigger>
        <label
          className="flex shrink-0 items-center gap-2 py-3 text-sm"
          onClick={(e) => e.stopPropagation()}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <input type="checkbox" checked={enabled} onChange={(e) => onToggle(e.target.checked)} />
          Aktif
        </label>
      </div>
      <AccordionContent>
        <div className="grid gap-2">{children}</div>
      </AccordionContent>
    </AccordionItem>
  );
}

export function MessagingGWPage() {
  const [tab, setTab] = usePersistedTab("messaging-channel", "whatsapp", ["whatsapp", "telegram", "email"]);
  const current = tab === "telegram" || tab === "email" ? tab : "whatsapp";
  return (
    <Tabs
      value={current}
      onValueChange={(v) => {
        if (v === "whatsapp" || v === "telegram" || v === "email") setTab(v);
      }}
      className="space-y-0"
    >
      <TabsList aria-label="Messaging Gateway">
        <TabsTrigger value="whatsapp">
          <IconWhatsApp />
          WhatsApp
        </TabsTrigger>
        <TabsTrigger value="telegram">
          <IconSend />
          Telegram
        </TabsTrigger>
        <TabsTrigger value="email">
          <IconMail />
          Email
        </TabsTrigger>
      </TabsList>
      <TabsContent value="whatsapp">
        <WhatsAppTab />
      </TabsContent>
      <TabsContent value="telegram">
        <TelegramTab />
      </TabsContent>
      <TabsContent value="email">
        <SmtpTab />
      </TabsContent>
    </Tabs>
  );
}

type WADevice = { device_id: string; label: string };
type WhatsAppIntegration = {
  configured: boolean;
  enabled: boolean;
  base_url: string;
  username: string;
  password?: string;
  devices: WADevice[];
};
type WACheckRow = {
  device_id: string;
  label?: string;
  connected: boolean;
  logged_in: boolean;
  jid?: string;
  error?: string;
};

function WhatsAppTab() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["integration-whatsapp"],
    queryFn: () => api<WhatsAppIntegration>("/api/integrations/whatsapp"),
  });
  const [form, setForm] = useState({
    enabled: false,
    base_url: "",
    username: "",
    password: "",
    devices: [] as WADevice[],
  });
  const [checks, setChecks] = useState<WACheckRow[]>([]);

  useEffect(() => {
    if (!q.data) return;
    setForm({
      enabled: q.data.enabled,
      base_url: q.data.base_url || "",
      username: q.data.username || "",
      password: q.data.password || "",
      devices: Array.isArray(q.data.devices) && q.data.devices.length ? q.data.devices : [{ device_id: "", label: "" }],
    });
  }, [q.data]);

  const save = useMutation({
    mutationFn: () =>
      api<WhatsAppIntegration>("/api/integrations/whatsapp", {
        method: "PUT",
        body: JSON.stringify({
          enabled: form.enabled,
          base_url: form.base_url.trim(),
          username: form.username.trim(),
          password: form.password.trim() || undefined,
          devices: form.devices
            .map((d) => ({ device_id: d.device_id.trim(), label: d.label.trim() }))
            .filter((d) => d.device_id !== ""),
        }),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["integration-whatsapp"], data);
      void qc.invalidateQueries({ queryKey: ["integration-whatsapp"] });
      void toastSuccess("WhatsApp gateway disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const check = useMutation({
    mutationFn: () =>
      api<{ devices: WACheckRow[] }>("/api/integrations/whatsapp/check", { method: "POST", body: "{}" }),
    onSuccess: (data) => {
      const rows = data.devices || [];
      setChecks(rows);
      const ok = rows.filter((r) => r.logged_in).length;
      void swalAlert({
        title: ok > 0 ? "WhatsApp terhubung" : "Gateway merespons",
        description:
          `${ok}/${rows.length} nomor login` +
          rows
            .map((r) => `\n• ${r.device_id || "(default)"}: ${r.error ? r.error : r.logged_in ? "OK" : "belum login"}`)
            .join(""),
        icon: ok > 0 ? "success" : "warning",
      });
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const [loadMsg, setLoadMsg] = useState("");
  const loadDevices = useMutation({
    mutationFn: () =>
      api<{ devices: { device_id: string; name?: string; jid?: string }[] }>(
        "/api/integrations/whatsapp/devices",
      ),
    onSuccess: (data) => {
      const list = data.devices || [];
      if (!list.length) {
        setLoadMsg("Gateway tidak melaporkan device apa pun.");
        return;
      }
      setForm((f) => {
        const existing = new Set(f.devices.map((d) => d.device_id.trim()));
        const merged = [...f.devices];
        for (const d of list) {
          if (!existing.has(d.device_id)) {
            merged.push({ device_id: d.device_id, label: d.name || d.jid || "" });
          }
        }
        return { ...f, devices: merged.filter((d, i) => i === 0 || d.device_id.trim() !== "") };
      });
      setLoadMsg(`Ditemukan ${list.length} device. Periksa & simpan.`);
    },
    onError: (e: Error) => setLoadMsg(e.message),
  });

  const [testPhone, setTestPhone] = useState("");
  const testSend = useMutation({
    mutationFn: () => sendMessagingTest({ channel: "whatsapp", recipient: testPhone.trim() }),
    onSuccess: () => void toastSuccess("Pesan tes WhatsApp terkirim"),
    onError: (e: Error) => void toastError(e.message),
  });

  function setDevice(i: number, patch: Partial<WADevice>) {
    setForm((f) => ({ ...f, devices: f.devices.map((d, j) => (j === i ? { ...d, ...patch } : d)) }));
  }
  function addDevice() {
    setForm((f) => ({ ...f, devices: [...f.devices, { device_id: "", label: "" }] }));
  }
  function removeDevice(i: number) {
    setForm((f) => ({ ...f, devices: f.devices.filter((_, j) => j !== i) }));
  }

  return (
    <Section title="WhatsApp (gateway eksternal)">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Kirim WhatsApp lewat gateway GOWA (go-whatsapp-web-multidevice) milik tenant. Bisa lebih dari satu nomor
        (device) pada base URL yang sama; pengiriman mencoba nomor berikutnya bila yang pertama gagal (redundan).
        Login/scan QR di dashboard gateway.
      </p>
      {q.isLoading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <div className="grid max-w-xl gap-4">
          <div className="rounded-xl border border-[var(--border)] p-3">
            <div className="mb-3 flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">Gateway WhatsApp</p>
                <p className="text-[10px] text-[var(--muted)]">
                  {q.data?.configured ? "base URL tersimpan" : "belum dikonfigurasi"}
                </p>
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                />
                Aktif
              </label>
            </div>
            <div className="grid gap-2">
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Base URL gateway</span>
                <input
                  className="input"
                  placeholder="https://wa-gateway.example.com"
                  value={form.base_url}
                  onChange={(e) => setForm({ ...form, base_url: e.target.value })}
                />
              </label>
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Basic Auth username</span>
                <input
                  className="input"
                  placeholder="username"
                  value={form.username}
                  onChange={(e) => setForm({ ...form, username: e.target.value })}
                  autoComplete="off"
                />
              </label>
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Basic Auth password</span>
                <SecretInput
                  name="whatsapp-gateway-password"
                  placeholder="password"
                  value={form.password}
                  onChange={(e) => setForm({ ...form, password: e.target.value })}
                  autoComplete="new-password"
                />
              </label>
            </div>
          </div>

          <div className="rounded-xl border border-[var(--border)] p-3">
            <div className="mb-2 flex items-center justify-between">
              <p className="text-sm font-medium">Nomor / device</p>
              <button type="button" className="btn-ghost" onClick={addDevice}>
                + Tambah nomor
              </button>
            </div>
            <div className="grid gap-2">
              {form.devices.map((d, i) => (
                <div key={i} className="flex flex-wrap items-end gap-2">
                  <label className="grid flex-1 gap-1 text-sm">
                    <span className="text-[10px] text-[var(--muted)]">Device ID</span>
                    <input
                      className="input"
                      placeholder="org_2 / 628xxx"
                      value={d.device_id}
                      onChange={(e) => setDevice(i, { device_id: e.target.value })}
                    />
                  </label>
                  <label className="grid flex-1 gap-1 text-sm">
                    <span className="text-[10px] text-[var(--muted)]">Label (opsional)</span>
                    <input
                      className="input"
                      placeholder="Nomor utama"
                      value={d.label}
                      onChange={(e) => setDevice(i, { label: e.target.value })}
                    />
                  </label>
                  <IconButton
                    label="Hapus nomor"
                    danger
                    onClick={() => removeDevice(i)}
                    disabled={form.devices.length <= 1}
                  >
                    <IconTrash />
                  </IconButton>
                </div>
              ))}
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <button
                type="button"
                className="btn-ghost"
                disabled={loadDevices.isPending || !form.base_url.trim()}
                onClick={() => {
                  setLoadMsg("");
                  loadDevices.mutate();
                }}
              >
                {loadDevices.isPending ? "Memuat..." : "Muat device dari gateway"}
              </button>
              {loadMsg ? <span className="text-xs text-[var(--muted)]">{loadMsg}</span> : null}
            </div>
            <p className="mt-1 text-xs text-[var(--muted)]">
              Kosongkan device ID untuk memakai device default gateway (bila gateway hanya punya satu nomor).
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <button type="button" className="btn w-fit" disabled={save.isPending} onClick={() => save.mutate()}>
              {save.isPending ? "Menyimpan..." : "Simpan"}
            </button>
            <button
              type="button"
              className="btn-ghost"
              disabled={check.isPending || !form.base_url.trim()}
              onClick={() => check.mutate()}
            >
              {check.isPending ? "Mengecek..." : "Cek koneksi"}
            </button>
          </div>
          {checks.length ? (
            <div className="rounded-xl border border-[var(--border)] p-3 text-sm">
              {checks.map((r) => (
                <p key={r.device_id || "default"} className="text-[var(--muted)]">
                  <strong className="text-[var(--text)]">{r.device_id || "(default)"}</strong>
                  {r.label ? ` · ${r.label}` : ""}: {r.error ? r.error : r.logged_in ? `OK${r.jid ? ` · ${r.jid}` : ""}` : "belum login"}
                </p>
              ))}
            </div>
          ) : null}

          <div className="rounded-xl border border-[var(--border)] p-3">
            <p className="mb-2 text-sm font-medium">Tes kirim</p>
            <div className="flex flex-wrap gap-2">
              <input
                className="input min-w-[200px] flex-1"
                placeholder="No. WhatsApp tujuan (08\u2026 / 62\u2026)"
                value={testPhone}
                onChange={(e) => setTestPhone(e.target.value)}
              />
              <button
                type="button"
                className="btn-ghost"
                disabled={testSend.isPending || !testPhone.trim()}
                onClick={() => testSend.mutate()}
              >
                {testSend.isPending ? "Mengirim..." : "Kirim tes"}
              </button>
            </div>
            <p className="mt-1 text-xs text-[var(--muted)]">
              Simpan dulu perubahan base URL/kredensial, lalu kirim tes.
            </p>
          </div>
        </div>
      )}
    </Section>
  );
}

type TelegramIntegration = {
  telegram_configured: boolean;
  telegram_enabled: boolean;
  telegram_chat_id: string;
  telegram_bot_token?: string;
};

function TelegramTab() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["integration-telegram"],
    queryFn: () => api<TelegramIntegration>("/api/integrations/telegram"),
  });
  const [form, setForm] = useState({
    telegram_bot_token: "",
    telegram_chat_id: "",
    telegram_enabled: false,
  });

  useEffect(() => {
    if (!q.data) return;
    setForm({
      telegram_bot_token: q.data.telegram_bot_token || "",
      telegram_chat_id: q.data.telegram_chat_id || "",
      telegram_enabled: q.data.telegram_enabled,
    });
  }, [q.data]);

  const save = useMutation({
    mutationFn: () =>
      api<TelegramIntegration>("/api/integrations/telegram", {
        method: "PUT",
        body: JSON.stringify({
          telegram_enabled: form.telegram_enabled,
          telegram_chat_id: form.telegram_chat_id.trim(),
          telegram_bot_token: form.telegram_bot_token.trim() || undefined,
        }),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["integration-telegram"], data);
      setForm({
        telegram_bot_token: data.telegram_bot_token || form.telegram_bot_token,
        telegram_chat_id: data.telegram_chat_id || "",
        telegram_enabled: data.telegram_enabled,
      });
      void qc.invalidateQueries({ queryKey: ["integration-telegram"] });
      void toastSuccess("Telegram Gateway disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const testSend = useMutation({
    mutationFn: () =>
      sendMessagingTest({ channel: "telegram", recipient: form.telegram_chat_id.trim() }),
    onSuccess: () => void toastSuccess("Pesan tes Telegram terkirim"),
    onError: (e: Error) => void toastError(e.message),
  });

  return (
    <Section title="Telegram Gateway">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Alert ke grup/channel/DM admin (bukan ke pelanggan). Aktifkan, isi bot token + chat ID, lalu start bot di chat
        tujuan. Pesan terkirim otomatis untuk: tiket baru, work order baru, auto isolir, router tidak merespons, dan
        drift reconcile mingguan. Router down paling banyak sekali per hari.
      </p>
      {q.isLoading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <div className="grid max-w-xl gap-4">
          <div className="rounded-xl border border-[var(--border)] p-3">
            <div className="mb-3 flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">Bot + chat tenant</p>
                <p className="text-[10px] text-[var(--muted)]">
                  {q.data?.telegram_configured ? "token tersimpan" : "butuh token + chat ID"}
                </p>
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={form.telegram_enabled}
                  onChange={(e) => setForm({ ...form, telegram_enabled: e.target.checked })}
                />
                Aktif
              </label>
            </div>
            <div className="grid gap-2">
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Bot token</span>
                <SecretInput
                  name="telegram-bot-token"
                  placeholder="Bot token (123456:AA…)"
                  value={form.telegram_bot_token}
                  onChange={(e) => setForm({ ...form, telegram_bot_token: e.target.value })}
                  autoComplete="new-password"
                />
              </label>
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Chat ID</span>
                <input
                  className="input"
                  placeholder="Chat ID (contoh: -100123… atau 123456789)"
                  value={form.telegram_chat_id}
                  onChange={(e) => setForm({ ...form, telegram_chat_id: e.target.value })}
                />
              </label>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <button type="button" className="btn w-fit" disabled={save.isPending} onClick={() => save.mutate()}>
              {save.isPending ? "Menyimpan..." : "Simpan"}
            </button>
            <button
              type="button"
              className="btn-ghost"
              disabled={testSend.isPending || !form.telegram_chat_id.trim()}
              onClick={() => testSend.mutate()}
            >
              {testSend.isPending ? "Mengirim..." : "Kirim tes"}
            </button>
          </div>
          <p className="text-xs text-[var(--muted)]">
            Simpan dulu perubahan token/chat ID, lalu kirim tes untuk memastikan bot berfungsi.
          </p>
        </div>
      )}
    </Section>
  );
}

type SmtpIntegration = {
  configured: boolean;
  enabled: boolean;
  host: string;
  port: number;
  username: string;
  password?: string;
  from: string;
  from_name: string;
  env_fallback: boolean;
};

function SmtpTab() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["integration-smtp"],
    queryFn: () => api<SmtpIntegration>("/api/integrations/smtp"),
  });
  const [form, setForm] = useState({
    enabled: false,
    host: "",
    port: 587,
    username: "",
    password: "",
    from: "",
    from_name: "",
  });

  useEffect(() => {
    if (!q.data) return;
    setForm({
      enabled: q.data.enabled,
      host: q.data.host || "",
      port: q.data.port || 587,
      username: q.data.username || "",
      password: q.data.password || "",
      from: q.data.from || "",
      from_name: q.data.from_name || "",
    });
  }, [q.data]);

  const save = useMutation({
    mutationFn: () =>
      api<SmtpIntegration>("/api/integrations/smtp", {
        method: "PUT",
        body: JSON.stringify({
          enabled: form.enabled,
          host: form.host.trim(),
          port: Number(form.port) || 587,
          username: form.username.trim(),
          password: form.password.trim() || undefined,
          from: form.from.trim(),
          from_name: form.from_name.trim(),
        }),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["integration-smtp"], data);
      setForm({
        enabled: data.enabled,
        host: data.host || "",
        port: data.port || 587,
        username: data.username || "",
        password: data.password || form.password,
        from: data.from || "",
        from_name: data.from_name || "",
      });
      void qc.invalidateQueries({ queryKey: ["integration-smtp"] });
      void toastSuccess("SMTP tenant disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const [testEmail, setTestEmail] = useState("");
  const testSend = useMutation({
    mutationFn: () => sendMessagingTest({ channel: "email", recipient: testEmail.trim() }),
    onSuccess: () => void toastSuccess("Email tes terkirim"),
    onError: (e: Error) => void toastError(e.message),
  });

  return (
    <Section title="Email (SMTP tenant)">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Server SMTP milik tenant untuk notifikasi email (laporan bulanan, broadcast, dunning). Port 587 memakai
        STARTTLS; port 465 memakai TLS langsung. Password kosong saat simpan = tetap memakai yang tersimpan. Jika
        nonaktif, sistem memakai SMTP platform (env) bila ada.
      </p>
      {q.isLoading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <div className="grid max-w-xl gap-4">
          <div className="rounded-xl border border-[var(--border)] p-3">
            <div className="mb-3 flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">Server SMTP</p>
                <p className="text-[10px] text-[var(--muted)]">
                  {q.data?.configured
                    ? "host + From tersimpan"
                    : q.data?.env_fallback
                      ? "belum diisi · fallback SMTP platform"
                      : "belum dikonfigurasi"}
                </p>
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                />
                Aktif
              </label>
            </div>
            <div className="grid gap-2">
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Host</span>
                <input
                  className="input"
                  placeholder="smtp.domain.id"
                  value={form.host}
                  onChange={(e) => setForm({ ...form, host: e.target.value })}
                  autoComplete="off"
                />
              </label>
              <div className="grid grid-cols-2 gap-2">
                <label className="grid gap-1 text-sm">
                  <span className="text-[var(--muted)]">Port</span>
                  <input
                    className="input"
                    type="number"
                    min={1}
                    max={65535}
                    placeholder="587"
                    value={form.port}
                    onChange={(e) => setForm({ ...form, port: Number(e.target.value) || 0 })}
                  />
                </label>
                <label className="grid gap-1 text-sm">
                  <span className="text-[var(--muted)]">Nama pengirim</span>
                  <input
                    className="input"
                    placeholder="Nama ISP"
                    value={form.from_name}
                    onChange={(e) => setForm({ ...form, from_name: e.target.value })}
                  />
                </label>
              </div>
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">From (email)</span>
                <input
                  className="input"
                  type="email"
                  placeholder="noreply@domain.id"
                  value={form.from}
                  onChange={(e) => setForm({ ...form, from: e.target.value })}
                  autoComplete="off"
                />
              </label>
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Username</span>
                <input
                  className="input"
                  placeholder="Biasanya sama dengan From"
                  value={form.username}
                  onChange={(e) => setForm({ ...form, username: e.target.value })}
                  autoComplete="off"
                />
              </label>
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Password</span>
                <SecretInput
                  name="smtp-password"
                  placeholder="Password SMTP"
                  value={form.password}
                  onChange={(e) => setForm({ ...form, password: e.target.value })}
                  autoComplete="new-password"
                />
              </label>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <button type="button" className="btn w-fit" disabled={save.isPending} onClick={() => save.mutate()}>
              {save.isPending ? "Menyimpan..." : "Simpan"}
            </button>
          </div>
          <div className="rounded-xl border border-[var(--border)] p-3">
            <p className="mb-2 text-sm font-medium">Tes kirim email</p>
            <div className="flex flex-wrap gap-2">
              <input
                className="input min-w-[200px] flex-1"
                type="email"
                placeholder="Email tujuan tes"
                value={testEmail}
                onChange={(e) => setTestEmail(e.target.value)}
              />
              <button
                type="button"
                className="btn-ghost"
                disabled={testSend.isPending || !testEmail.trim()}
                onClick={() => testSend.mutate()}
              >
                {testSend.isPending ? "Mengirim..." : "Kirim tes"}
              </button>
            </div>
            <p className="mt-1 text-xs text-[var(--muted)]">
              Simpan dulu perubahan SMTP, lalu kirim tes untuk memastikan server email berfungsi.
            </p>
          </div>
        </div>
      )}
    </Section>
  );
}

