import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { useAppDialog } from "./confirm";
import { IconTrash } from "./icons";
import { toastError, toastSuccess } from "./swal";
import { FormDialog, IconButton, Section, SecretInput, Table } from "./ui";

type OutboundWebhook = {
  id: string;
  url: string;
  secret?: string | null;
  events: string[];
  is_active: boolean;
};

type PaymentIntegration = {
  midtrans_configured: boolean;
  xendit_configured: boolean;
  tripay_configured: boolean;
  midtrans_enabled: boolean;
  xendit_enabled: boolean;
  tripay_enabled: boolean;
  tripay_merchant_code: string;
  webhook_base_hint: string;
};

const EVENT_PRESETS = ["payment.paid", "invoice.created", "subscription.suspended", "subscription.activated"];

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
  const [form, setForm] = useState({
    midtrans_enabled: false,
    xendit_enabled: false,
    tripay_enabled: false,
    tripay_merchant_code: "",
    midtrans_server_key: "",
    xendit_secret_key: "",
    tripay_private_key: "",
  });

  useEffect(() => {
    if (!q.data) return;
    setForm({
      midtrans_enabled: q.data.midtrans_enabled,
      xendit_enabled: q.data.xendit_enabled,
      tripay_enabled: q.data.tripay_enabled,
      tripay_merchant_code: q.data.tripay_merchant_code || "",
      midtrans_server_key: "",
      xendit_secret_key: "",
      tripay_private_key: "",
    });
  }, [q.data]);

  const save = useMutation({
    mutationFn: () =>
      api<PaymentIntegration>("/api/integrations/payment", {
        method: "PUT",
        body: JSON.stringify({
          midtrans_enabled: form.midtrans_enabled,
          xendit_enabled: form.xendit_enabled,
          tripay_enabled: form.tripay_enabled,
          tripay_merchant_code: form.tripay_merchant_code.trim(),
          midtrans_server_key: form.midtrans_server_key.trim() || undefined,
          xendit_secret_key: form.xendit_secret_key.trim() || undefined,
          tripay_private_key: form.tripay_private_key.trim() || undefined,
        }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["integration-payment"] });
      setForm((f) => ({
        ...f,
        midtrans_server_key: "",
        xendit_secret_key: "",
        tripay_private_key: "",
      }));
      void toastSuccess("Payment gateway disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  return (
    <Section title="Payment Gateway">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Kredensial per tenant (terenkripsi). Kosongkan field secret untuk mempertahankan nilai lama. Webhook masuk:{" "}
        <code className="text-xs">{q.data?.webhook_base_hint || "/api/webhooks/payment/{provider}"}</code>
      </p>
      {q.isLoading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <div className="grid max-w-xl gap-4">
          <ProviderBlock
            title="Midtrans"
            enabled={form.midtrans_enabled}
            configured={Boolean(q.data?.midtrans_configured)}
            onToggle={(v) => setForm({ ...form, midtrans_enabled: v })}
          >
            <SecretInput
              placeholder={q.data?.midtrans_configured ? "Server key baru (opsional)" : "Server key"}
              value={form.midtrans_server_key}
              onChange={(e) => setForm({ ...form, midtrans_server_key: e.target.value })}
              autoComplete="off"
            />
          </ProviderBlock>
          <ProviderBlock
            title="Xendit"
            enabled={form.xendit_enabled}
            configured={Boolean(q.data?.xendit_configured)}
            onToggle={(v) => setForm({ ...form, xendit_enabled: v })}
          >
            <SecretInput
              placeholder={q.data?.xendit_configured ? "Secret key baru (opsional)" : "Secret key"}
              value={form.xendit_secret_key}
              onChange={(e) => setForm({ ...form, xendit_secret_key: e.target.value })}
              autoComplete="off"
            />
          </ProviderBlock>
          <ProviderBlock
            title="Tripay"
            enabled={form.tripay_enabled}
            configured={Boolean(q.data?.tripay_configured)}
            onToggle={(v) => setForm({ ...form, tripay_enabled: v })}
          >
            <input
              className="input"
              placeholder="Merchant code"
              value={form.tripay_merchant_code}
              onChange={(e) => setForm({ ...form, tripay_merchant_code: e.target.value })}
            />
            <SecretInput
              placeholder={q.data?.tripay_configured ? "Private key baru (opsional)" : "Private key"}
              value={form.tripay_private_key}
              onChange={(e) => setForm({ ...form, tripay_private_key: e.target.value })}
              autoComplete="off"
            />
          </ProviderBlock>
          <button type="button" className="btn w-fit" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? "Menyimpan..." : "Simpan"}
          </button>
        </div>
      )}
    </Section>
  );
}

function ProviderBlock({
  title,
  enabled,
  configured,
  onToggle,
  children,
}: {
  title: string;
  enabled: boolean;
  configured: boolean;
  onToggle: (v: boolean) => void;
  children: ReactNode;
}) {
  return (
    <div className="rounded-xl border border-[var(--border)] p-3">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <p className="text-sm font-medium">{title}</p>
          <p className="text-[10px] text-[var(--muted)]">{configured ? "kunci tersimpan" : "belum dikonfigurasi"}</p>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => onToggle(e.target.checked)} />
          Aktif
        </label>
      </div>
      <div className="grid gap-2">{children}</div>
    </div>
  );
}

