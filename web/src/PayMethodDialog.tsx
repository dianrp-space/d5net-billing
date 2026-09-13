import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { IconExternalLink, IconQrCode } from "./icons";
import {
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
  type PayableInvoice,
  type PayMethodDef,
  type PayMethodId,
  type PayOption,
} from "./payMethod";
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
              const fee = methodCustomerFee(m, amount);
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
                    <span className="mt-0.5 block text-xs text-[var(--muted)]">{m.description}</span>
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
  const [step, setStep] = useState<"method" | "busy">("method");
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

  useEffect(() => {
    setStep("method");
    setError("");
    setBusy(false);
    autoTriedFor.current = null;
  }, [invoice?.id]);

  // Auto-bayar bila 1 gateway (bukan fee / sandbox).
  useEffect(() => {
    if (!invoice?.id || step !== "method" || busy) return;
    if (autoTriedFor.current === invoice.id) return;
    if (options.isLoading || !methods.length || sandboxAvailable || hasCustomerFee) return;
    const saved = getSavedPayMethod(tenantSlug);
    const useSaved = hasSavedPayMethod(tenantSlug) && methods.some((m) => m.id === saved);
    if (!useSaved && methods.length !== 1) return;
    const pick = useSaved ? saved : methods[0].id;
    autoTriedFor.current = invoice.id;
    void confirmMethod(pick);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [invoice?.id, step, busy, options.isLoading, methods.length, tenantSlug, sandboxAvailable, hasCustomerFee]);

  if (!invoice?.id) return null;

  async function confirmMethod(method: PayMethodId) {
    if (!invoice?.id) return;
    setSavedPayMethod(method, tenantSlug);
    setBusy(true);
    setError("");
    setStep("busy");
    try {
      const next = await api<{ checkout_url?: string }>(`/api/portal/invoices/${invoice.id}/checkout`, {
        method: "POST",
        headers,
        body: JSON.stringify({
          provider: payMethodToProvider(method),
          return_url: portalPaymentReturnURL(),
        }),
      });
      const url = String(next.checkout_url || "").trim();
      if (!url) throw new Error("Link pembayaran kosong");
      // Sama seperti Duitku: redirect di tab yang sama (bukan tab baru).
      window.location.assign(url);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Gagal membuat pembayaran";
      setError(msg);
      void toastError(msg);
      setStep("method");
    } finally {
      setBusy(false);
    }
  }

  async function confirmSandboxPay() {
    if (!invoice?.id) return;
    setBusy(true);
    setError("");
    try {
      await api(`/api/portal/invoices/${invoice.id}/sandbox-pay`, {
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
    <PayMethodDialog
      open={step === "method" || step === "busy"}
      invoiceNumber={invoice.invoice_number}
      amount={amount}
      methods={methods}
      loading={options.isLoading}
      busy={busy}
      error={error || (options.isError ? "Gagal memuat metode pembayaran" : "")}
      sandboxAvailable={sandboxAvailable}
      onClose={onClose}
      onConfirm={(m) => void confirmMethod(m)}
      onSandboxPay={() => void confirmSandboxPay()}
    />
  );
}
