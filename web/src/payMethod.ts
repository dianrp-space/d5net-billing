/** Portal payment methods. Add new PG entries here; the picker stays the first step. */

export const PAY_METHOD_QRIS = "qris" as const;
export const PAY_METHOD_TUNAI = "tunai" as const;
export const PAY_METHOD_TRANSFER = "transfer" as const;

export type PayMethodId = typeof PAY_METHOD_QRIS | typeof PAY_METHOD_TUNAI | typeof PAY_METHOD_TRANSFER;

export type PayMethodDef = {
  id: PayMethodId;
  label: string;
  description: string;
};

export const PORTAL_PAY_METHODS: PayMethodDef[] = [
  {
    id: PAY_METHOD_QRIS,
    label: "QRIS",
    description: "Scan QR dengan e-wallet atau m-banking",
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

/** Label for a stored method/provider id. Unknown values are shown as-is (never forced to "manual"). */
export function paymentMethodLabel(method?: string | null) {
  const key = String(method || "").trim().toLowerCase();
  if (!key) return "—";
  const fromCatalog = PORTAL_PAY_METHODS.find((m) => m.id === key);
  if (fromCatalog) return fromCatalog.label;
  if (key === "drp" || key === "qr") return "QRIS";
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
  return PAY_METHOD_QRIS;
}

export function setSavedPayMethod(id: PayMethodId, slug?: string) {
  try {
    localStorage.setItem(storageKey(slug), id);
  } catch {
    /* ignore quota */
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
