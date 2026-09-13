/** Portal payment methods. Add new PG entries here; the picker stays the first step. */

export const PAY_METHOD_QRIS = "qris" as const;
export const PAY_METHOD_DUITKU = "duitku" as const;
export const PAY_METHOD_DOKU = "doku" as const;
export const PAY_METHOD_TUNAI = "tunai" as const;
export const PAY_METHOD_TRANSFER = "transfer" as const;

export type PayMethodId =
  | typeof PAY_METHOD_QRIS
  | typeof PAY_METHOD_DUITKU
  | typeof PAY_METHOD_DOKU
  | typeof PAY_METHOD_TUNAI
  | typeof PAY_METHOD_TRANSFER;

export type DokuChannelOption = {
  id: string;
  label: string;
  kind: string;
  enabled?: boolean;
  fee_flat?: number;
  fee_percent?: number;
};

export type PayOption = {
  provider: string;
  label: string;
  description: string;
  kind: string;
  sandbox?: boolean;
  fee_mode?: string;
  fee_flat?: number;
  fee_percent?: number;
  channels?: DokuChannelOption[];
};

export type PayMethodDef = {
  id: PayMethodId;
  label: string;
  description: string;
  feeMode?: string;
  feeFlat?: number;
  feePercent?: number;
  channels?: DokuChannelOption[];
};

export const PORTAL_PAY_METHODS: PayMethodDef[] = [
  {
    id: PAY_METHOD_DUITKU,
    label: "Duitku Payment Gateway",
    description: "Halaman pembayaran Duitku (VA, e-wallet, retail, QRIS)",
  },
  {
    id: PAY_METHOD_DOKU,
    label: "DOKU",
    description: "QRIS, VA bank, e-wallet, Alfamart/Indomaret",
  },
];

export type PayableInvoice = {
  id?: string;
  invoice_number: string;
  status: string;
  total_amount: number;
  paid_amount?: number;
};

function storageKey(slug?: string) {
  const s = String(slug || "").trim();
  return s ? `drp_pay_method:${s}` : "drp_pay_method";
}

export function isPayMethodId(value: string | null | undefined): value is PayMethodId {
  return PORTAL_PAY_METHODS.some((m) => m.id === value);
}

export function providerToPayMethod(provider?: string | null): PayMethodId | null {
  const key = String(provider || "").trim().toLowerCase();
  if (!key) return null;
  if (key === "drp" || key === "qris" || key === "qr") return PAY_METHOD_QRIS;
  if (key === "duitku" || key === "duitku_pop" || key === "pop") return PAY_METHOD_DUITKU;
  if (key === "doku") return PAY_METHOD_DOKU;
  if (isPayMethodId(key)) return key;
  return null;
}

export function payMethodToProvider(method: PayMethodId): string {
  // QRIS khusus (gateway lama) sudah dihapus — fallback ke Duitku agar tidak
  // ada request provider "drp" yang tidak dikenal backend.
  if (method === PAY_METHOD_QRIS) return PAY_METHOD_DUITKU;
  return method;
}

export function payOptionsToMethods(options: PayOption[] | null | undefined): PayMethodDef[] {
  if (!options?.length) return [];
  const out: PayMethodDef[] = [];
  for (const opt of options) {
    const id = providerToPayMethod(opt.provider);
    if (!id) continue;
    out.push({
      id,
      label: opt.label || paymentMethodLabel(id),
      description: opt.description || "",
      feeMode: opt.fee_mode,
      feeFlat: opt.fee_flat,
      feePercent: opt.fee_percent,
      channels: opt.channels,
    });
  }
  return out;
}

/** True when Duitku sandbox is the active PG — testers can mark paid without the Duitku dashboard. */
export function payOptionsHasDuitkuSandbox(options: PayOption[] | null | undefined): boolean {
  return Boolean(
    options?.some((opt) => opt.sandbox && providerToPayMethod(opt.provider) === PAY_METHOD_DUITKU),
  );
}

/** Biaya admin channel DOKU: persen dari biaya dasar MDR (fee_flat), bukan dari nominal invoice.
 * Persen 0/kosong = 100% biaya dasar dibebankan ke customer.
 */
export function channelCustomerFee(
  feeMode: string | undefined,
  channel: Pick<DokuChannelOption, "fee_flat" | "fee_percent"> | null | undefined,
  invoiceBase: number,
  fallback?: Pick<PayMethodDef, "feeFlat" | "feePercent">,
): number {
  if (feeMode !== "customer" || invoiceBase <= 0) return 0;
  const flat = Math.max(0, Math.floor(channel?.fee_flat ?? fallback?.feeFlat ?? 0));
  if (flat <= 0) return 0;
  let pct = Math.max(0, channel?.fee_percent ?? fallback?.feePercent ?? 0);
  if (pct <= 0) pct = 100;
  const fee = Math.round((flat * pct) / 100);
  return fee > 0 ? fee : 0;
}

