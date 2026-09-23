import { useMemo, useState } from "react";
import { IconBell } from "./icons";
import { IconButton } from "./ui";
import { formatRp } from "./ui";
import { formatDate } from "./tenantTime";
import { invoiceRemaining, isInvoiceUnpaid, isIsolirStatus } from "./payMethod";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export type ClientPageId = "home" | "plans" | "invoices" | "payments" | "tickets" | "account";

export type BellInvoice = {
  id: string;
  invoice_number: string;
  total_amount: number;
  paid_amount?: number;
  status: string;
  due_date?: string;
  customer_name?: string;
  customer_code?: string;
};

export type BellPayment = {
  amount: number;
  method: string;
  status: string;
  paid_at?: string;
  created_at?: string;
  invoice_number?: string;
  customer_name?: string;
  customer_code?: string;
};

export type BellSub = {
  id?: string;
  username: string;
  plan_name: string;
  status: string;
  customer_name?: string;
  customer_code?: string;
};

export type Item = {
  key: string;
  severity: "danger" | "warn" | "ok";
  title: string;
  message: string;
  at: string;
  page: ClientPageId;
};

const READ_KEY = "d5net_portal_notif_read";

function loadRead(): string[] {
  try {
    const raw = JSON.parse(localStorage.getItem(READ_KEY) || "[]") as unknown;
    return Array.isArray(raw) ? raw.filter((x): x is string => typeof x === "string").slice(-200) : [];
  } catch {
    return [];
  }
}

function fmtDate(iso?: string): string {
  return formatDate(iso, { day: "numeric", month: "short", year: "numeric" });
}

function timeAgoID(iso: string): string {
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "";
  const s = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (s < 60) return "baru saja";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} mnt lalu`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} jam lalu`;
  const d = Math.floor(h / 24);
  if (d < 7) return `${d} hari lalu`;
  return fmtDate(iso);
}

function severityColor(sev: Item["severity"]): string {
  switch (sev) {
    case "danger":
      return "var(--danger)";
    case "warn":
      return "var(--warn, #b7791f)";
    default:
      return "var(--ok, #2b9a66)";
  }
}

function paymentTime(p: BellPayment): number {
  const t = new Date(p.paid_at || p.created_at || "").getTime();
  return Number.isFinite(t) ? t : 0;
}

/** Bangun daftar item bell dari data tagihan, pembayaran, dan langganan. */
export function buildClientBellItems(
  invoices: BellInvoice[],
  payments: BellPayment[],
  subscriptions: BellSub[],
  multi: boolean,
): Item[] {
  const out: Item[] = [];
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  for (const s of subscriptions) {
    if (!isIsolirStatus(s.status)) continue;
    const who = multi && (s.customer_code || s.customer_name) ? ` • ${s.customer_code || s.customer_name}` : "";
    out.push({
      key: `sub-${s.id || s.username}`,
      severity: "danger",
      title: "Layanan diisolir",
      message: `${s.username}${s.plan_name ? ` · ${s.plan_name}` : ""}${who}. Bayar tagihan agar aktif kembali.`,
      at: "",
      page: "home",
    });
  }
  for (const i of invoices) {
    if (!isInvoiceUnpaid(i)) continue;
    const overdue = i.due_date ? new Date(i.due_date) < today : false;
    const who = multi && (i.customer_code || i.customer_name) ? ` • ${i.customer_code || i.customer_name}` : "";
    out.push({
      key: `inv-${i.id}`,
      severity: overdue ? "danger" : "warn",
      title: overdue ? "Tagihan lewat jatuh tempo" : "Tagihan belum dibayar",
      message: `${i.invoice_number} • ${formatRp(invoiceRemaining(i))} • jatuh tempo ${fmtDate(i.due_date)}${who}`,
      at: i.due_date || "",
      page: "invoices",
    });
  }
  // Pembayaran terbaru di atas. Data portal sudah terurut menurun, tapi kita
  // urutkan ulang agar tidak bergantung pada asumsi urutan dari API.
  const paid = payments
    .filter((p) => p.status === "paid")
    .slice()
    .sort((a, b) => paymentTime(b) - paymentTime(a))
    .slice(0, 5);
  for (const p of paid) {
    const when = p.paid_at || p.created_at || "";
    const who = multi && (p.customer_code || p.customer_name) ? ` • ${p.customer_code || p.customer_name}` : "";
    out.push({
      key: `pay-${p.invoice_number || ""}-${when}`,
      severity: "ok",
      title: "Pembayaran diterima",
      message: `${formatRp(p.amount)}${p.invoice_number ? ` • ${p.invoice_number}` : ""}${who}`,
      at: when,
      page: "payments",
    });
  }
  return out;
}

