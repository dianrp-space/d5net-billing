import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { IconExternalLink, IconQrCode } from "./icons";
import {
  getSavedPayMethod,
  hasSavedPayMethod,
  invoiceRemaining,
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
            {methods.length > 1 || sandboxAvailable ? (
              <p className="text-xs text-[var(--muted)]">Klik salah satu untuk langsung bayar:</p>
            ) : (
              <p className="text-xs text-[var(--muted)]">Menyiapkan pembayaran…</p>
            )}
            {methods.map((m) => (
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
                </span>
                <span className="shrink-0 text-[var(--muted)]" aria-hidden>
                  →
                </span>
              </button>
            ))}
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
  const [step, setStep] = useState<"method" | "qris">("method");
  const [intent, setIntent] = useState<QrisIntent | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  // Skip the picker straight to checkout when a preferred method exists.
  const autoTriedFor = useRef<string | null>(null);
  const options = useQuery({
    queryKey: ["portal-pay-options", tenantSlug],
    queryFn: () => api<PayOption[]>("/api/portal/pay-options", { headers }),
    enabled: Boolean(invoice?.id),
  });
  const methods = payOptionsToMethods(options.data);
  const sandboxAvailable = payOptionsHasDuitkuSandbox(options.data);

  useEffect(() => {
    setStep("method");
    setIntent(null);
    setError("");
    setBusy(false);
    autoTriedFor.current = null;
  }, [invoice?.id]);

  // Langsung bayar tanpa konfirmasi tambahan: 1 PG aktif → otomatis jalan;
  // beberapa PG + ada metode tersimpan → pakai yang tersimpan. Selain itu
  // tampilkan opsi, dan setiap opsi yang diklik langsung memproses bayar.
  // Sandbox Duitku: jangan auto-redirect supaya tester bisa tandai lunas lokal.
  useEffect(() => {
    if (!invoice?.id || step !== "method" || busy) return;
    if (autoTriedFor.current === invoice.id) return;
    if (options.isLoading || !methods.length || sandboxAvailable) return;
    const saved = getSavedPayMethod(tenantSlug);
    const useSaved = hasSavedPayMethod(tenantSlug) && methods.some((m) => m.id === saved);
    if (!useSaved && methods.length !== 1) return;
    autoTriedFor.current = invoice.id;
    void confirmMethod(useSaved ? saved : methods[0].id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [invoice?.id, step, busy, options.isLoading, methods.length, tenantSlug, sandboxAvailable]);

  if (!invoice?.id) return null;

  async function confirmMethod(method: PayMethodId) {
    if (!invoice?.id) return;
    setSavedPayMethod(method, tenantSlug);
    setBusy(true);
    setError("");
    try {
      const next = await api<QrisIntent>(`/api/portal/invoices/${invoice.id}/checkout`, {
        method: "POST",
        headers,
        body: JSON.stringify({
          provider: payMethodToProvider(method),
          return_url: portalPaymentReturnURL(),
        }),
      });
      if (method === PAY_METHOD_DUITKU || method === PAY_METHOD_DOKU) {
        // Halaman penuh di tab yang sama (seperti DOKU) agar background/custom
        // yang dipasang di dashboard PG terlihat. Kembali via return_url
        // (?payment=success) lalu dialog sukses tampil otomatis.
        const url = String(next.checkout_url || "").trim();
        if (!url) {
          throw new Error(`Link pembayaran ${method === PAY_METHOD_DOKU ? "DOKU" : "Duitku"} kosong`);
        }
        window.location.assign(url);
        return;
      }
      setIntent(next);
      setStep("qris");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Gagal membuat pembayaran";
      setError(msg);
      void toastError(msg);
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
        onConfirm={(m) => void confirmMethod(m)}
        onSandboxPay={() => void confirmSandboxPay()}
      />
      <QrisPayDialog
        open={step === "qris"}
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
