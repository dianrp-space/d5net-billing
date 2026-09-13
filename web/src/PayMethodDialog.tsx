import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { IconExternalLink, IconQrCode } from "./icons";
import {
  channelCustomerFee,
  dokuChannelsFromMethod,
  getSavedPayMethod,
  hasSavedPayMethod,
  invoiceRemaining,
  methodCustomerFee,
  payMethodsHaveCustomerFee,
  PAY_METHOD_DOKU,
  PAY_METHOD_DUITKU,
  PAY_METHOD_QRIS,
  payMethodToProvider,
  payOptionsHasDuitkuSandbox,
  payOptionsToMethods,
  portalPaymentReturnURL,
  setSavedPayMethod,
  type DokuChannelOption,
  type PayableInvoice,
  type PayMethodDef,
  type PayMethodId,
  type PayOption,
} from "./payMethod";
import { QrisPayDialog, type QrisIntent } from "./QrisPayDialog";
import { toastError } from "./swal";
import { formatRp, FormDialog } from "./ui";

function methodIcon(id: PayMethodId) {
  switch (id) {
    case PAY_METHOD_DUITKU:
    case PAY_METHOD_DOKU:
      return <IconExternalLink />;
    case PAY_METHOD_QRIS:
    default:
      return <IconQrCode />;
  }
}

function feeBreakdown(base: number, fee: number) {
  if (fee <= 0) return null;
  return (
    <span className="mt-1 block rounded-md bg-[var(--panel-muted,rgba(0,0,0,0.04))] px-2 py-1 text-[11px] leading-relaxed text-[var(--muted)]">
      Tagihan {formatRp(base)} + biaya admin {formatRp(fee)} ={" "}
      <strong className="text-[var(--fg,inherit)]">{formatRp(base + fee)}</strong>
    </span>
  );
}

export function PayMethodDialog({
  open,
  invoiceNumber,
  amount,
  methods,
  loading,
  busy,
  error,
  sandboxAvailable,
  onClose,
  onConfirm,
  onSandboxPay,
}: {
  open: boolean;
  invoiceNumber: string;
  amount: number;
  methods: PayMethodDef[];
  loading?: boolean;
  busy?: boolean;
  error?: string;
  sandboxAvailable?: boolean;
  onClose: () => void;
  onConfirm: (method: PayMethodId) => void;
  onSandboxPay?: () => void;
}) {
  return (
    <FormDialog open={open} title="Pilih metode pembayaran" onClose={onClose}>
      <div className="grid gap-3">
        <div>
          <p className="text-xs text-[var(--muted)]">{invoiceNumber || "Tagihan"}</p>
          <p className="text-lg font-bold">{formatRp(amount)}</p>
        </div>
        {loading ? (
          <p className="text-sm text-[var(--muted)]">Memuat metode pembayaran…</p>
        ) : methods.length === 0 ? (
          <p className="text-sm text-[var(--danger)]">Belum ada payment gateway yang aktif. Hubungi admin.</p>
        ) : (
          <div className="grid gap-2" role="list" aria-label="Metode pembayaran">
            {methods.length > 1 || sandboxAvailable || methods.some((m) => methodCustomerFee(m, amount) > 0) ? (
              <p className="text-xs text-[var(--muted)]">Klik salah satu untuk langsung bayar:</p>
            ) : (
              <p className="text-xs text-[var(--muted)]">Menyiapkan pembayaran…</p>
            )}
            {methods.map((m) => {
              const channels = dokuChannelsFromMethod(m);
              const isDokuDirect = m.id === PAY_METHOD_DOKU && channels.length > 0;
              const fee = isDokuDirect ? 0 : methodCustomerFee(m, amount);
              return (
                <button
                  key={m.id}
                  type="button"
                  role="listitem"
                  disabled={busy}
                  onClick={() => onConfirm(m.id)}
                  className="flex w-full items-center gap-3 rounded-xl border border-[var(--border)] bg-[var(--panel)] p-3 text-left transition-colors hover:border-[var(--accent)] disabled:opacity-60"
                >
                  <span className="mt-0.5 text-[var(--accent)]">{methodIcon(m.id)}</span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-semibold">{m.label}</span>
                    <span className="mt-0.5 block text-xs text-[var(--muted)]">
                      {isDokuDirect ? `${channels.length} channel · pilih di langkah berikutnya` : m.description}
                    </span>
                    {feeBreakdown(amount, fee)}
                  </span>
                  <span className="shrink-0 text-[var(--muted)]" aria-hidden>
                    →
                  </span>
                </button>
              );
            })}
          </div>
        )}
        {sandboxAvailable && onSandboxPay ? (
          <div className="grid gap-1.5 rounded-xl border border-dashed border-[var(--border)] p-3">
            <p className="text-xs text-[var(--muted)]">
              Dashboard Duitku sandbox tidak punya tandai lunas. Tombol ini menjalankan alur webhook yang sama.
            </p>
            <button type="button" className="btn-ghost w-fit text-sm" disabled={busy} onClick={onSandboxPay}>
              Uji sandbox: tandai lunas
            </button>
          </div>
        ) : null}
        {error ? <p className="text-sm text-[var(--danger)]">{error}</p> : null}
      </div>
    </FormDialog>
  );
}

