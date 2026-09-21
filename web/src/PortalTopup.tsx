import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { QrisPayDialog, type QrisIntent } from "./QrisPayDialog";
import { markPendingTopup, payMethodRequest, payOptionsToMethods, portalPaymentReturnURL, type PayMethodDef, type PayMethodId, type PayOption } from "./payMethod";
import { alertTopupSuccess, toastError } from "./swal";
import { formatRp, FormDialog } from "./ui";

/** Dialog topup saldo portal: input nominal + metode, lalu QR/redirect seperti bayar tagihan. */
export function PortalTopupDialog({
  open,
  onClose,
  headers,
  tenantSlug,
  minTopup,
  balance,
  onDone,
}: {
  open: boolean;
  onClose: () => void;
  headers?: HeadersInit;
  tenantSlug?: string;
  minTopup: number;
  balance: number;
  onDone?: () => void;
}) {
  const [amount, setAmount] = useState("");
  const [method, setMethod] = useState<PayMethodId | "">("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [intent, setIntent] = useState<QrisIntent | null>(null);

  const options = useQuery({
    queryKey: ["portal-pay-options", tenantSlug],
    queryFn: () => api<PayOption[]>("/api/portal/pay-options", { headers }),
    enabled: open,
  });
  const methods = payOptionsToMethods(options.data);
  // Bila hanya 1 PG aktif: jangan tampilkan nama/pilihan PG, langsung ke checkout.
  const singleMethod = methods.length === 1 ? methods[0] : null;

  useEffect(() => {
    if (open) return;
    setIntent(null);
    setError("");
    setAmount("");
  }, [open]);

  useEffect(() => {
    if (singleMethod) {
      setMethod(singleMethod.id);
      return;
    }
    if (!method && methods.length > 0) setMethod(methods[0].id);
  }, [methods, method, singleMethod]);

  async function submit() {
    const amt = Math.floor(Number(amount) || 0);
    if (amt < minTopup) {
      setError(`Minimal topup ${formatRp(minTopup)}.`);
      return;
    }
    const chosenId = singleMethod?.id || method;
    if (!chosenId) {
      setError("Pilih metode pembayaran.");
      return;
    }
    const chosenDef: PayMethodDef | undefined =
      methods.find((m) => m.id === chosenId) || singleMethod || undefined;
    const { provider, channel } = chosenDef
      ? payMethodRequest(chosenDef)
      : { provider: String(chosenId), channel: undefined };
    setBusy(true);
    setError("");
    try {
      const pi = await api<QrisIntent>("/api/portal/wallet/topup", {
        method: "POST",
        headers,
        body: JSON.stringify({
          amount: amt,
          provider,
          channel,
          return_url: portalPaymentReturnURL(),
        }),
      });
      const checkoutURL = String(pi.checkout_url || "").trim();
      const meta = (pi.metadata || {}) as Record<string, unknown>;
      const codePay = Boolean(meta.va_number || meta.payment_code);
      // Satu PG dengan halaman redirect: langsung ke halaman checkout tanpa
      // dialog perantara (sama seperti alur bayar tagihan). Simpan external_id
      // agar status topup bisa diverifikasi begitu kembali ke dashboard portal.
      if (singleMethod && checkoutURL && !pi.qr_image_base64 && !codePay) {
        markPendingTopup(String(pi.external_id || ""));
        window.location.assign(checkoutURL);
        return;
      }
      setIntent(pi);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Gagal membuat pembayaran");
      void toastError(e instanceof Error ? e.message : "Gagal membuat pembayaran");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <FormDialog open={open && !intent} title="Topup saldo" onClose={onClose}>
        <div className="grid gap-3">
          <div className="rounded-md border border-[var(--border)] bg-[var(--panel-muted)]/50 px-3 py-2 text-sm">
            Saldo saat ini: <strong>{formatRp(balance)}</strong>
          </div>
          <div>
            <label className="mb-1.5 block text-sm font-medium">Nominal topup (Rp)</label>
            <input
              className="input"
              type="number"
              min={minTopup}
              step={1000}
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder={String(minTopup)}
            />
            <p className="mt-1 text-xs text-[var(--muted)]">Minimal {formatRp(minTopup)}.</p>
          </div>
          {options.isLoading ? (
            <div>
              <label className="mb-1.5 block text-sm font-medium">Metode pembayaran</label>
              <p className="text-sm text-[var(--muted)]">Memuat metode…</p>
            </div>
          ) : methods.length === 0 ? (
            <p className="text-sm text-[var(--danger)]">Belum ada metode pembayaran online yang aktif.</p>
          ) : singleMethod ? (
            singleMethod.channel ? (
              <p className="text-xs text-[var(--muted)]">
                Kode QR <strong>{singleMethod.label}</strong> akan ditampilkan untuk menyelesaikan topup.
              </p>
            ) : (
              <p className="text-xs text-[var(--muted)]">
                Anda akan diarahkan ke <strong>gateway pembayaran online</strong> untuk menyelesaikan topup.
              </p>
            )
          ) : (
            <div>
              <label className="mb-1.5 block text-sm font-medium">Metode pembayaran</label>
              <select className="input" value={method} onChange={(e) => setMethod(e.target.value as PayMethodId)}>
                {methods.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.label}
                  </option>
                ))}
              </select>
            </div>
          )}
          {error ? <p className="text-sm text-[var(--danger)]">{error}</p> : null}
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              className="btn"
              disabled={busy || methods.length === 0}
              onClick={() => void submit()}
            >
              {busy ? "Memproses…" : "Lanjut bayar"}
            </button>
            <button type="button" className="btn-ghost" onClick={onClose}>
              Batal
            </button>
          </div>
        </div>
      </FormDialog>

      <QrisPayDialog
        open={Boolean(intent)}
        title="Topup saldo"
        invoiceNumber="Topup saldo"
        intent={intent}
        onClose={() => setIntent(null)}
        pollPath={intent?.external_id ? `/api/portal/wallet/topup/${intent.external_id}` : undefined}
        pollHeaders={headers}
        onPaid={() => {
          setIntent(null);
          void alertTopupSuccess();
          onDone?.();
          onClose();
        }}
      />
    </>
  );
}
