import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { IconSearch } from "./icons";
import type { AdminPage } from "./AdminApp";
import { canAccessPage } from "./permissions";

type SearchHit = {
  kind: string;
  id: string;
  title: string;
  subtitle?: string;
  page: string;
};

const kindLabel: Record<string, string> = {
  customer: "Pelanggan",
  subscription: "Secrets",
  invoice: "Tagihan",
  odp: "ODP",
  plan: "Paket",
  router: "Router",
  cluster: "Cluster",
  menu: "Menu",
};

const menuHits: { page: AdminPage; title: string; keywords: string }[] = [
  { page: "dashboard", title: "Dashboard", keywords: "dashboard overview" },
  { page: "customers", title: "Pelanggan", keywords: "pelanggan customer secret langganan subscription pppoe" },
  { page: "clusters", title: "Cluster / POP", keywords: "cluster pop site" },
  { page: "plans", title: "Paket", keywords: "paket plan harga grace jatuh tempo tenggang" },
  { page: "invoices", title: "Tagihan", keywords: "tagihan invoice" },
  { page: "routers", title: "Router", keywords: "router mikrotik" },
  { page: "ip-pool", title: "IP Pool", keywords: "ipam ip pool cidr gateway router cluster" },
  { page: "odp", title: "MAP FTTH", keywords: "odp ftth jalur kabel peta map" },
  { page: "coverage", title: "Coverage", keywords: "coverage jangkauan radius pop odp sales peta km" },
  { page: "vouchers", title: "Voucher", keywords: "voucher hotspot" },
  { page: "tickets", title: "Tiket", keywords: "tiket ticket teknisi instalasi wo" },
  { page: "sla-report", title: "Laporan SLA", keywords: "sla laporan gangguan tiket resolve waktu" },
  { page: "leads", title: "Lead", keywords: "lead prospek" },
  { page: "accounting", title: "Akunting", keywords: "akunting accounting laporan" },
  { page: "resellers", title: "Reseller & Komisi", keywords: "reseller komisi commission agen" },
  { page: "general", title: "Umum", keywords: "umum general tenant nama pajak timezone logo favicon branding warna primary tombol grace tenggang isolir siklus prorata tanggal awal" },
  { page: "isolir-template", title: "Template Isolir", keywords: "isolir captive pool profil firewall nat redirect" },
  { page: "jobs", title: "Cronjob", keywords: "cron job worker jadwal dunning reconcile laporan billing isolir grace tenggang poller mikrotik" },
  { page: "notifications", title: "Notifikasi", keywords: "notifikasi broadcast dunning promo whatsapp delay" },
  { page: "roles", title: "Roles", keywords: "roles rbac permission settings" },
  { page: "users", title: "Users", keywords: "users staf portal pelanggan settings" },
  { page: "webhooks", title: "Webhook", keywords: "webhook outbound integrasi n8n" },
  { page: "payment-gw", title: "Payment Gateway", keywords: "payment gateway drp qris integrasi" },
  { page: "messaging-gw", title: "Messaging Gateway", keywords: "whatsapp telegram email smtp notifikasi messaging whatsmeow integrasi" },
  { page: "backup", title: "Backup / Restore", keywords: "backup restore database pg_dump export import settings" },
];

function matchMenus(q: string, allowedPages?: string[] | null): SearchHit[] {
  const n = q.toLowerCase();
  return menuHits
    .filter((m) => canAccessPage(allowedPages, m.page))
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
  allowedPages,
}: {
  onNavigate: (page: AdminPage, rest?: string[]) => void;
  allowedPages?: string[] | null;
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

  const menuResults = debounced.length >= 1 ? matchMenus(debounced, allowedPages) : [];
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
    if ((hit.kind === "subscription" || hit.kind === "customer") && hit.id) {
      onNavigate("customers", [hit.id, "secrets"]);
    } else if (hit.page) {
      onNavigate(hit.page as AdminPage);
    }
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
