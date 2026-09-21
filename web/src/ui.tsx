import {
  useEffect,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";
import { IconEye, IconEyeOff } from "./icons";
import { DEFAULT_BRAND_LOGO } from "./branding";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Table as UiTable, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

export { Button, Input, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Textarea };
export { SearchableSelect, type SearchableOption } from "./SearchableSelect";

/** Stat card for dashboard — keeps NeedMCP layout class. */
export function Card({ title, value, hint, onClick }: { title: string; value: string | number; hint?: string; onClick?: () => void }) {
  return (
    <div
      className={`stat-card${onClick ? " cursor-pointer transition-colors hover:border-[var(--accent)]" : ""}`}
      title={hint}
      onClick={onClick}
      role={onClick ? "button" : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={
        onClick
          ? (e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onClick();
              }
            }
          : undefined
      }
    >
      <div className="stat-card-label">{title}</div>
      <div className="stat-card-value">{value}</div>
      {hint ? <p className="mt-1 text-[10px] leading-snug text-[var(--muted)]">{hint}</p> : null}
    </div>
  );
}

export function Table({
  columns,
  rows,
  onRowClick,
  rowNumberStart = 1,
  hideRowNumber = false,
}: {
  columns: string[];
  rows: ReactNode[][];
  onRowClick?: (index: number) => void;
  /** Nomor urut baris pertama (untuk tabel ber-halaman: page * limit + 1). */
  rowNumberStart?: number;
  hideRowNumber?: boolean;
}) {
  const cols = hideRowNumber ? columns : ["No", ...columns];
  return (
    <UiTable>
      <TableHeader>
        <TableRow>
          {cols.map((c) => (
            <TableHead key={c}>{c}</TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.length === 0 ? (
          <TableRow>
            <TableCell colSpan={cols.length} className="text-[var(--muted)]">
              Belum ada data
            </TableCell>
          </TableRow>
        ) : (
          rows.map((r, i) => (
            <TableRow
              key={i}
              className={onRowClick ? "cursor-pointer" : undefined}
              onClick={onRowClick ? () => onRowClick(i) : undefined}
            >
              {!hideRowNumber ? (
                <TableCell key="no" className="w-10 tabular-nums text-[var(--muted)]">
                  {rowNumberStart + i}
                </TableCell>
              ) : null}
              {r.map((c, j) => (
                <TableCell key={j} onClick={j === r.length - 1 && onRowClick ? (e) => e.stopPropagation() : undefined}>
                  {c}
                </TableCell>
              ))}
            </TableRow>
          ))
        )}
      </TableBody>
    </UiTable>
  );
}

export function Section({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
        {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
      {children}
    </div>
  );
}

export function formatRp(n: number) {
  return new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", maximumFractionDigits: 0 }).format(n);
}

/** Format byte ke satuan manusiawi (KB/MB/GB) ala Indonesia, mis. 1500 → "1,46 KB". */
export function formatBytesID(n: number) {
  const v = Math.max(0, Number(n) || 0);
  const units = ["B", "KB", "MB", "GB", "TB"];
  let x = v;
  let u = 0;
  while (x >= 1024 && u < units.length - 1) {
    x /= 1024;
    u += 1;
  }
  const digits = u === 0 ? 0 : x >= 100 ? 0 : x >= 10 ? 1 : 2;
  const num = new Intl.NumberFormat("id-ID", {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(x);
  return `${num} ${units[u]}`;
}

/** Format bit-per-second ke satuan manusiawi, mis. 1500000 → "1,5 Mbps". */
export function formatBpsID(n: number) {
  const v = Math.max(0, Number(n) || 0);
  if (v <= 0) return "0 bps";
  const units = ["bps", "Kbps", "Mbps", "Gbps"];
  let x = v;
  let u = 0;
  while (x >= 1000 && u < units.length - 1) {
    x /= 1000;
    u += 1;
  }
  const digits = x >= 100 ? 0 : x >= 10 ? 1 : 2;
  const num = new Intl.NumberFormat("id-ID", {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(x);
  return `${num} ${units[u]}`;
}

const INVOICE_STATUS_ID: Record<string, string> = {
  paid: "Sudah bayar",
  issued: "Belum bayar",
  unpaid: "Belum bayar",
  pending: "Belum bayar",
  partial: "Bayar sebagian",
  overdue: "Jatuh tempo",
  void: "Dibatalkan",
  cancelled: "Dibatalkan",
  canceled: "Dibatalkan",
  draft: "Draf",
};

const PAYMENT_STATUS_ID: Record<string, string> = {
  paid: "Berhasil",
  success: "Berhasil",
  pending: "Menunggu",
  failed: "Gagal",
  expired: "Kedaluwarsa",
  cancelled: "Dibatalkan",
  canceled: "Dibatalkan",
  void: "Dibatalkan",
};

export function invoiceStatusLabel(status?: string | null) {
  const key = String(status || "").trim().toLowerCase();
  return INVOICE_STATUS_ID[key] || status || "—";
}

export function paymentStatusLabel(status?: string | null) {
  const key = String(status || "").trim().toLowerCase();
  return PAYMENT_STATUS_ID[key] || status || "—";
}

const SUBSCRIPTION_STATUS_ID: Record<string, string> = {
  active: "Aktif",
  suspended: "Isolir",
  isolir: "Isolir",
  overdue: "Tunggakan",
  cancelled: "Dibatalkan",
  canceled: "Dibatalkan",
  pending: "Menunggu",
  expired: "Kedaluwarsa",
};

export function subscriptionStatusLabel(status?: string | null) {
  const key = String(status || "").trim().toLowerCase();
  return SUBSCRIPTION_STATUS_ID[key] || status || "—";
}

const TICKET_STATUS_ID: Record<string, string> = {
  open: "Menunggu",
  in_progress: "Sedang diproses",
  resolved: "Selesai",
  closed: "Ditutup",
  cancelled: "Dibatalkan",
  canceled: "Dibatalkan",
};

const TICKET_STATUS_HINT: Record<string, string> = {
  open: "Keluhan sudah kami terima. Tim akan menindaklanjuti.",
  in_progress: "Tim sedang menangani keluhan ini.",
  resolved: "Keluhan sudah diselesaikan.",
  closed: "Tiket ini sudah ditutup.",
  cancelled: "Keluhan ini dibatalkan.",
  canceled: "Keluhan ini dibatalkan.",
};

export function ticketStatusLabel(status?: string | null) {
  const key = String(status || "").trim().toLowerCase();
  return TICKET_STATUS_ID[key] || status || "—";
}

export function ticketStatusHint(status?: string | null) {
  const key = String(status || "").trim().toLowerCase();
  return TICKET_STATUS_HINT[key] || "";
}

export function ticketStatusTone(status?: string | null): "wait" | "progress" | "done" | "stop" {
  switch (String(status || "").trim().toLowerCase()) {
    case "open":
      return "wait";
    case "in_progress":
      return "progress";
    case "resolved":
    case "closed":
      return "done";
    default:
      return "stop";
  }
}

export function LoginShell({
  brand = "Delima Net",
  title,
  subtitle,
  logoUrl,
  badge,
  footer,
  visual,
  children,
}: {
  brand?: string;
  title: string;
  subtitle: string;
  logoUrl?: string | null;
  badge?: ReactNode;
  footer?: ReactNode;
  /** Bila diisi: layout split startup (panel visual + form). */
  visual?: ReactNode;
  children: ReactNode;
}) {
  const card = (
    <div className="auth-card">
      <div className="auth-card-bar" aria-hidden />
      <div className="auth-card-body">
        <div className="text-center">
          <a href="/" className="auth-logo-link" title="Ke halaman utama" aria-label="Ke halaman utama">
            <img src={logoUrl || DEFAULT_BRAND_LOGO} alt="" className="auth-logo" />
          </a>
          <div className="auth-brand">{brand}</div>
          <h1 className="auth-title">{title}</h1>
          {badge ? <span className="auth-badge">{badge}</span> : null}
          <p className="auth-subtitle">{subtitle}</p>
        </div>
        {children}
        {footer ? <div className="auth-footer">{footer}</div> : null}
      </div>
    </div>
  );
  if (visual) {
    return (
      <div className="auth-split">
        <aside className="auth-visual">
          <div className="auth-visual-inner">{visual}</div>
        </aside>
        <div className="auth-pane">
          <div className="auth-glow" aria-hidden />
          {card}
        </div>
      </div>
    );
  }
  return (
    <div className="auth-wrap">
      <div className="auth-glow" aria-hidden />
      {card}
    </div>
  );
}

/** Infer hover tone from Indonesian action labels so row icons feel distinct. */
export type IconActionTone = "neutral" | "accent" | "secondary" | "success" | "danger";

export function iconActionTone(label: string, danger?: boolean): IconActionTone {
  const t = label.toLowerCase();
  if (danger || /\b(hapus|delete|void|batal|nonaktif|buang)\b/.test(t)) return "danger";
  if (/\b(aktifkan|tambah secret|secrets|secret|plug|pasang)\b/.test(t)) return "success";
  if (/\b(ganti paket|sync|refresh|test|backup|restore|export|import|download|upload)\b/.test(t)) {
    return "secondary";
  }
  if (/\b(edit|ubah|pensil|kelola|lihat|assign|convert|tandai|bayar)\b/.test(t)) return "accent";
  return "neutral";
}

/** Compact icon-only action; always pass label for tooltip + a11y. */
export function IconButton({
  label,
  children,
  danger,
  tone,
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  danger?: boolean;
  tone?: IconActionTone;
}) {
  const resolved = tone ?? iconActionTone(label, danger);
  return (
    <Button
      type="button"
      variant={resolved === "danger" ? "danger" : "ghost"}
      size="icon"
      title={label}
      aria-label={label}
      className={cn("icon-action", `icon-action--${resolved}`, className)}
      {...props}
    >
      {children}
    </Button>
  );
}

export function IconLink({
  label,
  href,
  children,
  className = "",
  tone,
}: {
  label: string;
  href: string;
  children: ReactNode;
  className?: string;
  tone?: IconActionTone;
}) {
  const resolved = tone ?? iconActionTone(label);
  return (
    <a
      href={href}
      title={label}
      aria-label={label}
      className={cn(
        "icon-action",
        `icon-action--${resolved}`,
        "inline-flex h-8 w-8 items-center justify-center rounded-md",
        className,
      )}
    >
      {children}
    </a>
  );
}

/** Password / secret field with show-hide (vision) toggle. */
export function SecretInput({
  className = "",
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type">) {
  const [visible, setVisible] = useState(false);
  return (
    <div className={cn("secret-input", className)}>
      <Input {...props} type={visible ? "text" : "password"} autoComplete={props.autoComplete ?? "current-password"} />
      <button
        type="button"
        className="secret-input-toggle"
        title={visible ? "Sembunyikan" : "Tampilkan"}
        aria-label={visible ? "Sembunyikan" : "Tampilkan"}
        aria-pressed={visible}
        onClick={() => setVisible((v) => !v)}
        tabIndex={-1}
      >
        {visible ? <IconEyeOff /> : <IconEye />}
      </button>
    </div>
  );
}

export function OnlineBadge({ online, title }: { online: boolean; title?: string }) {
  return (
    <Badge title={title} variant={online ? "success" : "danger"}>
      {online ? "ONLINE" : "OFFLINE"}
    </Badge>
  );
}

export function StatusDialog({
  open,
  ok,
  title,
  message,
  onClose,
}: {
  open: boolean;
  ok: boolean;
  title: string;
  message: string;
  onClose: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent hideClose className="max-w-md">
        <DialogHeader>
          <div className={`dialog-status ${ok ? "is-ok" : "is-err"}`}>{ok ? "ONLINE" : "OFFLINE"}</div>
          <DialogTitle className="mt-3">{title}</DialogTitle>
          <DialogDescription className="whitespace-pre-wrap break-words">{message}</DialogDescription>
        </DialogHeader>
        <Button type="button" className="mt-2 w-full" onClick={onClose}>
          Tutup
        </Button>
      </DialogContent>
    </Dialog>
  );
}

/** Modal form shell — use with +Tambah so create UI is not always inline.
 * Does not close on backdrop click. Header is draggable. Esc closes.
 */
export function FormDialog({
  open,
  title,
  onClose,
  children,
  wide,
  asPage,
}: {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
  /** Render as in-page panel instead of modal (e.g. dedicated create route). */
  asPage?: boolean;
}) {
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const drag = useRef<{ startX: number; startY: number; origX: number; origY: number } | null>(null);

  useEffect(() => {
    if (open) setOffset({ x: 0, y: 0 });
  }, [open]);

  useEffect(() => {
    if (!open || asPage) return;
    const onMove = (e: PointerEvent) => {
      if (!drag.current) return;
      setOffset({
        x: drag.current.origX + (e.clientX - drag.current.startX),
        y: drag.current.origY + (e.clientY - drag.current.startY),
      });
    };
    const onUp = () => {
      drag.current = null;
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    return () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
  }, [open, asPage]);

  if (asPage) {
    if (!open) return null;
    return (
      <div className={cn("panel-card p-5", wide ? "max-w-3xl" : "max-w-xl")}>
        <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
          <h2 className="text-base font-semibold text-[var(--text)]">{title}</h2>
          <button type="button" className="btn-ghost" onClick={onClose}>
            Batal
          </button>
        </div>
        {children}
      </div>
    );
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent
        hideClose
        className={cn(wide ? "max-w-2xl" : "max-w-md", "gap-0 overflow-y-auto p-6")}
        style={{ transform: `translate(calc(-50% + ${offset.x}px), calc(-50% + ${offset.y}px))` }}
      >
        <div
          className="dialog-drag-handle mb-4 flex cursor-grab items-start justify-between gap-3 active:cursor-grabbing"
          onPointerDown={(e) => {
            if ((e.target as HTMLElement).closest("button")) return;
            drag.current = {
              startX: e.clientX,
              startY: e.clientY,
              origX: offset.x,
              origY: offset.y,
            };
          }}
        >
          <DialogTitle className="select-none">{title}</DialogTitle>
          <Button type="button" variant="ghost" size="sm" className="px-2 py-1 text-lg leading-none" aria-label="Tutup" title="Tutup" onClick={onClose}>
            ×
          </Button>
        </div>
        {children}
      </DialogContent>
    </Dialog>
  );
}
