import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { IconExternalLink, IconQrCode } from "./icons";
import {
  invoiceRemaining,
  methodCustomerFee,
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

const SINGLE_PG_REDIRECT_SECONDS = 3;

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
            <p className="text-xs text-[var(--muted)]">Klik salah satu untuk langsung bayar:</p>
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

/** Layar singkat sebelum redirect otomatis bila hanya 1 PG aktif (tanpa menyebut nama PG). */
function PayRedirectNotice({
  open,
  invoiceNumber,
  amount,
  fee,
  secondsLeft,
  busy,
  error,
  sandboxAvailable,
  onCancel,
  onSkipWait,
  onSandboxPay,
}: {
  open: boolean;
  invoiceNumber: string;
  amount: number;
  fee: number;
  secondsLeft: number;
  busy?: boolean;
  error?: string;
  sandboxAvailable?: boolean;
  onCancel: () => void;
  onSkipWait: () => void;
  onSandboxPay?: () => void;
}) {
  const payTotal = amount + Math.max(0, fee);
  return (
    <FormDialog open={open} title="Menuju pembayaran" onClose={onCancel}>
      <div className="grid gap-4">
        <div>
          <p className="text-xs text-[var(--muted)]">{invoiceNumber || "Tagihan"}</p>
          <p className="text-lg font-bold">{formatRp(payTotal)}</p>
          {feeBreakdown(amount, fee)}
        </div>
        <div className="rounded-xl border border-[var(--border)] bg-[var(--panel-muted,rgba(0,0,0,0.03))] p-4 text-center">
          <p className="text-sm leading-relaxed">
            Anda akan diarahkan ke <strong>gateway pembayaran online</strong>
            {busy ? "…" : ` dalam ${Math.max(0, secondsLeft)} detik.`}
          </p>
          {!busy ? (
            <p className="mt-2 text-3xl font-semibold tabular-nums text-[var(--accent)]" aria-live="polite">
              {Math.max(0, secondsLeft)}
            </p>
          ) : (
            <p className="mt-2 text-sm text-[var(--muted)]">Menyiapkan halaman bayar…</p>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button type="button" className="btn" disabled={busy} onClick={onSkipWait}>
            Lanjutkan sekarang
          </button>
          <button type="button" className="btn-ghost" disabled={busy} onClick={onCancel}>
            Batal
          </button>
        </div>
        {sandboxAvailable && onSandboxPay ? (
          <div className="grid gap-1.5 rounded-xl border border-dashed border-[var(--border)] p-3">
            <p className="text-xs text-[var(--muted)]">
              Sandbox: uji tandai lunas tanpa membuka gateway.
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
  const [step, setStep] = useState<"method" | "redirect" | "busy">("method");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [secondsLeft, setSecondsLeft] = useState(SINGLE_PG_REDIRECT_SECONDS);
  const countdownFor = useRef<string | null>(null);
  const checkoutStarted = useRef(false);
  const options = useQuery({
    queryKey: ["portal-pay-options", tenantSlug],
    queryFn: () => api<PayOption[]>("/api/portal/pay-options", { headers }),
    enabled: Boolean(invoice?.id),
  });
  const methods = payOptionsToMethods(options.data);
  const sandboxAvailable = payOptionsHasDuitkuSandbox(options.data);
  const singleMethod = methods.length === 1 ? methods[0] : null;
  const singlePgFlow = !options.isLoading && methods.length === 1;

  useEffect(() => {
    setStep("method");
    setError("");
    setBusy(false);
    setSecondsLeft(SINGLE_PG_REDIRECT_SECONDS);
    countdownFor.current = null;
    checkoutStarted.current = false;
  }, [invoice?.id]);

  // Satu PG aktif → tampilkan countdown, bukan daftar nama PG.
  useEffect(() => {
    if (!invoice?.id || options.isLoading || busy) return;
    if (step !== "method") return;
    if (methods.length !== 1) return;
    countdownFor.current = invoice.id;
    checkoutStarted.current = false;
    setSecondsLeft(SINGLE_PG_REDIRECT_SECONDS);
    setStep("redirect");
  }, [invoice?.id, options.isLoading, methods.length, step, busy]);

  // Hitungan mundur lalu checkout.
  useEffect(() => {
    if (!invoice?.id || step !== "redirect" || busy || !singleMethod) return;
    if (countdownFor.current !== invoice.id) return;
    if (secondsLeft > 0) {
      const t = window.setTimeout(() => setSecondsLeft((s) => s - 1), 1000);
      return () => window.clearTimeout(t);
    }
    if (checkoutStarted.current) return;
    checkoutStarted.current = true;
    void confirmMethod(singleMethod.id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [invoice?.id, step, busy, secondsLeft, singleMethod?.id]);

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
      window.location.assign(url);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Gagal membuat pembayaran";
      setError(msg);
      void toastError(msg);
      checkoutStarted.current = false;
      setStep(methods.length === 1 ? "redirect" : "method");
      setSecondsLeft(SINGLE_PG_REDIRECT_SECONDS);
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
  const singleFee = singleMethod ? methodCustomerFee(singleMethod, amount) : 0;

  return (
    <>
      <PayMethodDialog
        open={!options.isLoading && step === "method" && methods.length !== 1}
        invoiceNumber={invoice.invoice_number}
        amount={amount}
        methods={methods}
        loading={false}
        busy={busy}
        error={error || (options.isError ? "Gagal memuat metode pembayaran" : "")}
        sandboxAvailable={sandboxAvailable}
        onClose={onClose}
        onConfirm={(m) => void confirmMethod(m)}
        onSandboxPay={() => void confirmSandboxPay()}
      />
      <PayRedirectNotice
        open={singlePgFlow && (step === "method" || step === "redirect" || step === "busy")}
        invoiceNumber={invoice.invoice_number}
        amount={amount}
        fee={singleFee}
        secondsLeft={secondsLeft}
        busy={busy || step === "busy"}
        error={error}
        sandboxAvailable={sandboxAvailable}
        onCancel={onClose}
        onSkipWait={() => {
          if (!singleMethod || busy || checkoutStarted.current) return;
          setSecondsLeft(0);
        }}
        onSandboxPay={() => void confirmSandboxPay()}
      />
      {options.isLoading ? (
        <FormDialog open title="Menuju pembayaran" onClose={onClose}>
          <p className="text-sm text-[var(--muted)]">Memuat metode pembayaran…</p>
        </FormDialog>
      ) : null}
    </>
  );
}