function DokuChannelDialog({
  open,
  invoiceNumber,
  amount,
  feeMode,
  channels,
  busy,
  error,
  onBack,
  onClose,
  onConfirm,
}: {
  open: boolean;
  invoiceNumber: string;
  amount: number;
  feeMode?: string;
  channels: DokuChannelOption[];
  busy?: boolean;
  error?: string;
  onBack: () => void;
  onClose: () => void;
  onConfirm: (channelId: string) => void;
}) {
  const byKind = (kind: string) => channels.filter((c) => c.kind === kind);
  const groups: { label: string; kind: string }[] = [
    { label: "QRIS", kind: "qr" },
    { label: "Virtual Account", kind: "va" },
    { label: "E-wallet", kind: "ewallet" },
    { label: "Retail", kind: "retail" },
  ];

  return (
    <FormDialog open={open} title="Pilih channel DOKU" onClose={onClose}>
      <div className="grid gap-3">
        <div>
          <p className="text-xs text-[var(--muted)]">{invoiceNumber || "Tagihan"}</p>
          <p className="text-lg font-bold">{formatRp(amount)}</p>
        </div>
        <button type="button" className="btn-ghost w-fit text-xs" disabled={busy} onClick={onBack}>
          ← Ganti gateway
        </button>
        {channels.length === 0 ? (
          <p className="text-sm text-[var(--danger)]">Belum ada channel DOKU yang aktif. Hubungi admin.</p>
        ) : (
          <div className="grid gap-3">
            {groups.map((g) => {
              const list = byKind(g.kind);
              if (!list.length) return null;
              return (
                <div key={g.kind} className="grid gap-2">
                  <p className="text-[11px] font-semibold uppercase tracking-wide text-[var(--muted)]">{g.label}</p>
                  {list.map((ch) => {
                    const fee = channelCustomerFee(feeMode, ch, amount);
                    return (
                      <button
                        key={ch.id}
                        type="button"
                        disabled={busy}
                        onClick={() => onConfirm(ch.id)}
                        className="flex w-full items-center gap-3 rounded-xl border border-[var(--border)] bg-[var(--panel)] p-3 text-left transition-colors hover:border-[var(--accent)] disabled:opacity-60"
                      >
                        <span className="min-w-0 flex-1">
                          <span className="block text-sm font-semibold">{ch.label}</span>
                          {feeBreakdown(amount, fee)}
                        </span>
                        <span className="shrink-0 text-[var(--muted)]" aria-hidden>
                          →
                        </span>
                      </button>
                    );
                  })}
                </div>
              );
            })}
          </div>
        )}
        {error ? <p className="text-sm text-[var(--danger)]">{error}</p> : null}
      </div>
    </FormDialog>
  );
}