/** Biaya admin yang dibebankan ke customer untuk metode ini (0 bila merchant tanggung). */
export function methodCustomerFee(method: Pick<PayMethodDef, "feeMode" | "feeFlat" | "feePercent" | "channels">, base: number): number {
  if (method.feeMode !== "customer" || base <= 0) return 0;
  if (method.channels?.length) {
    return method.channels.some((ch) => channelCustomerFee(method.feeMode, ch, base) > 0)
      ? Math.max(...method.channels.map((ch) => channelCustomerFee(method.feeMode, ch, base)))
      : 0;
  }
  return channelCustomerFee(method.feeMode, { fee_flat: method.feeFlat, fee_percent: method.feePercent }, base);
}

/** True bila ada metode dengan biaya admin ke customer (untuk menahan auto-redirect). */
export function payMethodsHaveCustomerFee(methods: PayMethodDef[], base: number): boolean {
  return methods.some((m) => methodCustomerFee(m, base) > 0);
}

export function dokuChannelsFromMethod(method: PayMethodDef | undefined): DokuChannelOption[] {
  return (method?.channels || []).filter((c) => c.enabled !== false);
}

/** Label for a stored method/provider id. Unknown values are shown as-is (never forced to "manual"). */
export function paymentMethodLabel(method?: string | null) {
  const key = String(method || "").trim().toLowerCase();
  if (!key) return "—";
  const fromCatalog = PORTAL_PAY_METHODS.find((m) => m.id === key);
  if (fromCatalog) return fromCatalog.label;
  if (key === "drp" || key === "qr" || key === PAY_METHOD_QRIS) return "QRIS";
  if (key === "duitku_pop" || key === "duitkupop" || key === "pop") return "Duitku Payment Gateway";
  if (key === PAY_METHOD_DOKU) return "DOKU";
  if (key === PAY_METHOD_TUNAI || key === "cash" || key === "kasir" || key === "manual") return "Tunai";
  if (key === PAY_METHOD_TRANSFER || key === "bank" || key === "va") return "Transfer";
  return String(method).trim();
}

export function getSavedPayMethod(slug?: string): PayMethodId {
  try {
    const raw = localStorage.getItem(storageKey(slug));
    if (isPayMethodId(raw)) return raw;
  } catch {
    /* private mode */
  }
  return PAY_METHOD_DUITKU;
}

export function setSavedPayMethod(id: PayMethodId, slug?: string) {
  try {
    localStorage.setItem(storageKey(slug), id);
  } catch {
    /* ignore quota */
  }
}

/** True when the customer has explicitly saved a method before (not the default). */
export function hasSavedPayMethod(slug?: string): boolean {
  try {
    return isPayMethodId(localStorage.getItem(storageKey(slug)));
  } catch {
    return false;
  }
}

/** Forget the saved method so the picker is shown again on the next payment. */
export function clearSavedPayMethod(slug?: string) {
  try {
    localStorage.removeItem(storageKey(slug));
  } catch {
    /* ignore */
  }
}

export function invoiceRemaining(inv: Pick<PayableInvoice, "total_amount" | "paid_amount">) {
  return Math.max(0, inv.total_amount - (inv.paid_amount || 0));
}

export function isInvoiceUnpaid(inv: PayableInvoice) {
  return inv.status !== "paid" && invoiceRemaining(inv) > 0;
}

export function isIsolirStatus(status?: string | null) {
  const s = String(status || "").trim().toLowerCase();
  return s === "suspended" || s === "isolir";
}

export const PAYMENT_RETURN_PARAM = "payment";
export const PAYMENT_RETURN_SUCCESS = "success";
const PAYMENT_RETURN_STORAGE = "drp_payment_return";

/** Return URL ke portal setelah hosted checkout (DOKU / fallback Duitku). */
export function portalPaymentReturnURL(): string {
  if (typeof window === "undefined") return "";
  const u = new URL(window.location.href);
  u.searchParams.set(PAYMENT_RETURN_PARAM, PAYMENT_RETURN_SUCCESS);
  return u.toString();
}

/** Simpan flag callback sebelum guard login membuang query string. */
export function notePaymentReturnFromLocation() {
  if (typeof window === "undefined") return;
  try {
    const u = new URL(window.location.href);
    if (u.searchParams.get(PAYMENT_RETURN_PARAM) === PAYMENT_RETURN_SUCCESS) {
      sessionStorage.setItem(PAYMENT_RETURN_STORAGE, "1");
    }
  } catch {
    /* private mode */
  }
}

/** True sekali saat customer kembali dari PG. Membersihkan query + sessionStorage. */
export function consumePaymentReturnSuccess(): boolean {
  notePaymentReturnFromLocation();
  let hit = false;
  try {
    hit = sessionStorage.getItem(PAYMENT_RETURN_STORAGE) === "1";
    if (hit) sessionStorage.removeItem(PAYMENT_RETURN_STORAGE);
  } catch {
    /* ignore */
  }
  if (typeof window === "undefined") return hit;
  const u = new URL(window.location.href);
  if (u.searchParams.get(PAYMENT_RETURN_PARAM) === PAYMENT_RETURN_SUCCESS) {
    hit = true;
    u.searchParams.delete(PAYMENT_RETURN_PARAM);
    const next = `${u.pathname}${u.search}${u.hash}`;
    window.history.replaceState(window.history.state, "", next);
  }
  return hit;
}
