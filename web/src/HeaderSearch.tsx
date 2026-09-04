import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { IconSearch } from "./icons";
import type { AdminPage } from "./AdminApp";

type SearchHit = {
  kind: string;
  id: string;
  title: string;
  subtitle?: string;
  page: string;
};

const kindLabel: Record<string, string> = {
  customer: "Pelanggan",
  subscription: "Langganan",
  invoice: "Tagihan",
  odp: "ODP",
  plan: "Paket",
  router: "Router",
  cluster: "Cluster",
  menu: "Menu",
};

const menuHits: { page: AdminPage; title: string; keywords: string }[] = [
  { page: "dashboard", title: "Dashboard", keywords: "dashboard overview" },
  { page: "customers", title: "Pelanggan", keywords: "pelanggan customer" },
  { page: "clusters", title: "Cluster / POP", keywords: "cluster pop site" },
  { page: "plans", title: "Paket", keywords: "paket plan harga" },
  { page: "subscriptions", title: "Langganan", keywords: "langganan subscription pppoe" },
  { page: "invoices", title: "Tagihan", keywords: "tagihan invoice" },
  { page: "routers", title: "Router", keywords: "router mikrotik" },
  { page: "ipam", title: "IP Pool", keywords: "ipam ip pool cidr gateway router" },
  { page: "odp", title: "ODP / FTTH", keywords: "odp ftth jalur kabel peta" },
  { page: "vouchers", title: "Voucher", keywords: "voucher hotspot" },
  { page: "tickets", title: "Tiket", keywords: "tiket ticket" },
  { page: "leads", title: "Lead", keywords: "lead prospek" },
  { page: "accounting", title: "Akunting", keywords: "akunting accounting laporan" },
  { page: "resellers", title: "Reseller", keywords: "reseller" },
  { page: "tech", title: "Teknisi", keywords: "teknisi tech" },
  { page: "branding", title: "Branding", keywords: "branding logo favicon app name settings" },
  { page: "roles", title: "Roles", keywords: "roles rbac permission settings" },
  { page: "users", title: "Users", keywords: "users staf portal pelanggan settings" },
];

function matchMenus(q: string): SearchHit[] {
  const n = q.toLowerCase();
  return menuHits
    .filter((m) => m.title.toLowerCase().includes(n) || m.keywords.includes(n))
    .slice(0, 5)
    .map((m) => ({
      kind: "menu",
      id: m.page,
      title: m.title,
      subtitle: "Buka halaman",
      page: m.page,
    }));
}

export function HeaderSearch({
  onNavigate,
}: {
  onNavigate: (page: AdminPage) => void;
}) {
  const [q, setQ] = useState("");
  const [debounced, setDebounced] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const t = window.setTimeout(() => setDebounced(q.trim()), 280);
    return () => window.clearTimeout(t);
  }, [q]);

  const searchQ = useQuery({
    queryKey: ["global-search", debounced],
    queryFn: () =>
      api<{ query: string; results: SearchHit[] }>(
        `/api/search?q=${encodeURIComponent(debounced)}&limit=5`,
      ),
    enabled: debounced.length >= 2,
  });

  const menuResults = debounced.length >= 1 ? matchMenus(debounced) : [];
  const apiResults = searchQ.data?.results ?? [];
  const results = [...menuResults, ...apiResults];
  const showPanel = open && (debounced.length >= 1 || q.length >= 1);

  useEffect(() => {
    setActive(0);
  }, [debounced, results.length]);

  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, []);

  function go(hit: SearchHit) {
    if (hit.page) onNavigate(hit.page as AdminPage);
    setQ("");
    setDebounced("");
    setOpen(false);
    inputRef.current?.blur();
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Escape") {
      setOpen(false);
      setQ("");
      return;
    }
    if (!showPanel || results.length === 0) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((i) => (i + 1) % results.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((i) => (i - 1 + results.length) % results.length);
    } else if (e.key === "Enter") {
      e.preventDefault();
      go(results[active] ?? results[0]);
    }
  }

  return (
    <div className="header-search hidden md:block" ref={rootRef}>
      <span className="header-search-icon">
        <IconSearch size={14} />
      </span>
      <input
        ref={inputRef}
        className="input header-search-input"
        placeholder="Cari pelanggan, ODP, tagihan…"
        value={q}
        onChange={(e) => {
          setQ(e.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={onKeyDown}
        aria-autocomplete="list"
        aria-expanded={showPanel}
        role="combobox"
      />
      {showPanel && (
        <div className="header-search-panel" role="listbox">
          {debounced.length >= 2 && searchQ.isFetching && (
            <p className="header-search-empty">Mencari…</p>
          )}
          {debounced.length >= 1 && !searchQ.isFetching && results.length === 0 && (
            <p className="header-search-empty">Tidak ada hasil untuk “{debounced}”</p>
          )}
          {results.map((hit, i) => (
            <button
              key={`${hit.kind}-${hit.id}`}
              type="button"
              role="option"
              aria-selected={i === active}
              className={`header-search-item${i === active ? " is-active" : ""}`}
              onMouseEnter={() => setActive(i)}
              onClick={() => go(hit)}
            >
              <span className="header-search-kind">{kindLabel[hit.kind] || hit.kind}</span>
              <span className="header-search-title">{hit.title}</span>
              {hit.subtitle ? <span className="header-search-sub">{hit.subtitle}</span> : null}
            </button>
          ))}
          {debounced.length === 1 && (
            <p className="header-search-hint">Ketik minimal 2 karakter untuk pencarian data</p>
          )}
        </div>
      )}
    </div>
  );
}