export function PortalPayHost({
  invoice,
  tenantSlug,
  headers,
  onClose,
  onPaid,
}: {
  invoice: PayableInvoice | null;
  tenantSlug?: string;
  headers?: HeadersInit;
  onClose: () => void;
  onPaid?: () => void;
}) {
  const [step, setStep] = useState<"method" | "channel" | "pay">("method");
  const [intent, setIntent] = useState<QrisIntent | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const autoTriedFor = useRef<string | null>(null);
  const options = useQuery({
    queryKey: ["portal-pay-options", tenantSlug],
    queryFn: () => api<PayOption[]>("/api/portal/pay-options", { headers }),
    enabled: Boolean(invoice?.id),
  });
  const methods = payOptionsToMethods(options.data);
  const sandboxAvailable = payOptionsHasDuitkuSandbox(options.data);
  const invoiceAmount = invoice ? invoiceRemaining(invoice) : 0;
  const hasCustomerFee = payMethodsHaveCustomerFee(methods, invoiceAmount);
  const dokuMethod = methods.find((m) => m.id === PAY_METHOD_DOKU);
  const dokuChannels = dokuChannelsFromMethod(dokuMethod);

  useEffect(() => {
    setStep("method");
    setIntent(null);
    setError("");
    setBusy(false);
    autoTriedFor.current = null;
  }, [invoice?.id]);

  // Auto-bayar bila 1 gateway (bukan DOKU multi-channel / fee / sandbox).
  useEffect(() => {
    if (!invoice?.id || step !== "method" || busy) return;
    if (autoTriedFor.current === invoice.id) return;
    if (options.isLoading || !methods.length || sandboxAvailable || hasCustomerFee) return;
    const saved = getSavedPayMethod(tenantSlug);
    const useSaved = hasSavedPayMethod(tenantSlug) && methods.some((m) => m.id === saved);
    if (!useSaved && methods.length !== 1) return;
    const pick = useSaved ? saved : methods[0].id;
    const m = methods.find((x) => x.id === pick);
    if (pick === PAY_METHOD_DOKU && dokuChannelsFromMethod(m).length > 1) return;
    autoTriedFor.current = invoice.id;
    void selectGateway(pick);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [invoice?.id, step, busy, options.isLoading, methods.length, tenantSlug, sandboxAvailable, hasCustomerFee]);

  if (!invoice?.id) return null;

  async function selectGateway(method: PayMethodId) {
    if (!invoice?.id) return;
    setSavedPayMethod(method, tenantSlug);
    setError("");
    if (method === PAY_METHOD_DOKU) {
      const channels = dokuChannelsFromMethod(methods.find((m) => m.id === PAY_METHOD_DOKU));
      if (channels.length === 0) {
        setError("Belum ada channel DOKU yang aktif");
        void toastError("Belum ada channel DOKU yang aktif");
        return;
      }
      if (channels.length === 1 && !hasCustomerFee) {
        await checkout(method, channels[0].id);
        return;
      }
      setStep("channel");
      return;
    }
    await checkout(method);
  }

  async function checkout(method: PayMethodId, channel?: string) {
    if (!invoice?.id) return;
    setBusy(true);
    setError("");
    try {
      const next = await api<QrisIntent>(`/api/portal/invoices/${invoice.id}/checkout`, {
        method: "POST",
        headers,
        body: JSON.stringify({
          provider: payMethodToProvider(method),
          channel: channel || undefined,
          return_url: portalPaymentReturnURL(),
        }),
      });
      if (method === PAY_METHOD_DUITKU) {
        const url = String(next.checkout_url || "").trim();
        if (!url) throw new Error("Link pembayaran Duitku kosong");
        window.location.assign(url);
        return;
      }
      // DOKU Direct: tampilkan QR / VA / kode retail / atau buka e-wallet di dialog.
      setIntent(next);
      setStep("pay");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Gagal membuat pembayaran";
      setError(msg);
      void toastError(msg);
      if (method === PAY_METHOD_DOKU && channel) setStep("channel");
      else setStep("method");
    } finally {
      setBusy(false);
    }
  }

  async function confirmSandboxPay() {
    if (!invoice?.id) return;
    setBusy(true);
    setError("");
    try {
      await api<QrisIntent>(`/api/portal/invoices/${invoice.id}/sandbox-pay`, {
        method: "POST",
        headers,
      });
      onPaid?.();
      onClose();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Gagal mensimulasikan pembayaran";
      setError(msg);
      void toastError(msg);
    } finally {
      setBusy(false);
    }
  }

  const amount = invoiceRemaining(invoice);

  return (
    <>
      <PayMethodDialog
        open={step === "method"}
        invoiceNumber={invoice.invoice_number}
        amount={amount}
        methods={methods}
        loading={options.isLoading}
        busy={busy}
        error={error || (options.isError ? "Gagal memuat metode pembayaran" : "")}
        sandboxAvailable={sandboxAvailable}
        onClose={onClose}
        onConfirm={(m) => void selectGateway(m)}
        onSandboxPay={() => void confirmSandboxPay()}
      />
      <DokuChannelDialog
        open={step === "channel"}
        invoiceNumber={invoice.invoice_number}
        amount={amount}
        feeMode={dokuMethod?.feeMode}
        channels={dokuChannels}
        busy={busy}
        error={error}
        onBack={() => {
          setError("");
          setStep("method");
        }}
        onClose={onClose}
        onConfirm={(ch) => void checkout(PAY_METHOD_DOKU, ch)}
      />
      <QrisPayDialog
        open={step === "pay"}
        invoiceNumber={invoice.invoice_number}
        intent={intent}
        pollPath={`/api/portal/invoices/${invoice.id}/payment-intent`}
        cancelPath={`/api/portal/invoices/${invoice.id}/payment-intent/cancel`}
        pollHeaders={headers}
        onClose={onClose}
        onPaid={onPaid}
      />
    </>
  );
}
