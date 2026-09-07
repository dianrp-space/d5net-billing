import {
  useEffect,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";
import { IconEye, IconEyeOff } from "./icons";
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
export function Card({ title, value, hint }: { title: string; value: string | number; hint?: string }) {
  return (
    <div className="stat-card" title={hint}>
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
}: {
  columns: string[];
  rows: ReactNode[][];
  onRowClick?: (index: number) => void;
}) {
  return (
    <UiTable>
      <TableHeader>
        <TableRow>
          {columns.map((c) => (
            <TableHead key={c}>{c}</TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.length === 0 ? (
          <TableRow>
            <TableCell colSpan={columns.length} className="text-[var(--muted)]">
              Belum ada data
            </TableCell>
          </TableRow>
        ) : (
          rows.map((r, i) => (
            <TableRow
              key={i}
              className={onRowClick ? "cursor-pointer hover:bg-[var(--panel-muted)]/60" : undefined}
              onClick={onRowClick ? () => onRowClick(i) : undefined}
            >
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

export function LoginShell({
  brand = "drp-billing",
  title,
  subtitle,
  logoUrl,
  children,
}: {
  brand?: string;
  title: string;
  subtitle: string;
  logoUrl?: string | null;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-full items-center justify-center bg-[var(--bg)] p-6">
      <div className="w-full max-w-md rounded-2xl border border-[var(--border)] bg-[var(--panel)] p-6 shadow-sm">
        <div className="mb-4 flex items-center gap-3">
          {logoUrl ? (
            <img src={logoUrl} alt="" className="h-10 w-10 rounded-lg object-contain" />
          ) : null}
          <div className="min-w-0">
            <div className="text-sm tracking-wide text-[var(--muted)]">{brand}</div>
            <h1 className="text-xl font-bold tracking-tight">{title}</h1>
          </div>
        </div>
        <p className="mb-6 text-sm text-[var(--muted)]">{subtitle}</p>
        {children}
      </div>
    </div>
  );
}

/** Compact icon-only action; always pass label for tooltip + a11y. */
export function IconButton({
  label,
  children,
  danger,
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  danger?: boolean;
}) {
  return (
    <Button
      type="button"
      variant={danger ? "danger" : "ghost"}
      size="icon"
      title={label}
      aria-label={label}
      className={cn(className)}
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
}: {
  label: string;
  href: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <a
      href={href}
      title={label}
      aria-label={label}
      className={cn(
        "inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border)] bg-transparent text-[var(--muted)] transition-colors hover:bg-[var(--panel-muted)] hover:text-[var(--text)] hover:border-[var(--accent)]",
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
}: {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const drag = useRef<{ startX: number; startY: number; origX: number; origY: number } | null>(null);

  useEffect(() => {
    if (open) setOffset({ x: 0, y: 0 });
  }, [open]);

  useEffect(() => {
    if (!open) return;
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
  }, [open]);

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent
        hideClose
        className={cn(wide ? "max-w-2xl" : "max-w-md", "gap-0 overflow-visible p-6")}
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
