import { useEffect, useId, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/utils";

export type SearchableOption = {
  value: string;
  label: string;
  /** Extra text matched by search (codes, aliases, etc.) */
  keywords?: string;
};

type SearchableSelectProps = {
  options: SearchableOption[];
  value: string;
  onValueChange: (value: string) => void;
  placeholder?: string;
  searchPlaceholder?: string;
  emptyText?: string;
  /** Show a clear row that sets value to "" */
  allowClear?: boolean;
  clearLabel?: string;
  disabled?: boolean;
  required?: boolean;
  className?: string;
  id?: string;
};

function normalize(s: string) {
  return s.trim().toLowerCase();
}

function matches(opt: SearchableOption, q: string) {
  if (!q) return true;
  const hay = normalize(`${opt.label} ${opt.keywords || ""} ${opt.value}`);
  return hay.includes(q);
}

/**
 * Combobox-style select with type-to-filter — for long lists (pelanggan, ODP, router, …).
 * Renders the panel in-place (not portaled) so it stays interactive inside FormDialog:
 * Radix Dialog blocks pointer events on body-portaled content via onPointerDownOutside.
 */
export function SearchableSelect({
  options,
  value,
  onValueChange,
  placeholder = "Pilih…",
  searchPlaceholder = "Ketik untuk mencari…",
  emptyText = "Tidak ada hasil",
  allowClear = false,
  clearLabel = "— Kosongkan —",
  disabled = false,
  required = false,
  className,
  id,
}: SearchableSelectProps) {
  const autoId = useId();
  const triggerId = id || autoId;
  const rootRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [openUp, setOpenUp] = useState(false);

  const selected = useMemo(() => options.find((o) => o.value === value), [options, value]);
  const filtered = useMemo(() => {
    const q = normalize(query);
    return options.filter((o) => matches(o, q));
  }, [options, query]);

  function placePanel() {
    const el = rootRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const spaceBelow = window.innerHeight - rect.bottom;
    setOpenUp(spaceBelow < 260 && rect.top > spaceBelow);
  }

  useEffect(() => {
    if (!open) return;
    placePanel();
    const t = window.setTimeout(() => searchRef.current?.focus(), 0);
    const onScroll = () => placePanel();
    const onResize = () => placePanel();
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onResize);
    return () => {
      window.clearTimeout(t);
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onResize);
    };
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node;
      if (rootRef.current?.contains(t)) return;
      setOpen(false);
      setQuery("");
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        setQuery("");
      }
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  function pick(v: string) {
    onValueChange(v);
    setOpen(false);
    setQuery("");
  }

  return (
    <div ref={rootRef} className={cn("relative z-0 min-w-0 w-full", open && "z-30", className)}>
      {required ? <input type="hidden" value={value} required readOnly /> : null}
      <button
        id={triggerId}
        type="button"
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={open}
        className={cn(
          "flex h-9 w-full min-w-0 items-center justify-between gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-3 text-left text-sm text-[var(--text)] shadow-sm transition-colors",
          "hover:bg-[var(--panel)] focus-visible:outline-none focus-visible:border-[var(--accent)] focus-visible:ring-[3px] focus-visible:ring-[var(--focus-ring)]",
          "disabled:cursor-not-allowed disabled:opacity-50",
        )}
        onClick={() => {
          if (disabled) return;
          setOpen((v) => !v);
          setQuery("");
        }}
      >
        <span className={cn("min-w-0 flex-1 truncate", !selected && "text-[var(--muted)]")}>
          {selected?.label || placeholder}
        </span>
        <span className="shrink-0 text-[var(--muted)]" aria-hidden>
          ▾
        </span>
      </button>

      {open ? (
        <div
          data-searchable-select-panel
          className={cn(
            "absolute left-0 right-0 z-40 flex max-h-[min(280px,70vh)] flex-col overflow-hidden rounded-md border border-[var(--border)] bg-[var(--panel)] shadow-lg",
            openUp ? "bottom-[calc(100%+4px)]" : "top-[calc(100%+4px)]",
          )}
          role="listbox"
        >
          <div className="shrink-0 border-b border-[var(--border)] p-2">
            <input
              ref={searchRef}
              type="search"
              className="input w-full min-w-0 text-sm"
              placeholder={searchPlaceholder}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                e.stopPropagation();
                if (e.key === "Enter") {
                  e.preventDefault();
                  if (filtered[0]) pick(filtered[0].value);
                }
              }}
              onClick={(e) => e.stopPropagation()}
              autoComplete="off"
            />
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-1">
            {allowClear ? (
              <button
                type="button"
                className={cn(
                  "flex w-full rounded px-2 py-1.5 text-left text-sm text-[var(--muted)] hover:bg-[var(--panel-muted)]",
                  !value && "bg-[var(--panel-muted)] text-[var(--text)]",
                )}
                onClick={() => pick("")}
              >
                {clearLabel}
              </button>
            ) : null}
            {filtered.length === 0 ? (
              <p className="px-2 py-3 text-center text-xs text-[var(--muted)]">{emptyText}</p>
            ) : (
              filtered.map((o) => (
                <button
                  key={o.value}
                  type="button"
                  role="option"
                  aria-selected={o.value === value}
                  className={cn(
                    "flex w-full rounded px-2 py-1.5 text-left text-sm text-[var(--text)] hover:bg-[var(--panel-muted)]",
                    o.value === value && "bg-[var(--accent)]/15 font-medium",
                  )}
                  onClick={() => pick(o.value)}
                >
                  <span className="min-w-0 break-words">{o.label}</span>
                </button>
              ))
            )}
          </div>
        </div>
      ) : null}
    </div>
  );
}
