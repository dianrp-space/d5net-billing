import { useEffect, useState, type ReactNode } from "react";
import { Input, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Button } from "./ui";

export function useDebouncedValue<T>(value: T, delayMs = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(t);
  }, [value, delayMs]);
  return debounced;
}

/** Opsi jumlah baris per halaman untuk select "Tampilkan". */
export const PAGE_SIZE_OPTIONS = [10, 20, 25, 50, 100];

/** Client-side pagination for already-loaded lists (slices into pages). */
export function usePagination<T>(items: T[], defaultPageSize = 25) {
  const [page, setPage] = useState(0);
  const [pageSize, setPageSizeState] = useState(defaultPageSize);
  const pageCount = Math.max(1, Math.ceil(items.length / pageSize));
  const safePage = Math.min(page, pageCount - 1);
  useEffect(() => {
    if (page > pageCount - 1) setPage(pageCount - 1);
  }, [page, pageCount]);
  function setPageSize(n: number) {
    setPageSizeState(n);
    setPage(0);
  }
  const pageItems = items.slice(safePage * pageSize, safePage * pageSize + pageSize);
  return { page, setPage, pageCount, pageItems, pageSize, setPageSize };
}

export type ListFilterOption = { value: string; label: string };

export type ListFilterSpec = {
  key: string;
  label: string;
  value: string;
  options: ListFilterOption[];
  onChange: (value: string) => void;
  /** Select value that means “all” (cleared). Default __all__ */
  allValue?: string;
  className?: string;
};

/** Shared search + select filters + optional pagination for admin tables. */
export function ListToolbar({
  search,
  onSearchChange,
  searchPlaceholder = "Cari…",
  filters = [],
  page,
  pageCount,
  onPageChange,
  total,
  pageSize,
  pageSizeOptions = PAGE_SIZE_OPTIONS,
  onPageSizeChange,
  children,
}: {
  search?: string;
  onSearchChange?: (v: string) => void;
  searchPlaceholder?: string;
  filters?: ListFilterSpec[];
  page?: number;
  pageCount?: number;
  onPageChange?: (page: number) => void;
  total?: number;
  /** Jumlah baris per halaman (tampilkan select "Tampilkan" bila onPageSizeChange ada). */
  pageSize?: number;
  pageSizeOptions?: number[];
  onPageSizeChange?: (size: number) => void;
  children?: ReactNode;
}) {
  const showPager =
    typeof page === "number" &&
    typeof pageCount === "number" &&
    pageCount > 1 &&
    typeof onPageChange === "function";
  const showPageSize = typeof onPageSizeChange === "function" && typeof pageSize === "number";

  return (
    <div className="mb-4 space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        {onSearchChange ? (
          <div className="min-w-[200px] flex-1 basis-[220px]">
            <Label className="mb-1.5 block">Cari</Label>
            <Input
              value={search ?? ""}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder={searchPlaceholder}
              autoComplete="off"
            />
          </div>
        ) : null}
        {showPageSize ? (
          <div className="min-w-[130px]">
            <Label className="mb-1.5 block">Tampilkan</Label>
            <Select value={String(pageSize)} onValueChange={(v) => onPageSizeChange!(Number(v) || pageSize!)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {pageSizeOptions.map((n) => (
                  <SelectItem key={n} value={String(n)}>
                    {n} data
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ) : null}
        {filters.map((f) => {
          const all = f.allValue ?? "__all__";
          return (
            <div key={f.key} className={f.className ?? "min-w-[160px]"}>
              <Label className="mb-1.5 block">{f.label}</Label>
              <Select
                value={f.value || all}
                onValueChange={(v) => f.onChange(v === all ? "" : v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={all}>Semua</SelectItem>
                  {f.options.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          );
        })}
        {children}
      </div>
      {showPager || total != null ? (
        <div className="flex flex-wrap items-center justify-between gap-2 text-sm text-[var(--muted)]">
          <span>{total != null ? `${total} data` : null}</span>
          {showPager ? (
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page! <= 0}
                onClick={() => onPageChange!(page! - 1)}
              >
                Sebelumnya
              </Button>
              <span>
                Hal. {page! + 1}/{pageCount}
              </span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page! + 1 >= pageCount!}
                onClick={() => onPageChange!(page! + 1)}
              >
                Berikutnya
              </Button>
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

/** Case-insensitive match against joined string fields. */
export function matchesQuery(q: string, ...parts: Array<string | number | null | undefined>): boolean {
  const needle = q.trim().toLowerCase();
  if (!needle) return true;
  return parts.some((p) => String(p ?? "").toLowerCase().includes(needle));
}
