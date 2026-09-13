import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api";
import { ListToolbar, useDebouncedValue } from "../ListToolbar";
import { Section, Table } from "../ui";

type AuditRow = {
  id: string;
  action: string;
  entity_type?: string | null;
  entity_id?: string | null;
  metadata?: Record<string, unknown> | null;
  ip_address?: string | null;
  created_at: string;
  user_id?: string | null;
  user_email?: string | null;
};

const ACTION_LABELS: Record<string, string> = {
  "auth.login": "Login admin",
  "auth.login_failed": "Gagal login",
  "portal.login": "Login pelanggan",
  "invoice.pay": "Bayar tagihan",
  "invoice.issue_manual": "Terbitkan manual",
  "invoice.delete": "Hapus tagihan",
  "invoice.restore": "Pulihkan tagihan",
  "invoice.purge": "Hapus permanen",
  "customer.batch_status": "Ubah status massal",
  "customer.delete": "Hapus pelanggan",
  "customer.dismantle": "Cabut pelanggan",
  "notification.broadcast": "Broadcast",
  "notification.purge": "Hapus log notifikasi",
  "role.create": "Buat role",
  "role.update": "Ubah role",
  "role.delete": "Hapus role",
  "user.create": "Buat user",
  "user.delete": "Hapus user",
};

const ACTION_OPTIONS = Object.entries(ACTION_LABELS).map(([value, label]) => ({ value, label }));

function metaSummary(meta?: Record<string, unknown> | null): string {
  if (!meta) return "—";
  const parts: string[] = [];
  const push = (k: string, label?: string) => {
    const v = meta[k];
    if (v === undefined || v === null || v === "") return;
    parts.push(`${label || k}: ${String(v)}`);
  };
  push("email");
  push("phone");
  push("invoice_number", "tagihan");
  push("amount", "nominal");
  push("method", "metode");
  push("total", "total");
  push("customer_code", "kode");
  push("full_name", "nama");
  push("channel");
  push("audience");
  push("queued", "antre");
  push("batch_id", "batch");
  push("retention_days", "retensi");
  push("deleted", "dihapus");
  push("slug");
  push("role");
  push("is_active", "aktif");
  push("updated", "diubah");
  push("skipped_dismantled", "lewati cabut");
  push("failed", "gagal");
  push("items", "item");
  return parts.length > 0 ? parts.join(" · ") : "—";
}

function formatDateTime(s?: string | null): string {
  if (!s) return "—";
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString("id-ID", { dateStyle: "short", timeStyle: "short" });
}

export function AuditLogPage() {
  const [action, setAction] = useState("");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, 300);
  const [page, setPage] = useState(0);
  const [limit, setLimit] = useState(20);
  function setPageSize(n: number) {
    setLimit(n);
    setPage(0);
  }

  const q = useQuery({
    queryKey: ["audit-logs", action, debouncedSearch, page, limit],
    queryFn: () => {
      const params = new URLSearchParams({ limit: String(limit), offset: String(page * limit) });
      if (action) params.set("action", action);
      if (debouncedSearch.trim()) params.set("search", debouncedSearch.trim());
      return api<{ data: AuditRow[]; total: number }>(`/api/audit-logs?${params}`);
    },
  });

  const rows = q.data?.data ?? [];
  const total = q.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / limit));

  return (
    <Section title="Audit Log">
      <p className="mb-3 text-sm text-[var(--muted)]">
        Jejak aksi penting: login (berhasil/gagal), pembayaran, hapus/pulihkan tagihan, cabut/hapus pelanggan,
        broadcast, dan perubahan role/user. IP dicatat untuk penelusuran.
      </p>
      <ListToolbar
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(0);
        }}
        searchPlaceholder="Aksi, email user, isi…"
        filters={[
          {
            key: "action",
            label: "Aksi",
            value: action,
            onChange: (v) => {
              setAction(v);
              setPage(0);
            },
            options: ACTION_OPTIONS,
          },
        ]}
        page={page}
        pageCount={pages}
        onPageChange={setPage}
        total={total}
        pageSize={limit}
        onPageSizeChange={setPageSize}
      />
      <Table
        rowNumberStart={page * limit + 1}
        columns={["Waktu", "Aksi", "Pelaku", "IP", "Detail"]}
        rows={rows.map((r) => {
          const failed = r.action === "auth.login_failed";
          return [
            formatDateTime(r.created_at),
            <span key="a" style={failed ? { color: "var(--danger)", fontWeight: 600 } : undefined}>
              {ACTION_LABELS[r.action] || r.action}
            </span>,
            r.user_email || "—",
            r.ip_address || "—",
            <span key="d" title={r.metadata ? JSON.stringify(r.metadata) : undefined}>
              {metaSummary(r.metadata)}
            </span>,
          ];
        })}
      />
    </Section>
  );
}
