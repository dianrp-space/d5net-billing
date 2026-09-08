import { useEffect, useState } from "react";
import { api } from "./api";
import { IconQrCode } from "./icons";
import {
  getSavedPayMethod,
  invoiceRemaining,
  PAY_METHOD_QRIS,
  PORTAL_PAY_METHODS,
  setSavedPayMethod,
  type PayableInvoice,
  type PayMethodId,
} from "./payMethod";
import { QrisPayDialog, type QrisIntent } from "./QrisPayDialog";
import { toastError } from "./swal";
import { formatRp, FormDialog } from "./ui";

function methodIcon(id: PayMethodId) {
  switch (id) {
    case PAY_METHOD_QRIS:
      return <IconQrCode />;
    default:
      return <IconQrCode />;
  }
}

export function PayMethodDialog({
  open,
  invoiceNumber,
  amount,
  tenantSlug,
  busy,
  error,
  onClose,
  onConfirm,
}: {
  open: boolean;
  invoiceNumber: string;
  amount: number;
  tenantSlug?: string;
  busy?: boolean;
  error?: string;
  onClose: () => void;
  onConfirm: (method: PayMethodId) => void;
}) {
  const [method, setMethod] = useState<PayMethodId>(() => getSavedPayMethod(tenantSlug));

  useEffect(() => {
    if (open) setMethod(getSavedPayMethod(tenantSlug));
  }, [open, tenantSlug]);

  return (
    <FormDialog open={open} title="Pilih metode pembayaran" onClose={onClose}>
      <div className="grid gap-3">
        <div>
          <p className="text-xs text-[var(--muted)]">{invoiceNumber || "Tagihan"}</p>
          <p className="text-lg font-bold">{formatRp(amount)}</p>
        </div>
        <div className="grid gap-2" role="radiogroup" aria-label="Metode pembayaran">
          {PORTAL_PAY_METHODS.map((m) => {
            const selected = method === m.id;
            return (
              <button
                key={m.id}
                type="button"
                role="radio"
                aria-checked={selected}
                disabled={busy}
                onClick={() => setMethod(m.id)}
                className={`flex w-full items-start gap-3 rounded-xl border p-3 text-left transition-colors ${
                  selected
                    ? "border-[var(--accent)] bg-[color-mix(in_srgb,var(--accent)_10%,transparent)]"
                    : "border-[var(--border)] bg-[var(--panel)] hover:border-[var(--border-strong)]"
                }`}
              >
                <span className="mt-0.5 text-[var(--accent)]">{methodIcon(m.id)}</span>
                <span className="min-w-0 flex-1">
                  <span className="block text-sm font-semibold">{m.label}</span>
                  <span className="mt-0.5 block text-xs text-[var(--muted)]">{m.description}</span>
                </span>
              </button>
            );
          })}
        </div>
        {error ? <p className="text-sm text-[var(--danger)]">{error}</p> : null}
        <button
          type="button"
          className="btn w-fit"
          disabled={busy}
          onClick={() => onConfirm(method)}
        >
          {busy ? "Menyiapkan…" : "Lanjut bayar"}
        </button>
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

  useEffect(() => {
    setStep("method");
    setIntent(null);
    setError("");
    setBusy(false);
  }, [invoice?.id]);

  if (!invoice?.id) return null;

  async function confirmMethod(method: PayMethodId) {
    if (!invoice?.id) return;
    setSavedPayMethod(method, tenantSlug);
    if (method !== PAY_METHOD_QRIS) {
      setError("Metode ini belum tersedia.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const next = await api<QrisIntent>(`/api/portal/invoices/${invoice.id}/checkout`, {
        method: "POST",
        headers,
      });
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

  const amount = invoiceRemaining(invoice);

  return (
    <>
      <PayMethodDialog
        open={step === "method"}
        invoiceNumber={invoice.invoice_number}
        amount={amount}
        tenantSlug={tenantSlug}
        busy={busy}
        error={error}
        onClose={onClose}
        onConfirm={(m) => void confirmMethod(m)}
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
