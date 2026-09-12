import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { openDuitkuPopup } from "./duitkuPop";
import { IconExternalLink, IconQrCode } from "./icons";
import {
  getSavedPayMethod,
  hasSavedPayMethod,
  invoiceRemaining,
  PAY_METHOD_DOKU,
  PAY_METHOD_DUITKU,
  PAY_METHOD_QRIS,
  payMethodToProvider,
  payOptionsToMethods,
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
  onClose,
  onConfirm,
}: {
  open: boolean;
  invoiceNumber: string;
  amount: number;
  methods: PayMethodDef[];
  loading?: boolean;
  busy?: boolean;
  error?: string;
  onClose: () => void;
  onConfirm: (method: PayMethodId) => void;
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
            {methods.length > 1 ? (
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
  const [step, setStep] = useState<"method" | "qris" | "duitku">("method");
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
  useEffect(() => {
    if (!invoice?.id || step !== "method" || busy) return;
    if (autoTriedFor.current === invoice.id) return;
    if (options.isLoading || !methods.length) return;
    const saved = getSavedPayMethod(tenantSlug);
    const useSaved = hasSavedPayMethod(tenantSlug) && methods.some((m) => m.id === saved);
    if (!useSaved && methods.length !== 1) return;
    autoTriedFor.current = invoice.id;
    void confirmMethod(useSaved ? saved : methods[0].id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [invoice?.id, step, busy, options.isLoading, methods.length, tenantSlug]);

  if (!invoice?.id) return null;

  async function confirmMethod(method: PayMethodId) {
    if (!invoice?.id) return;
    setSavedPayMethod(method, tenantSlug);
    setBusy(true);
    setError("");
    // Halaman hosted (DOKU) tidak punya popup SDK. Buka tab kosong secara
    // sinkron dari klik user agar tidak diblokir popup-blocker, lalu arahkan
    // ke halaman PG begitu intent selesai dibuat.
    let popup: Window | null = null;
    if (method === PAY_METHOD_DOKU && typeof window !== "undefined") {
      try {
        popup = window.open("about:blank", "_blank");
      } catch {
        popup = null;
      }
    }
    try {
      const next = await api<QrisIntent>(`/api/portal/invoices/${invoice.id}/checkout`, {
        method: "POST",
        headers,
        body: JSON.stringify({
          provider: payMethodToProvider(method),
          return_url: typeof window !== "undefined" ? window.location.href : "",
        }),
      });
      if (method === PAY_METHOD_DUITKU) {
        await launchDuitku(next);
        return;
      }
      if (method === PAY_METHOD_DOKU) {
        const url = String(next.checkout_url || "").trim();
        if (popup && !popup.closed) {
          if (url) {
            popup.location.href = url;
            try {
              popup.opener = null;
            } catch {
              /* abaikan */
            }
          } else {
            void popup.close();
          }
        } else if (url) {
          window.open(url, "_blank", "noopener,noreferrer");
        }
      }
      setIntent(next);
      setStep("qris");
    } catch (err: unknown) {
      if (popup && !popup.closed) void popup.close();
      const msg = err instanceof Error ? err.message : "Gagal membuat pembayaran";
      setError(msg);
      void toastError(msg);
    } finally {
      setBusy(false);
    }
  }

  // Duitku POP renders its own overlay popup. Our Radix dialogs are modal and
  // block outside pointer events, so we close them first and hand control to the
  // Duitku SDK (checkout.process). Falls back to the hosted paymentUrl.
  async function launchDuitku(next: QrisIntent) {
    const meta = (next.metadata ?? {}) as Record<string, unknown>;
    const reference = String(meta.reference || meta.transaction_id || "").trim();
    const sandbox = meta.duitku_sandbox === true;
    const url = String(next.checkout_url || "").trim();
    setStep("duitku"); // hide our modals so the Duitku popup is clickable
    if (!reference) {
      if (url) window.open(url, "_blank", "noopener,noreferrer");
      onClose();
      return;
    }
    try {
      await openDuitkuPopup(reference, sandbox, {
        onSuccess: () => {
          // #region agent log
          fetch("http://127.0.0.1:7813/ingest/d5ceb638-f02e-4b79-b49c-b843ba23dc69", {
            method: "POST",
            headers: { "Content-Type": "application/json", "X-Debug-Session-Id": "f19e18" },
            body: JSON.stringify({
              sessionId: "f19e18",
              runId: "pre-fix",
              hypothesisId: "F",
              location: "PayMethodDialog.tsx:duitkuOnSuccess",
              message: "Duitku popup onSuccess fired (UI only, no RecordPayment)",
              data: { invoice: invoice.invoice_number, invoiceId: invoice.id },
              timestamp: Date.now(),
            }),
          }).catch(() => {});
          // #endregion
          onPaid?.();
        },
        onError: () => void toastError("Pembayaran Duitku gagal atau dibatalkan."),
        onClose: () => onClose(),
      });
    } catch {
      if (url) window.open(url, "_blank", "noopener,noreferrer");
      onClose();
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