/** Bell notifikasi portal pelanggan: tagihan, pembayaran, isolir. Read-state lokal. */
export function ClientBell({
  invoices,
  payments,
  subscriptions,
  multi,
  onNavigatePage,
}: {
  invoices: BellInvoice[];
  payments: BellPayment[];
  subscriptions: BellSub[];
  multi: boolean;
  onNavigatePage: (page: ClientPageId) => void;
}) {
  const [read, setRead] = useState<string[]>(loadRead);
  const [open, setOpen] = useState(false);

  const items = useMemo<Item[]>(
    () => buildClientBellItems(invoices, payments, subscriptions, multi),
    [invoices, payments, subscriptions, multi],
  );

  const readSet = useMemo(() => new Set(read), [read]);
  const unread = items.filter((i) => !readSet.has(i.key)).length;

  function markRead(keys: string[]) {
    setRead((prev) => {
      const next = [...prev];
      for (const k of keys) {
        if (!next.includes(k)) next.push(k);
      }
      const trimmed = next.slice(-200);
      try {
        localStorage.setItem(READ_KEY, JSON.stringify(trimmed));
      } catch {
        /* abaikan */
      }
      return trimmed;
    });
  }

  function openItem(it: Item) {
    markRead([it.key]);
    onNavigatePage(it.page);
    setOpen(false);
  }

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <span className="relative inline-flex">
          <IconButton label={unread > 0 ? `Notifikasi (${unread} belum dibaca)` : "Notifikasi"}>
            <IconBell />
          </IconButton>
          {unread > 0 ? (
            <span
              className="pointer-events-none absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-[10px] font-bold text-white"
              style={{ background: "var(--danger)" }}
            >
              {unread > 99 ? "99+" : unread}
            </span>
          ) : null}
        </span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80 p-0">
        <div className="flex items-center justify-between border-b border-[var(--border)] px-3 py-2">
          <p className="text-sm font-semibold">Notifikasi</p>
          {unread > 0 ? (
            <button
              type="button"
              className="text-xs font-medium text-[var(--accent)] underline-offset-2 hover:underline"
              onClick={() => {
                markRead(items.map((i) => i.key));
                setOpen(false);
              }}
            >
              Tandai semua dibaca
            </button>
          ) : null}
        </div>
        <div className="max-h-80 overflow-y-auto">
          {items.length === 0 ? (
            <p className="px-3 py-4 text-sm text-[var(--muted)]">Tidak ada notifikasi.</p>
          ) : (
            items.map((it) => {
              const seen = readSet.has(it.key);
              return (
                <button
                  key={it.key}
                  type="button"
                  onClick={() => openItem(it)}
                  className={`flex w-full items-start gap-2.5 border-b border-[var(--border)] px-3 py-2.5 text-left last:border-0 hover:bg-[var(--panel-muted)] ${
                    seen ? "opacity-70" : ""
                  }`}
                >
                  <span
                    className="mt-1.5 h-2 w-2 shrink-0 rounded-full"
                    style={{ background: severityColor(it.severity) }}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="flex items-baseline justify-between gap-2">
                      <span className="truncate text-sm font-semibold">{it.title}</span>
                      {it.at ? (
                        <span className="shrink-0 text-[11px] text-[var(--muted)]">{timeAgoID(it.at)}</span>
                      ) : null}
                    </span>
                    <span className="mt-0.5 line-clamp-2 block text-xs text-[var(--muted)]">{it.message}</span>
                  </span>
                  {!seen ? (
                    <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full" style={{ background: "var(--accent)" }} />
                  ) : null}
                </button>
              );
            })
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
