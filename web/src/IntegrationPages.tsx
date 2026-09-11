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

type DokuIntegration = {
  configured: boolean;
  enabled: boolean;
  sandbox: boolean;
  client_id: string;
  secret_key?: string;
  has_private_key: boolean;
  merchant_id: string;
  terminal_id: string;
  postal_code: string;
  expires_in_minutes: number;
  qr_enabled: boolean;
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

function paymentWebhookDisplayURL(path?: string, fallback = "/api/webhooks/payment/duitku") {
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
          Kirim event ke URL eksternal (mis. otomasi n8n / Zapier). Secret opsional untuk HMAC.
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
  const duitkuQ = useQuery({
    queryKey: ["integration-duitku"],
    queryFn: () => api<DuitkuIntegration>("/api/integrations/duitku"),
  });
  const dokuQ = useQuery({
    queryKey: ["integration-doku"],
    queryFn: () => api<DokuIntegration>("/api/integrations/doku"),
  });
  const [duitkuForm, setDuitkuForm] = useState({
    enabled: false,
    sandbox: true,
    merchant_code: "",
    api_key: "",
    expires_in_minutes: 60,
  });
  const [dokuForm, setDokuForm] = useState({
    enabled: false,
    sandbox: true,
    client_id: "",
    secret_key: "",
    private_key: "",
    merchant_id: "",
    terminal_id: "",
    postal_code: "",
    expires_in_minutes: 60,
  });

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

  useEffect(() => {
    if (!dokuQ.data) return;
    setDokuForm({
      enabled: dokuQ.data.enabled,
      sandbox: dokuQ.data.configured ? dokuQ.data.sandbox : true,
      client_id: dokuQ.data.client_id || "",
      secret_key: dokuQ.data.secret_key || "",
      private_key: "",
      merchant_id: dokuQ.data.merchant_id || "",
      terminal_id: dokuQ.data.terminal_id || "",
      postal_code: dokuQ.data.postal_code || "",
      expires_in_minutes: dokuQ.data.expires_in_minutes || 60,
    });
  }, [dokuQ.data]);

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

  const duitkuWebhookURL = paymentWebhookDisplayURL(
    duitkuQ.data?.webhook_url || duitkuQ.data?.webhook_path,
    "/api/webhooks/payment/duitku",
  );
  const dokuWebhookURL = paymentWebhookDisplayURL(
    dokuQ.data?.webhook_url || dokuQ.data?.webhook_path,
    "/api/webhooks/payment/doku",
  );

  async function copyWebhook(url: string) {
    try {
      await navigator.clipboard.writeText(url);
      void toastSuccess("Callback URL disalin");
    } catch {
      void toastError("Gagal menyalin. Salin manual dari kolom URL.");
    }
  }

  const loading = duitkuQ.isLoading || dokuQ.isLoading;

  const saveDoku = useMutation({
    mutationFn: () =>
      api<DokuIntegration>("/api/integrations/doku", {
        method: "PUT",
        body: JSON.stringify({
          enabled: dokuForm.enabled,
          sandbox: dokuForm.sandbox,
          client_id: dokuForm.client_id.trim(),
          secret_key: dokuForm.secret_key.trim() || undefined,
          private_key: dokuForm.private_key.trim() || undefined,
          merchant_id: dokuForm.merchant_id.trim() || undefined,
          terminal_id: dokuForm.terminal_id.trim() || undefined,
          postal_code: dokuForm.postal_code.trim() || undefined,
          expires_in_minutes: Math.min(1440, Math.max(1, dokuForm.expires_in_minutes || 60)),
        }),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["integration-doku"], data);
      setDokuForm({
        enabled: data.enabled,
        sandbox: data.sandbox,
        client_id: data.client_id || "",
        secret_key: data.secret_key || dokuForm.secret_key,
        private_key: "",
        merchant_id: data.merchant_id || "",
        terminal_id: data.terminal_id || "",
        postal_code: data.postal_code || "",
        expires_in_minutes: data.expires_in_minutes || dokuForm.expires_in_minutes,
      });
      void qc.invalidateQueries({ queryKey: ["integration-doku"] });
      void toastSuccess("DOKU disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  return (
    <Section title="Payment Gateway">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Aktifkan <strong>Duitku</strong> untuk pembayaran online (VA, e-wallet, retail, QRIS) atau{" "}
        <strong>DOKU</strong> (halaman bayar + QRIS langsung). Kredensial disimpan terenkripsi.
      </p>
      {loading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <Accordion type="multiple" className="grid max-w-xl gap-3">
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
              <span className="text-[var(--muted)]">Callback URL Duitku · /api/webhooks/payment/duitku</span>
              <div className="flex gap-2">
                <input className="input min-w-0 flex-1 font-mono text-xs" readOnly value={duitkuWebhookURL} />
                <IconButton label="Salin callback URL" onClick={() => void copyWebhook(duitkuWebhookURL)}>
                  <IconCopy />
                </IconButton>
              </div>
              <span className="text-[11px] text-[var(--muted)]">
                Tempel di dashboard Duitku. Domain harus publik; callback berupa form POST.
              </span>
            </label>
            <button type="button" className="btn w-fit" disabled={saveDuitku.isPending} onClick={() => saveDuitku.mutate()}>
              {saveDuitku.isPending ? "Menyimpan..." : "Simpan"}
            </button>
          </ProviderAccordionItem>

          <ProviderAccordionItem
            value="doku"
            title="DOKU"
            enabled={dokuForm.enabled}
            configured={Boolean(dokuQ.data?.configured)}
            onToggle={(v) => setDokuForm({ ...dokuForm, enabled: v })}
          >
            <p className="text-[11px] leading-relaxed text-[var(--muted)]">
              Pelanggan membayar di <strong>halaman bayar DOKU</strong> (VA, kartu, e-wallet, QRIS, retail).
              Untuk QRIS langsung (gambar QR di portal & bot WA), lengkapi juga private key + merchant ID +
              terminal ID + kode pos.
            </p>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={dokuForm.sandbox}
                onChange={(e) => setDokuForm({ ...dokuForm, sandbox: e.target.checked })}
              />
              Sandbox (api-sandbox.doku.com)
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Client ID</span>
              <input
                className="input"
                placeholder="BRN-xxxx / MCH-xxxx"
                value={dokuForm.client_id}
                onChange={(e) => setDokuForm({ ...dokuForm, client_id: e.target.value })}
                autoComplete="off"
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Secret key</span>
              <SecretInput
                name="doku-secret-key"
                placeholder="SK-…"
                value={dokuForm.secret_key}
                onChange={(e) => setDokuForm({ ...dokuForm, secret_key: e.target.value })}
                autoComplete="new-password"
              />
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">RSA private key (PEM — untuk QRIS Direct)</span>
              <textarea
                className="input font-mono text-xs"
                rows={3}
                placeholder="-----BEGIN PRIVATE KEY-----"
                value={dokuForm.private_key}
                onChange={(e) => setDokuForm({ ...dokuForm, private_key: e.target.value })}
                autoComplete="off"
                spellCheck={false}
              />
              <span className="text-[11px] text-[var(--muted)]">
                {dokuQ.data?.has_private_key ? "Private key tersimpan (kosongkan bila tidak diganti)." : "Wajib untuk QRIS langsung."}
              </span>
            </label>
            <div className="grid grid-cols-3 gap-2">
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Merchant ID</span>
                <input
                  className="input"
                  placeholder="mall ID QRIS"
                  value={dokuForm.merchant_id}
                  onChange={(e) => setDokuForm({ ...dokuForm, merchant_id: e.target.value })}
                  autoComplete="off"
                />
              </label>
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Terminal ID</span>
                <input
                  className="input"
                  placeholder="T001"
                  value={dokuForm.terminal_id}
                  onChange={(e) => setDokuForm({ ...dokuForm, terminal_id: e.target.value })}
                  autoComplete="off"
                />
              </label>
              <label className="grid gap-1 text-sm">
                <span className="text-[var(--muted)]">Kode pos</span>
                <input
                  className="input"
                  placeholder="28111"
                  value={dokuForm.postal_code}
                  onChange={(e) => setDokuForm({ ...dokuForm, postal_code: e.target.value })}
                  autoComplete="off"
                />
              </label>
            </div>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Masa berlaku invoice (TTL)</span>
              <input
                className="input"
                type="number"
                min={1}
                max={1440}
                step={1}
                value={dokuForm.expires_in_minutes}
                onChange={(e) =>
                  setDokuForm({
                    ...dokuForm,
                    expires_in_minutes: Number(e.target.value) || 60,
                  })
                }
              />
              <div className="flex flex-wrap gap-1">
                {TTL_PRESETS.map((p) => (
                  <button
                    key={`doku-${p.minutes}`}
                    type="button"
                    className="btn-ghost px-2 py-1 text-[11px]"
                    onClick={() => setDokuForm({ ...dokuForm, expires_in_minutes: p.minutes })}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
              <span className="text-[11px] text-[var(--muted)]">
                Invoice DOKU berlaku {ttlHint(dokuForm.expires_in_minutes)}.
              </span>
            </label>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Callback URL DOKU · /api/webhooks/payment/doku</span>
              <div className="flex gap-2">
                <input className="input min-w-0 flex-1 font-mono text-xs" readOnly value={dokuWebhookURL} />
                <IconButton label="Salin callback URL" onClick={() => void copyWebhook(dokuWebhookURL)}>
                  <IconCopy />
                </IconButton>
              </div>
              <span className="text-[11px] text-[var(--muted)]">
                Daftarkan URL ini sebagai Notification URL di dashboard DOKU (per channel). Domain harus publik.
                {dokuQ.data?.qr_enabled ? "" : " QRIS langsung butuh private key + merchant/terminal/kode pos."}
              </span>
            </label>
            <button type="button" className="btn w-fit" disabled={saveDoku.isPending} onClick={() => saveDoku.mutate()}>
              {saveDoku.isPending ? "Menyimpan..." : "Simpan"}
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

type WADevice = { device_id: string; label: string; priority?: number };
type WhatsAppIntegration = {
  configured: boolean;
  enabled: boolean;
  base_url: string;
  username: string;
  password?: string;
  devices: WADevice[];
  bot_enabled: boolean;
  bot_device_id?: string;
  bot_name?: string;
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
    bot_enabled: false,
    bot_device_id: "",
  });
  const [checks, setChecks] = useState<WACheckRow[]>([]);

  useEffect(() => {
    if (!q.data) return;
    const devices = Array.isArray(q.data.devices) && q.data.devices.length ? q.data.devices : [{ device_id: "", label: "" }];
    setForm({
      enabled: q.data.enabled,
      base_url: q.data.base_url || "",
      username: q.data.username || "",
      password: q.data.password || "",
      devices: [...devices].sort((a, b) => (a.priority ?? 0) - (b.priority ?? 0)),
      bot_enabled: q.data.bot_enabled,
      bot_device_id: q.data.bot_device_id || "",
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
            .map((d, i) => ({ device_id: d.device_id.trim(), label: d.label.trim(), priority: i }))
            .filter((d) => d.device_id !== ""),
          bot_enabled: form.bot_enabled,
          bot_device_id: form.bot_device_id.trim() || undefined,
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
  function moveDevice(i: number, dir: -1 | 1) {
    setForm((f) => {
      const j = i + dir;
      if (j < 0 || j >= f.devices.length) return f;
      const next = [...f.devices];
      [next[i], next[j]] = [next[j], next[i]];
      return { ...f, devices: next };
    });
  }

  return (
    <Section title="WhatsApp (gateway eksternal)">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Kirim WhatsApp lewat gateway GOWA (go-whatsapp-web-multidevice) sendiri. Bisa lebih dari satu nomor
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
                  <div className="flex items-center gap-1">
                    <span
                      className="inline-flex h-7 w-7 items-center justify-center rounded-md border text-xs font-semibold"
                      style={
                        i === 0
                          ? { borderColor: "var(--accent)", color: "var(--accent)" }
                          : { borderColor: "var(--border)", color: "var(--muted)" }
                      }
                      title={i === 0 ? "Prioritas utama" : `Prioritas ${i + 1}`}
                    >
                      {i + 1}
                    </span>
                    <button
                      type="button"
                      className="btn-ghost"
                      style={{ padding: "0.2rem 0.45rem", fontSize: "0.75rem", lineHeight: 1 }}
                      title="Naikkan prioritas"
                      aria-label="Naikkan prioritas"
                      disabled={i === 0}
                      onClick={() => moveDevice(i, -1)}
                    >
                      ↑
                    </button>
                    <button
                      type="button"
                      className="btn-ghost"
                      style={{ padding: "0.2rem 0.45rem", fontSize: "0.75rem", lineHeight: 1 }}
                      title="Turunkan prioritas"
                      aria-label="Turunkan prioritas"
                      disabled={i === form.devices.length - 1}
                      onClick={() => moveDevice(i, 1)}
                    >
                      ↓
                    </button>
                  </div>
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
            <p className="mt-1 text-xs text-[var(--muted)]">
              Urutan = prioritas kirim: nomor 1 dicoba lebih dulu; jika gagal, lanjut ke nomor berikutnya.
            </p>
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

          <div className="rounded-xl border border-[var(--border)] p-3">
            <div className="mb-2 flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">
                  Bot WhatsApp pelanggan <code className="rounded bg-[var(--panel-muted)] px-1.5 py-0.5 font-mono text-[11px]">{q.data?.bot_name || "wabot"}</code>
                </p>
                <p className="text-[10px] text-[var(--muted)]">
                  Jawab <code>/tagihan</code> · <code>/link</code> · <code>/qris</code> bila ada tagihan berjalan
                </p>
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={form.bot_enabled}
                  onChange={(e) => setForm({ ...form, bot_enabled: e.target.checked })}
                />
                Aktif
              </label>
            </div>
            <label className="grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Nomor bot (device ID)</span>
              <select
                className="input"
                value={form.bot_device_id}
                onChange={(e) => setForm({ ...form, bot_device_id: e.target.value })}
              >
                <option value="">— Nomor pertama / default —</option>
                {form.devices
                  .map((d) => d.device_id.trim())
                  .filter((id) => id !== "")
                  .map((id) => {
                    const dev = form.devices.find((d) => d.device_id.trim() === id);
                    const label = dev?.label?.trim();
                    return (
                      <option key={id} value={id}>
                        {label ? `${id} · ${label}` : id}
                      </option>
                    );
                  })}
              </select>
              <span className="text-[11px] text-[var(--muted)]">
                Bot hanya membalas dari nomor ini. Di dashboard gateway, arahkan webhook device ini ke URL di bawah
                dengan event <code>message</code>.
              </span>
            </label>
            <label className="mt-2 grid gap-1 text-sm">
              <span className="text-[var(--muted)]">Webhook URL bot</span>
              <div className="flex gap-2">
                <input
                  className="input min-w-0 flex-1 font-mono text-xs"
                  readOnly
                  value={typeof window === "undefined" ? "/api/webhooks/whatsapp" : `${window.location.origin}/api/webhooks/whatsapp`}
                />
                <IconButton
                  label="Salin webhook URL"
                  onClick={() => {
                    const url = `${window.location.origin}/api/webhooks/whatsapp`;
                    void navigator.clipboard
                      .writeText(url)
                      .then(() => toastSuccess("Webhook URL disalin"))
                      .catch(() => toastError("Gagal menyalin. Salin manual dari kolom URL."));
                  }}
                >
                  <IconCopy />
                </IconButton>
              </div>
            </label>
            <ul className="mt-2 space-y-1 text-[11px] leading-relaxed text-[var(--muted)]">
              <li><code>/tagihan</code> — kirim PDF tagihan berjalan.</li>
              <li><code>/link</code> — kirim link bayar online.</li>
              <li><code>/qris</code> — kirim QR bayar langsung (fallback ke link bila QR tidak tersedia).</li>
              <li>Perintah lain tidak dibalas. Tanpa tagihan berjalan, bot membalas info lunas/belum terbit.</li>
            </ul>
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
                <p className="text-sm font-medium">Bot + chat</p>
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
      void toastSuccess("SMTP disimpan");
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
    <Section title="Email (SMTP)">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Server SMTP untuk notifikasi email (laporan bulanan, broadcast, dunning). Port 587 memakai
        STARTTLS; port 465 memakai TLS langsung. Password kosong saat simpan = tetap memakai yang tersimpan. Jika
        nonaktif, sistem memakai SMTP dari env bila ada.
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
                      ? "belum diisi · fallback SMTP env"
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

