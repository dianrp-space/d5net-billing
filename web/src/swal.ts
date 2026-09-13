import Swal from "sweetalert2";
import { DEFAULT_PRIMARY } from "./theme";

export type ConfirmOptions = {
  title?: string;
  description: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
};

const FALLBACK_SECONDARY = "#8C7355";
const danger = "#b91c1c";

/** Warna accent aktif dari setting branding (CSS var --accent, di-set TenantAccent). */
function cssVar(name: string, fallback: string): string {
  try {
    const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    if (v) return v;
  } catch {
    /* abaikan (SSR / test tanpa DOM) */
  }
  return fallback;
}

export function swalConfirmColor(): string {
  return cssVar("--accent", DEFAULT_PRIMARY);
}

export function swalCancelColor(): string {
  return cssVar("--secondary", FALLBACK_SECONDARY);
}

function mixin() {
  return Swal.mixin({
    buttonsStyling: true,
    confirmButtonColor: swalConfirmColor(),
    cancelButtonColor: swalCancelColor(),
  });
}

function escapeHtml(s: string) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

/** Escape user text then allow explicit line breaks via \n. */
function toHtml(s: string) {
  return escapeHtml(s).replace(/\n/g, "<br/>");
}

export async function swalConfirm(opts: ConfirmOptions | string): Promise<boolean> {
  const n = typeof opts === "string" ? { description: opts } : opts;
  const isDanger = n.danger ?? true;
  const result = await mixin().fire({
    title: n.title ?? "Konfirmasi",
    html: toHtml(n.description),
    icon: isDanger ? "warning" : "question",
    showCancelButton: true,
    focusCancel: true,
    reverseButtons: true,
    confirmButtonText: n.confirmLabel ?? "Ya, lanjutkan",
    cancelButtonText: n.cancelLabel ?? "Batal",
    confirmButtonColor: isDanger ? danger : swalConfirmColor(),
    cancelButtonColor: swalCancelColor(),
  });
  return result.isConfirmed;
}

export async function swalAlert(opts: { title?: string; description: string; icon?: "info" | "success" | "warning" | "error" } | string): Promise<void> {
  const n = typeof opts === "string" ? { description: opts } : opts;
  await mixin().fire({
    title: n.title ?? "Pemberitahuan",
    html: toHtml(n.description),
    icon: (typeof opts === "object" && opts.icon) || "info",
    confirmButtonText: "Tutup",
  });
}

export function alertPaymentSuccess() {
  return swalAlert({
    title: "Pembayaran sukses",
    description: "Pembayaran diterima. Status tagihan akan diperbarui otomatis.",
    icon: "success",
  });
}

export function toastSuccess(title: string) {
  return Swal.fire({
    toast: true,
    position: "top-end",
    icon: "success",
    title,
    showConfirmButton: false,
    timer: 2200,
    timerProgressBar: true,
  });
}

export function toastError(title: string) {
  return Swal.fire({
    toast: true,
    position: "top-end",
    icon: "error",
    title,
    showConfirmButton: false,
    timer: 3200,
    timerProgressBar: true,
  });
}
