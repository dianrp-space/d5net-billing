/**
 * Duitku POP (popup) checkout loader.
 *
 * Instead of redirecting to `paymentUrl` in a new tab, we load Duitku's JS SDK
 * and call `checkout.process(reference, callbacks)` to show the payment popup
 * on top of the portal. The SDK URL differs per environment (sandbox/prod).
 */

const SANDBOX_SDK = "https://app-sandbox.duitku.com/lib/js/duitku.js";
const PROD_SDK = "https://app-prod.duitku.com/lib/js/duitku.js";

type DuitkuCheckout = {
  process: (reference: string, options: Record<string, unknown>) => void;
};

declare global {
  interface Window {
    checkout?: DuitkuCheckout;
  }
}

const loaders: Record<string, Promise<void>> = {};

function loadScript(url: string): Promise<void> {
  if (typeof document === "undefined") {
    return Promise.reject(new Error("Duitku hanya tersedia di browser"));
  }
  if (url in loaders) return loaders[url];
  loaders[url] = new Promise<void>((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(`script[src="${url}"]`);
    if (existing) {
      if (window.checkout) {
        resolve();
        return;
      }
      existing.addEventListener("load", () => resolve(), { once: true });
      existing.addEventListener("error", () => reject(new Error("Gagal memuat modul Duitku")), { once: true });
      return;
    }
    const s = document.createElement("script");
    s.src = url;
    s.async = true;
    s.addEventListener("load", () => resolve(), { once: true });
    s.addEventListener("error", () => {
      delete loaders[url];
      reject(new Error("Gagal memuat modul Duitku"));
    }, { once: true });
    document.head.appendChild(s);
  });
  return loaders[url];
}

export type DuitkuPopupCallbacks = {
  onSuccess?: (result: unknown) => void;
  onPending?: (result: unknown) => void;
  onError?: (result: unknown) => void;
  onClose?: (result: unknown) => void;
};

/**
 * Opens the Duitku payment popup for a given reference. Throws if the SDK cannot
 * load or the reference is missing so callers can fall back to the redirect URL.
 */
export async function openDuitkuPopup(
  reference: string,
  sandbox: boolean,
  cb: DuitkuPopupCallbacks = {},
): Promise<void> {
  const ref = String(reference || "").trim();
  if (!ref) throw new Error("Referensi Duitku kosong");
  await loadScript(sandbox ? SANDBOX_SDK : PROD_SDK);
  const checkout = window.checkout;
  if (!checkout || typeof checkout.process !== "function") {
    throw new Error("Modul Duitku belum siap");
  }
  checkout.process(ref, {
    defaultLanguage: "id",
    successEvent: (result: unknown) => cb.onSuccess?.(result),
    pendingEvent: (result: unknown) => cb.onPending?.(result),
    errorEvent: (result: unknown) => cb.onError?.(result),
    closeEvent: (result: unknown) => cb.onClose?.(result),
  });
}
