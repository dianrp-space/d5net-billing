// Zona waktu tenant (Pengaturan → Umum, mis. "Asia/Jakarta").
// Roots (AdminApp, ClientHome) mengisinya sekali dari /api/public/branding;
// semua format tanggal di UI mengikuti zona ini. Fallback = zona browser
// (perilaku lama) bila belum di-set atau zona invalid.
let tenantTimeZone = "";

export function setTenantTimeZone(tz: unknown) {
  const s = String(tz ?? "").trim();
  if (!s || s === tenantTimeZone) return;
  try {
    new Intl.DateTimeFormat("id-ID", { timeZone: s });
    tenantTimeZone = s;
  } catch {
    // Abaikan zona invalid — tetap pakai zona browser.
  }
}

function tzOpts(extra?: Intl.DateTimeFormatOptions): Intl.DateTimeFormatOptions | undefined {
  if (!tenantTimeZone) return extra;
  return { ...extra, timeZone: tenantTimeZone };
}

function toDate(input?: string | Date | null): Date | null {
  if (!input) return null;
  const d = input instanceof Date ? input : new Date(input);
  return Number.isNaN(d.getTime()) ? null : d;
}

/** Datetime "id-ID" mengikuti zona tenant. */
export function formatDateTime(input?: string | Date | null, opts?: Intl.DateTimeFormatOptions): string {
  if (input === undefined || input === null || input === "") return "—";
  try {
    const d = toDate(input);
    return d ? d.toLocaleString("id-ID", tzOpts(opts)) : String(input);
  } catch {
    return String(input);
  }
}

/** Date-only "id-ID" mengikuti zona tenant; opts opsional (day/month/year …). */
export function formatDate(input?: string | Date | null, opts?: Intl.DateTimeFormatOptions): string {
  if (input === undefined || input === null || input === "") return "—";
  try {
    const d = toDate(input);
    return d ? d.toLocaleDateString("id-ID", tzOpts(opts)) : String(input);
  } catch {
    return String(input);
  }
}

/** Datetime ringkas (short date + short time) mengikuti zona tenant. */
export function formatDateTimeShort(input?: string | Date | null): string {
  if (!input) return "—";
  const d = toDate(input);
  if (!d) return "—";
  return d.toLocaleString("id-ID", tzOpts({ dateStyle: "short", timeStyle: "short" }));
}