export function MessagingGWPage() {
  const [tab, setTab] = useState<"whatsapp" | "telegram">("whatsapp");

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-2">
        <button type="button" className={tab === "whatsapp" ? "btn" : "btn-ghost"} onClick={() => setTab("whatsapp")}>
          WhatsApp
        </button>
        <button type="button" className={tab === "telegram" ? "btn" : "btn-ghost"} onClick={() => setTab("telegram")}>
          Telegram
        </button>
      </div>
      {tab === "whatsapp" ? <WhatsAppTab /> : <TelegramTab />}
    </div>
  );
}

type WAStatus = {
  enabled: boolean;
  connected: boolean;
  logged_in: boolean;
  jid?: string;
  phone?: string;
  qr_code?: string;
  qr_event?: string;
  qr_image_base64?: string;
};

function WhatsAppTab() {
  const qc = useQueryClient();
  const status = useQuery({
    queryKey: ["integration-whatsapp"],
    queryFn: () => api<WAStatus>("/api/integrations/whatsapp/status"),
    refetchInterval: (q) => {
      const d = q.state.data;
      if (d?.logged_in) return false;
      if (d?.qr_code) return 2500;
      return 8000;
    },
  });

  const connect = useMutation({
    mutationFn: () => api<WAStatus>("/api/integrations/whatsapp/connect", { method: "POST", body: "{}" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["integration-whatsapp"] });
      void toastSuccess("Sesi WhatsApp dimulai — scan QR jika diminta");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const logout = useMutation({
    mutationFn: () => api<WAStatus>("/api/integrations/whatsapp/logout", { method: "POST", body: "{}" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["integration-whatsapp"] });
      void toastSuccess("WhatsApp logout");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const st = status.data;

  return (
    <Section title="WhatsApp (whatsmeow)">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Pairing multi-device via QR (library <code className="text-xs">go.mau.fi/whatsmeow</code>). Setelah login,
        notifikasi tenant dikirim lewat sesi ini.
      </p>
      {status.isLoading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <div className="grid max-w-xl gap-4">
          <div className="rounded-xl border border-[var(--border)] p-3 text-sm">
            <p>
              Status:{" "}
              <strong>
                {st?.logged_in ? "terhubung" : st?.qr_code ? "menunggu scan QR" : "belum login"}
              </strong>
            </p>
            <p className="text-[var(--muted)]">
              Connected: {st?.connected ? "ya" : "tidak"}
              {st?.phone ? ` · ${st.phone}` : ""}
            </p>
            {st?.qr_event ? <p className="text-xs text-[var(--muted)]">Event: {st.qr_event}</p> : null}
          </div>
          {st?.qr_image_base64 && !st.logged_in ? (
            <div className="rounded-xl border border-[var(--border)] p-4">
              <p className="mb-2 text-sm font-medium">Scan QR dari WhatsApp → Perangkat tertaut</p>
              <img src={st.qr_image_base64} alt="QR WhatsApp" className="mx-auto h-56 w-56 rounded-lg bg-white p-2" />
            </div>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <button type="button" className="btn" disabled={connect.isPending} onClick={() => connect.mutate()}>
              {st?.logged_in ? "Reconnect" : "Connect / Tampilkan QR"}
            </button>
            <button
              type="button"
              className="btn-ghost"
              disabled={logout.isPending || (!st?.logged_in && !st?.qr_code)}
              onClick={() => logout.mutate()}
            >
              Logout
            </button>
          </div>
        </div>
      )}
    </Section>
  );
}

function TelegramTab() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["integration-telegram"],
    queryFn: () =>
      api<{
        telegram_configured: boolean;
        telegram_enabled: boolean;
        telegram_chat_id: string;
      }>("/api/integrations/telegram"),
  });
  const [form, setForm] = useState({
    telegram_bot_token: "",
    telegram_chat_id: "",
    telegram_enabled: false,
  });

  useEffect(() => {
    if (!q.data) return;
    setForm({
      telegram_bot_token: "",
      telegram_chat_id: q.data.telegram_chat_id || "",
      telegram_enabled: q.data.telegram_enabled,
    });
  }, [q.data]);

  const save = useMutation({
    mutationFn: () =>
      api("/api/integrations/telegram", {
        method: "PUT",
        body: JSON.stringify({
          telegram_enabled: form.telegram_enabled,
          telegram_chat_id: form.telegram_chat_id.trim(),
          telegram_bot_token: form.telegram_bot_token.trim() || undefined,
        }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["integration-telegram"] });
      setForm((f) => ({ ...f, telegram_bot_token: "" }));
      void toastSuccess("Telegram Gateway disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  return (
    <Section title="Telegram Gateway">
      <p className="mb-4 text-sm text-[var(--muted)]">
        Alert ops tenant saja (bukan ke pelanggan). Isi bot token + chat ID grup/channel/DM admin. Start bot di chat
        tujuan dulu, lalu salin chat ID. Kosongkan token untuk mempertahankan nilai lama.
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
                  {q.data?.telegram_configured ? "siap kirim" : "butuh token + chat ID"}
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
              <SecretInput
                placeholder={q.data?.telegram_configured ? "Bot token baru (opsional)" : "Bot token"}
                value={form.telegram_bot_token}
                onChange={(e) => setForm({ ...form, telegram_bot_token: e.target.value })}
                autoComplete="off"
              />
              <input
                className="input"
                placeholder="Chat ID (contoh: -100123… atau 123456789)"
                value={form.telegram_chat_id}
                onChange={(e) => setForm({ ...form, telegram_chat_id: e.target.value })}
              />
            </div>
          </div>
          <button type="button" className="btn w-fit" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? "Menyimpan..." : "Simpan"}
          </button>
        </div>
      )}
    </Section>
  );
}

