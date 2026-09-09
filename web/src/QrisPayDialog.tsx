import { useEffect, useState } from "react";
import { api } from "./api";
import { useAppDialog } from "./confirm";
import { toastError, toastSuccess } from "./swal";
import { formatRp, FormDialog } from "./ui";

function qrisDataUrl(src: string) {
  return src.startsWith("data:") ? src : `data:image/png;base64,${src}`;
}

function safeFilenamePart(value: string) {
  return value.replace(/[^\w.-]+/g, "-").replace(/^-+|-+$/g, "") || "tagihan";
}

async function blobFromDataUrl(src: string): Promise<Blob> {
  const res = await fetch(qrisDataUrl(src));
  return res.blob();
}

async function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.rel = "noopener";
  document.body.appendChild(a);
  a.click();
  a.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 2000);
}

export type QrisIntent = {
  id: string;
  status: string;
  provider?: string;
  qr_string?: string;
  qr_image_base64?: string;
  checkout_url?: string;
  payable_amount?: number;
  unique_digit?: number;
  amount?: number;
  expires_at?: string | null;
  metadata?: Record<string, unknown> | null;
};

function statusOf(status?: string) {
  return String(status || "").toLowerCase();
}

function isPaid(status?: string) {
  return statusOf(status) === "paid";
}

function isCancelled(status?: string) {
  const s = statusOf(status);
  return s === "cancelled" || s === "canceled";
}

function isExpired(status?: string) {
  return statusOf(status) === "expired" || statusOf(status) === "failed";
}

