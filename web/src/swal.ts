import Swal from "sweetalert2";

export type ConfirmOptions = {
  title?: string;
  description: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
};

const olive = "#5A5A40";
const terracotta = "#8C7355";
const danger = "#b91c1c";

function mixin() {
  return Swal.mixin({
    buttonsStyling: true,
    confirmButtonColor: olive,
    cancelButtonColor: terracotta,
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
    confirmButtonColor: isDanger ? danger : olive,
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
