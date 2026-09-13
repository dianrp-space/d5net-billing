import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { IconBell } from "./icons";
import { IconButton } from "./ui";
import type { AdminPage } from "./admin/pages";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export type BellAlert = {
  id: string;
  severity: string;
  kind: string;
  title: string;
  message: string;
  entity_type?: string | null;
  entity_id?: string | null;
  is_acked: boolean;
  created_at: string;
};

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
  return new Date(t).toLocaleDateString("id-ID", { day: "numeric", month: "short", year: "numeric" });
}

function severityColor(sev: string): string {
  switch (sev.toLowerCase()) {
    case "critical":
      return "var(--danger)";
    case "warn":
    case "warning":
      return "var(--warn, #b7791f)";
    default:
      return "var(--accent)";
  }
}

/** Bell notifikasi header: badge unread + dropdown daftar alert. */
export function AlertsBell({ onNavigatePage }: { onNavigatePage?: (page: AdminPage) => void }) {
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const q = useQuery({
    queryKey: ["alerts"],
    queryFn: () =>
      api<{ data: BellAlert[]; unread_count: number }>("/api/alerts?limit=20"),
    refetchInterval: 30_000,
    refetchOnWindowFocus: true,
    retry: false,
  });

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["alerts"] });
  };

  const ackOne = useMutation({
    mutationFn: (id: string) => api(`/api/alerts/${encodeURIComponent(id)}/ack`, { method: "POST" }),
    onSuccess: invalidate,
  });

  const ackAll = useMutation({
    mutationFn: () => api("/api/alerts/ack-all", { method: "POST" }),
    onSuccess: invalidate,
  });

  const items = Array.isArray(q.data?.data) ? q.data.data : [];
  const unread = q.data?.unread_count ?? 0;

  function openAlert(a: BellAlert) {
    if (!a.is_acked) ackOne.mutate(a.id);
    if (a.entity_type === "router" && onNavigatePage) {
      onNavigatePage("routers");
    } else if (a.entity_type === "ticket" && onNavigatePage) {
      onNavigatePage("tickets");
    } else if (a.entity_type === "lead" && onNavigatePage) {
      onNavigatePage("leads");
    }
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
              disabled={ackAll.isPending}
              onClick={() => {
                ackAll.mutate();
                setOpen(false);
              }}
            >
              Tandai semua dibaca
            </button>
          ) : null}
        </div>
        <div className="max-h-80 overflow-y-auto">
          {q.isLoading ? (
            <p className="px-3 py-4 text-sm text-[var(--muted)]">Memuat...</p>
          ) : items.length === 0 ? (
            <p className="px-3 py-4 text-sm text-[var(--muted)]">Tidak ada notifikasi.</p>
          ) : (
            items.map((a) => (
              <button
                key={a.id}
                type="button"
                onClick={() => openAlert(a)}
                className={`flex w-full items-start gap-2.5 border-b border-[var(--border)] px-3 py-2.5 text-left last:border-0 hover:bg-[var(--panel-muted)] ${
                  a.is_acked ? "opacity-70" : ""
                }`}
              >
                <span
                  className="mt-1.5 h-2 w-2 shrink-0 rounded-full"
                  style={{ background: severityColor(a.severity) }}
                />
                <span className="min-w-0 flex-1">
                  <span className="flex items-baseline justify-between gap-2">
                    <span className="truncate text-sm font-semibold">{a.title}</span>
                    <span className="shrink-0 text-[11px] text-[var(--muted)]">{timeAgoID(a.created_at)}</span>
                  </span>
                  {a.message ? (
                    <span className="mt-0.5 line-clamp-2 block text-xs text-[var(--muted)]">{a.message}</span>
                  ) : null}
                </span>
                {!a.is_acked ? (
                  <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full" style={{ background: "var(--accent)" }} />
                ) : null}
              </button>
            ))
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
