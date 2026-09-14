import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { QrisPayDialog, type QrisIntent } from "./QrisPayDialog";
import { payMethodToProvider, payOptionsToMethods, type PayMethodId, type PayOption } from "./payMethod";
import { toastError, toastSuccess } from "./swal";
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

  useEffect(() => {
    if (open) return;
    setIntent(null);
    setError("");
    setAmount("");
  }, [open]);

  useEffect(() => {
    if (!method && methods.length > 0) setMethod(methods[0].id);
  }, [methods, method]);

  async function submit() {
    const amt = Math.floor(Number(amount) || 0);
    if (amt < minTopup) {
      setError(`Minimal topup ${formatRp(minTopup)}.`);
      return;
    }
    if (!method) {
      setError("Pilih metode pembayaran.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const pi = await api<QrisIntent>("/api/portal/wallet/topup", {
        method: "POST",
        headers,
        body: JSON.stringify({ amount: amt, provider: payMethodToProvider(method) }),
      });
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
          <div>
            <label className="mb-1.5 block text-sm font-medium">Metode pembayaran</label>
            {options.isLoading ? (
              <p className="text-sm text-[var(--muted)]">Memuat metode…</p>
            ) : methods.length === 0 ? (
              <p className="text-sm text-[var(--danger)]">Belum ada metode pembayaran online yang aktif.</p>
            ) : (
              <select className="input" value={method} onChange={(e) => setMethod(e.target.value as PayMethodId)}>
                {methods.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.label}
                  </option>
                ))}
              </select>
            )}
          </div>
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
          void toastSuccess("Topup saldo berhasil");
          onDone?.();
          onClose();
        }}
      />
    </>
  );
}
