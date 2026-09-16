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
    label: "DUITKU",
    description: "Halaman pembayaran Duitku (VA, e-wallet, retail, QRIS)",
  },
  {
    id: PAY_METHOD_DOKU,
    label: "DOKU",
    description: "Halaman bayar DOKU (VA, e-wallet, QRIS, retail)",
  },
];

export type PayableInvoice = {
  id?: string;
  invoice_number: string;
  status: string;
  total_amount: number;
  paid_amount?: number;
  admin_fee?: number;
  payable_amount?: number;
  items_summary?: string;
  customer_code?: string;
  customer_name?: string;
  due_date?: string;
  paid_at?: string | null;
  /** True bila tagihan mengisolir layanan saat lewat jatuh tempo. */
  isolir?: boolean;
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

/** Biaya admin: persen dari biaya dasar MDR (fee_flat), bukan dari nominal invoice.
 * Persen 0/kosong = 100% biaya dasar dibebankan ke customer. Dipakai DOKU & Duitku.
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
export function paymentMethodLabel(method?: string | null, sandbox?: boolean | null) {
  const key = String(method || "").trim().toLowerCase();
  if (!key) return "—";
  const fromCatalog = PORTAL_PAY_METHODS.find((m) => m.id === key);
  let label: string;
  if (fromCatalog) label = fromCatalog.label;
  else if (key === "drp" || key === "qr" || key === PAY_METHOD_QRIS) label = "QRIS";
  else if (key === "duitku_pop" || key === "duitkupop" || key === "pop") label = "DUITKU";
  else if (key === PAY_METHOD_DOKU) label = "DOKU";
  else if (key === PAY_METHOD_TUNAI || key === "cash" || key === "kasir" || key === "manual") label = "Tunai";
  else if (key === "saldo" || key === "wallet") label = "Saldo";
  else if (key === PAY_METHOD_TRANSFER || key === "bank" || key === "va") label = "Transfer";
  else label = String(method).trim();
  return sandbox ? `Sandbox · ${label}` : label;
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
/** Marker bahwa customer kembali dari halaman PG (bukan jaminan sudah lunas). */
export const PAYMENT_RETURN_VALUE = "return";
/** @deprecated alias lama — masih dikenali saat consume. */
export const PAYMENT_RETURN_SUCCESS = "success";
const PAYMENT_RETURN_STORAGE = "drp_payment_return";
const PAYMENT_RESULT_CODE_STORAGE = "drp_payment_result_code";

function isPaymentReturnValue(v: string | null): boolean {
  return v === PAYMENT_RETURN_VALUE || v === PAYMENT_RETURN_SUCCESS;
}

function isPaidStatus(status?: string | null): boolean {
  const s = String(status || "").trim().toLowerCase();
  return s === "paid" || s === "success" || s === "settlement" || s === "completed";
}

/** Return URL ke portal setelah hosted checkout (DOKU / Duitku). */
export function portalPaymentReturnURL(): string {
  if (typeof window === "undefined") return "";
  const u = new URL(window.location.href);
  u.searchParams.set(PAYMENT_RETURN_PARAM, PAYMENT_RETURN_VALUE);
  return u.toString();
}

/** Simpan flag callback sebelum guard login membuang query string. */
export function notePaymentReturnFromLocation() {
  if (typeof window === "undefined") return;
  try {
    const u = new URL(window.location.href);
    if (isPaymentReturnValue(u.searchParams.get(PAYMENT_RETURN_PARAM))) {
      sessionStorage.setItem(PAYMENT_RETURN_STORAGE, "1");
    }
    // Duitku menyisipkan resultCode di return URL (00 sukses / 01 pending / 02 batal).
    const rc = u.searchParams.get("resultCode") || u.searchParams.get("result_code");
    if (rc) sessionStorage.setItem(PAYMENT_RESULT_CODE_STORAGE, rc);
  } catch {
    /* private mode */
  }
}

export type PaymentReturnInfo = {
  returned: boolean;
  /** Kode hasil Duitku bila ada (00/01/02). */
  resultCode: string;
};

/** True sekali saat customer kembali dari PG. Membersihkan query + sessionStorage.
 * Tidak berarti pembayaran sukses — pakai confirmPaymentAfterReturn untuk verifikasi. */
export function consumePaymentReturn(): PaymentReturnInfo {
  notePaymentReturnFromLocation();
  let returned = false;
  let resultCode = "";
  try {
    returned = sessionStorage.getItem(PAYMENT_RETURN_STORAGE) === "1";
    if (returned) sessionStorage.removeItem(PAYMENT_RETURN_STORAGE);
    resultCode = String(sessionStorage.getItem(PAYMENT_RESULT_CODE_STORAGE) || "").trim();
    if (resultCode) sessionStorage.removeItem(PAYMENT_RESULT_CODE_STORAGE);
  } catch {
    /* ignore */
  }
  if (typeof window !== "undefined") {
    const u = new URL(window.location.href);
    if (isPaymentReturnValue(u.searchParams.get(PAYMENT_RETURN_PARAM))) {
      returned = true;
      u.searchParams.delete(PAYMENT_RETURN_PARAM);
    }
    const rc = u.searchParams.get("resultCode") || u.searchParams.get("result_code");
    if (rc) {
      if (!resultCode) resultCode = rc;
      u.searchParams.delete("resultCode");
      u.searchParams.delete("result_code");
    }
    u.searchParams.delete("merchantOrderId");
    u.searchParams.delete("reference");
    if (returned || rc) {
      const next = `${u.pathname}${u.search}${u.hash}`;
      window.history.replaceState(window.history.state, "", next);
    }
  }
  return { returned, resultCode };
}

/** @deprecated gunakan consumePaymentReturn(). */
export function consumePaymentReturnSuccess(): boolean {
  return consumePaymentReturn().returned;
}

function sleep(ms: number) {
  return new Promise((r) => setTimeout(r, ms));
}

function isRecentPaidAt(when?: string | null, withinMs = 30 * 60 * 1000): boolean {
  if (!when) return true; // tanpa timestamp, anggap kandidat (baru dari list)
  const t = Date.parse(when);
  if (!Number.isFinite(t)) return true;
  return Date.now() - t <= withinMs;
}

/**
 * Setelah kembali dari PG: poll status sampai lunas terkonfirmasi, atau batal.
 * Menangani kasus webhook sudah menandai lunas sebelum redirect (unpaid kosong).
 */
export async function confirmPaymentAfterReturn(opts: {
  resultCode?: string;
  fetchInvoices: () => Promise<PayableInvoice[]>;
  fetchPaymentIntent: (invoiceId: string) => Promise<{ status?: string } | null>;
  fetchPayments?: () => Promise<Array<{ status?: string; paid_at?: string | null; created_at?: string | null }>>;
  attempts?: number;
  delayMs?: number;
}): Promise<boolean> {
  const rc = String(opts.resultCode || "").trim();
  // Duitku: 02 = dibatalkan / gagal — jangan tampilkan sukses.
  if (rc === "02") return false;

  const attempts = Math.max(1, opts.attempts ?? 6);
  const delayMs = Math.max(200, opts.delayMs ?? 1500);

  let unpaidAtStart: string[] | null = null;

  for (let i = 0; i < attempts; i++) {
    if (i > 0) await sleep(delayMs);

    let invoices: PayableInvoice[] = [];
    try {
      invoices = await opts.fetchInvoices();
    } catch {
      continue;
    }

    const unpaid = invoices.filter(isInvoiceUnpaid);
    if (unpaidAtStart === null) {
      unpaidAtStart = unpaid.map((inv) => String(inv.id || "")).filter(Boolean);
    }

    for (const inv of unpaid) {
      if (!inv.id) continue;
      try {
        const pi = await opts.fetchPaymentIntent(inv.id);
        if (pi && isPaidStatus(pi.status)) return true;
      } catch {
        /* belum ada intent / belum lunas */
      }
    }

    let invoicesAfter: PayableInvoice[] = invoices;
    try {
      invoicesAfter = await opts.fetchInvoices();
    } catch {
      /* pakai snapshot sebelumnya */
    }
    const unpaidAfterIds = new Set(
      invoicesAfter.filter(isInvoiceUnpaid).map((inv) => String(inv.id || "")).filter(Boolean),
    );
    if (unpaidAtStart.some((id) => !unpaidAfterIds.has(id))) return true;

    // Webhook sudah selesai sebelum return → tidak ada unpaid di awal.
    if (unpaidAtStart.length === 0) {
      if (opts.fetchPayments) {
        try {
          const payments = await opts.fetchPayments();
          if (
            payments.some(
              (p) => isPaidStatus(p.status) && isRecentPaidAt(p.paid_at || p.created_at),
            )
          ) {
            return true;
          }
        } catch {
          /* ignore */
        }
      }
      // Petunjuk Duitku sukses + tidak ada tunggakan → anggap OK untuk UX.
      if (rc === "00" && invoicesAfter.some((inv) => inv.status === "paid" || !isInvoiceUnpaid(inv))) {
        return true;
      }
    }
  }

  return false;
}