export function QrisPayDialog({
  open,
  title,
  invoiceNumber,
  intent,
  onClose,
  onPaid,
  onCancelled,
  pollPath,
  cancelPath,
  pollHeaders,
}: {
  open: boolean;
  title?: string;
  invoiceNumber: string;
  intent: QrisIntent | null;
  onClose: () => void;
  onPaid?: () => void;
  onCancelled?: () => void;
  pollPath?: string;
  cancelPath?: string;
  pollHeaders?: HeadersInit;
}) {
  const { confirm } = useAppDialog();
  const [current, setCurrent] = useState<QrisIntent | null>(intent);
  const [checking, setChecking] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const paid = isPaid(current?.status);
  const cancelled = isCancelled(current?.status);
  const expired = isExpired(current?.status);
  const img = current?.qr_image_base64 || "";
  const checkoutURL = String(current?.checkout_url || "").trim();
  const isRedirect = Boolean(checkoutURL) && !img;
  const live = Boolean(img || checkoutURL) && !paid && !cancelled && !expired;

  useEffect(() => {
    setCurrent(intent);
  }, [intent]);

  async function refreshStatus(opts?: { silent?: boolean }): Promise<QrisIntent | null> {
    if (!pollPath) return null;
    const next = await api<QrisIntent>(pollPath, { headers: pollHeaders });
    setCurrent(next);
    if (isPaid(next.status)) onPaid?.();
    if (!opts?.silent && !isPaid(next.status) && !isCancelled(next.status) && !isExpired(next.status)) {
      void toastError("Belum terdeteksi. Jika sudah transfer, tunggu sebentar lalu cek lagi.");
    }
    return next;
  }

  useEffect(() => {
    if (!open || !pollPath || paid || cancelled || expired) return;
    let stopped = false;
    const tick = async () => {
      try {
        if (stopped) return;
        await refreshStatus({ silent: true });
      } catch {
        /* intent may not exist yet */
      }
    };
    const id = window.setInterval(() => void tick(), 4000);
    return () => {
      stopped = true;
      window.clearInterval(id);
    };
    // refreshStatus is recreated each render; interval should follow open/status/path only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, pollPath, paid, cancelled, expired, onPaid, pollHeaders]);

  async function onCheckPaid() {
    if (!pollPath) return;
    setChecking(true);
    try {
      await refreshStatus();
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Gagal cek status");
    } finally {
      setChecking(false);
    }
  }

  async function onCancel() {
    if (!cancelPath) return;
    const ok = await confirm({
      title: isRedirect ? "Batalkan pembayaran?" : "Batalkan QRIS?",
      description: isRedirect
        ? "Link pembayaran ini tidak dipakai lagi di aplikasi. Selesaikan di Duitku jika sudah dibuka."
        : "Kode QR ini tidak bisa dipakai lagi setelah dibatalkan.",
      confirmLabel: "Batalkan",
      danger: true,
    });
    if (!ok) return;
    setCancelling(true);
    try {
      const next = await api<QrisIntent>(cancelPath, { method: "POST", headers: pollHeaders });
      setCurrent(next);
      if (isPaid(next.status)) {
        onPaid?.();
        void toastSuccess("Pembayaran QRIS diterima");
        return;
      }
      void toastSuccess(isRedirect ? "Pembayaran dibatalkan" : "QRIS dibatalkan");
      onCancelled?.();
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Gagal membatalkan pembayaran");
    } finally {
      setCancelling(false);
    }
  }

  const payAmount = current?.payable_amount || current?.amount || 0;
  const expires = current?.expires_at ? new Date(current.expires_at) : null;

  function openCheckout() {
    if (!checkoutURL) return;
    window.open(checkoutURL, "_blank", "noopener,noreferrer");
  }

  async function onDownloadQris() {
    if (!img) return;
    setDownloading(true);
    try {
      const blob = await blobFromDataUrl(img);
      const filename = `qris-${safeFilenamePart(invoiceNumber)}.png`;
      const file = new File([blob], filename, { type: blob.type || "image/png" });
      const shareData: ShareData = { files: [file], title: `QRIS ${invoiceNumber}` };
      if (typeof navigator.canShare === "function" && navigator.canShare(shareData) && navigator.share) {
        try {
          await navigator.share(shareData);
          return;
        } catch (err) {
          if (err instanceof DOMException && err.name === "AbortError") return;
        }
      }
      await downloadBlob(blob, filename);
      void toastSuccess("QRIS diunduh. Buka dari galeri di aplikasi e-wallet.");
    } catch {
      void toastError("Gagal mengunduh QRIS");
    } finally {
      setDownloading(false);
    }
  }

  return (
    <FormDialog open={open} title={title || (isRedirect ? `Bayar · ${invoiceNumber}` : `Bayar QRIS · ${invoiceNumber}`)} onClose={onClose}>
      {paid ? (
        <p className="text-sm font-medium text-[var(--ok)]">Pembayaran diterima. Tagihan akan ditandai lunas.</p>
      ) : cancelled ? (
        <p className="text-sm font-medium text-[var(--muted)]">
          {isRedirect ? "Pembayaran dibatalkan. Buat ulang jika ingin membayar." : "QRIS dibatalkan. Buat ulang jika ingin membayar."}
        </p>
      ) : expired ? (
        <p className="text-sm font-medium text-[var(--warn)]">
          {isRedirect ? "Link pembayaran kedaluwarsa. Buat ulang dari tombol bayar." : "QRIS kedaluwarsa. Buat ulang dari tombol Bayar QRIS."}
        </p>
      ) : isRedirect ? (
        <div className="grid justify-items-center gap-3 text-center">
          <div>
            <p className="text-xs text-[var(--muted)]">Total pembayaran</p>
            <p className="text-lg font-bold">{formatRp(payAmount)}</p>
          </div>
          {expires ? (
            <p className="text-[11px] text-[var(--muted)]">
              Berlaku sampai {expires.toLocaleString("id-ID")}
            </p>
          ) : null}
          <p className="max-w-xs text-[11px] leading-relaxed text-[var(--muted)]">
            Selesaikan di halaman pembayaran (VA, e-wallet, retail, atau QRIS). Setelah bayar, status tagihan
            terbarui otomatis.
          </p>
          <div className="mt-1 flex w-full flex-wrap justify-center gap-2">
            <button type="button" className="btn" disabled={checking} onClick={openCheckout}>
              Buka halaman pembayaran
            </button>
            {pollPath ? (
              <button type="button" className="btn-ghost" disabled={checking} onClick={() => void onCheckPaid()}>
                {checking ? "Mengecek…" : "Aku sudah bayar"}
              </button>
            ) : null}
          </div>
        </div>
      ) : (
        <div className="grid justify-items-center gap-3 text-center">
          {img ? (
            <div className={live ? "qris-frame is-live" : "qris-frame"}>
              <img
                src={img.startsWith("data:") ? img : `data:image/png;base64,${img}`}
                alt="QRIS"
              />
            </div>
          ) : (
            <p className="text-sm text-[var(--muted)]">Menyiapkan QRIS…</p>
          )}
          <div>
            <p className="text-xs text-[var(--muted)]">Bayar tepat sebesar</p>
            <p className="text-lg font-bold">{formatRp(payAmount)}</p>
            <p className="mt-1 max-w-xs text-[11px] leading-relaxed text-[var(--muted)]">
              Nominal ini sudah termasuk <strong>kode unik</strong>
              {current?.unique_digit ? ` (${current.unique_digit})` : " 3 digit"} untuk konfirmasi otomatis. Bayar
              persis jumlah itu, jangan dibulatkan.
            </p>
          </div>
          {expires ? (
            <p className="text-[11px] text-[var(--muted)]">
              Berlaku sampai {expires.toLocaleString("id-ID")}
            </p>
          ) : null}
          <p className="text-[11px] text-[var(--muted)]">
            Scan dengan e-wallet / m-banking. Kalau hanya punya 1 HP, unduh QRIS lalu unggah dari galeri.
          </p>
          <div className="mt-1 flex w-full flex-wrap justify-center gap-2">
            {img ? (
              <button
                type="button"
                className="btn-ghost"
                disabled={downloading || checking || cancelling}
                onClick={() => void onDownloadQris()}
              >
                {downloading ? "Mengunduh…" : "Unduh QRIS"}
              </button>
            ) : null}
            {pollPath ? (
              <button type="button" className="btn" disabled={checking || cancelling} onClick={() => void onCheckPaid()}>
                {checking ? "Mengecek…" : "Aku sudah bayar"}
              </button>
            ) : null}
            {cancelPath ? (
              <button type="button" className="btn-ghost" disabled={checking || cancelling} onClick={() => void onCancel()}>
                {cancelling ? "Membatalkan…" : "Batalkan QRIS"}
              </button>
            ) : null}
          </div>
        </div>
      )}
    </FormDialog>
  );
}
