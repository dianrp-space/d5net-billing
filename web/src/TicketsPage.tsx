import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LayoutGrid, List } from "lucide-react";
import { api, apiUpload } from "./api";
import { ProgressFileUpload } from "./ProgressFileUpload";
import { useAppDialog } from "./confirm";
import { nameWithSaya } from "./me";
import { UserAvatar } from "./UserMenu";
import { Badge } from "./components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "./components/ui/card";
import {
  Kanban,
  KanbanBoard,
  KanbanColumn,
  KanbanColumnContent,
  KanbanColumnHeader,
  KanbanItem,
  KanbanOverlay,
  type KanbanCommitMeta,
} from "./components/ui/kanban";
import { IconBan, IconCheck, IconPencil, IconTrash, IconUserCheck } from "./icons";
import { canDispatchOps, type MePermissions } from "./permissions";
import { toastError, toastSuccess } from "./swal";
import { ListToolbar, matchesQuery, useDebouncedValue } from "./ListToolbar";
import {
  Button,
  FormDialog,
  IconButton,
  Input,
  Label,
  SearchableSelect,
  Section,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Table,
  Textarea,
} from "./ui";

type TicketRow = {
  id: string;
  subject: string;
  description?: string | null;
  category: string;
  priority: string;
  status: string;
  customer_id?: string | null;
  customer_name?: string;
  assigned_to?: string | null;
  assignee_name?: string;
  sla_due_at?: string | null;
  created_at?: string;
};

type TicketMessage = {
  id: string;
  sender_type: string;
  sender_id?: string | null;
  sender_name?: string;
  avatar_url?: string | null;
  message: string;
  image_urls?: string[];
  created_at: string;
};

type CustomerOpt = { id: string; full_name: string; customer_code: string };

type StaffOpt = { user_id: string; full_name: string; is_active: boolean };

const COLUMNS: { id: string; label: string }[] = [
  { id: "open", label: "Open" },
  { id: "in_progress", label: "Proses" },
  { id: "resolved", label: "Resolved" },
  { id: "closed", label: "Closed" },
  { id: "cancelled", label: "Batal" },
];

const COLUMN_IDS = COLUMNS.map((c) => c.id);

const statusLabels: Record<string, string> = Object.fromEntries(COLUMNS.map((c) => [c.id, c.label]));

const priorityLabels: Record<string, string> = {
  low: "Rendah",
  normal: "Normal",
  high: "Tinggi",
  urgent: "Urgent",
};

const categoryLabels: Record<string, string> = {
  general: "Umum",
  billing: "Billing",
  technical: "Teknis",
  outage: "Gangguan",
  installation: "Instalasi",
};

const emptyForm = {
  subject: "",
  description: "",
  category: "general",
  priority: "normal",
  customer_id: "",
};

const TICKETS_VIEW_KEY = "drp_tickets_view";

function emptyBoard(): Record<string, TicketRow[]> {
  return Object.fromEntries(COLUMN_IDS.map((id) => [id, []]));
}

function boardFromList(list: TicketRow[]): Record<string, TicketRow[]> {
  const board = emptyBoard();
  for (const t of list) {
    const st = COLUMN_IDS.includes(t.status) ? t.status : "open";
    board[st].push(t);
  }
  return board;
}

function formatWhen(iso?: string | null) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("id-ID");
  } catch {
    return iso;
  }
}

function priorityVariant(p: string): "default" | "danger" | "outline" | "success" {
  if (p === "urgent" || p === "high") return "danger";
  if (p === "low") return "outline";
  return "default";
}

function statusVariant(s: string): "default" | "danger" | "outline" | "success" {
  if (s === "cancelled") return "danger";
  if (s === "resolved" || s === "closed") return "success";
  if (s === "in_progress") return "default";
  return "outline";
}

function TicketCardBody({
  ticket,
  overlay,
  actions,
  onOpen,
}: {
  ticket: TicketRow;
  overlay?: boolean;
  actions?: ReactNode;
  onOpen?: () => void;
}) {
  return (
    <Card
      role={onOpen ? "button" : undefined}
      tabIndex={onOpen ? 0 : undefined}
      onClick={onOpen}
      onKeyDown={
        onOpen
          ? (e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onOpen();
              }
            }
          : undefined
      }
      className={`shadow-none ${onOpen ? "cursor-pointer" : ""} ${
        overlay
          ? "rotate-1 scale-[1.02] shadow-[var(--shadow-md)] ring-1 ring-[var(--accent)]"
          : "hover:border-[var(--accent)]/40"
      }`}
    >
      <CardHeader className="space-y-1 p-3 pb-1">
        <CardTitle className="text-sm leading-snug">{ticket.subject}</CardTitle>
        <p className="text-xs text-[var(--muted)]">{ticket.customer_name || "Tanpa pelanggan"}</p>
      </CardHeader>
      <CardContent className="space-y-2 p-3 pt-1">
        <div className="flex flex-wrap gap-1">
          <Badge variant={priorityVariant(ticket.priority)}>
            {priorityLabels[ticket.priority] || ticket.priority}
          </Badge>
          <Badge variant="outline">{categoryLabels[ticket.category] || ticket.category}</Badge>
        </div>
        {ticket.assignee_name ? (
          <p className="text-[10px] text-[var(--muted)]">Teknisi: {ticket.assignee_name}</p>
        ) : null}
        {actions ? (
          <div
            className="flex flex-wrap items-center gap-1"
            onPointerDown={(e) => e.stopPropagation()}
            onClick={(e) => e.stopPropagation()}
          >
            {actions}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

export function TicketsPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [status, setStatus] = useState("");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, 300);
  const [page, setPage] = useState(0);
  const limit = 20;
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [formErr, setFormErr] = useState("");
  const [detail, setDetail] = useState<TicketRow | null>(null);
  const [msg, setMsg] = useState("");
  const [assignOpen, setAssignOpen] = useState<TicketRow | null>(null);
  const [assignUser, setAssignUser] = useState("");
  const [columns, setColumns] = useState<Record<string, TicketRow[]>>(emptyBoard);
  const [pendingImages, setPendingImages] = useState<string[]>([]);
  const [createImages, setCreateImages] = useState<string[]>([]);
  const [view, setView] = useState<"kanban" | "list">(() => {
    try {
      const v = localStorage.getItem(TICKETS_VIEW_KEY);
      return v === "list" ? "list" : "kanban";
    } catch {
      return "kanban";
    }
  });

  function setViewMode(next: "kanban" | "list") {
    setView(next);
    try {
      localStorage.setItem(TICKETS_VIEW_KEY, next);
    } catch {
      /* ignore */
    }
  }

  const meQ = useQuery({
    queryKey: ["me"],
    queryFn: () => api<MePermissions & { user_id: string }>("/api/me"),
  });
  const canDispatch = canDispatchOps(meQ.data?.permissions);

  const listQ = useQuery({
    queryKey: ["tickets", "list", status, debouncedSearch, page],
    queryFn: () => {
      const params = new URLSearchParams({
        limit: String(limit),
        offset: String(page * limit),
      });
      if (status) params.set("status", status);
      if (debouncedSearch.trim()) params.set("search", debouncedSearch.trim());
      return api<{ data: TicketRow[]; total: number }>(`/api/tickets?${params}`);
    },
    enabled: view === "list",
  });
  const boardQ = useQuery({
    queryKey: ["tickets", "kanban"],
    queryFn: () => api<{ data: TicketRow[]; total: number }>("/api/tickets?limit=200&offset=0"),
    enabled: view === "kanban",
  });
  const customersQ = useQuery({
    queryKey: ["customers"],
    queryFn: () => api<{ data: CustomerOpt[] }>("/api/customers?limit=500"),
    enabled: canDispatch,
  });
  const usersQ = useQuery({
    queryKey: ["user-options"],
    queryFn: () => api<StaffOpt[]>("/api/users/options"),
    enabled: canDispatch,
  });
  const messagesQ = useQuery({
    queryKey: ["ticket-messages", detail?.id],
    queryFn: () => api<TicketMessage[]>(`/api/tickets/${detail!.id}/messages`),
    enabled: Boolean(detail?.id),
  });

  const customers = customersQ.data?.data ?? [];
  const users = (Array.isArray(usersQ.data) ? usersQ.data : []).filter((u) => u.is_active);
  const list = listQ.data?.data ?? [];
  const total = listQ.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / limit));
  const boardList = boardQ.data?.data;
  const messages = Array.isArray(messagesQ.data) ? messagesQ.data : [];

  useEffect(() => {
    if (view === "kanban") {
      const all = boardList ?? [];
      const filtered = all.filter((t) =>
        matchesQuery(search, t.subject, t.customer_name, t.assignee_name, t.category),
      );
      setColumns(boardFromList(filtered));
    }
  }, [view, boardList, search]);

  useEffect(() => {
    if (!detail) {
      setMsg("");
      setPendingImages([]);
    }
  }, [detail]);

  const ticketById = useMemo(() => {
    const map = new Map<string, TicketRow>();
    for (const col of Object.values(columns)) {
      for (const t of col) map.set(t.id, t);
    }
    return map;
  }, [columns]);

  const create = useMutation({
    mutationFn: () =>
      api("/api/tickets", {
        method: "POST",
        body: JSON.stringify({
          subject: form.subject.trim(),
          description: form.description.trim(),
          category: form.category,
          priority: form.priority,
          customer_id: form.customer_id || undefined,
          image_urls: createImages.length ? createImages : undefined,
        }),
      }),
    onSuccess: () => {
      setOpen(false);
      setForm(emptyForm);
      setCreateImages([]);
      setFormErr("");
      qc.invalidateQueries({ queryKey: ["tickets"] });
      void toastSuccess("Tiket dibuat");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const setStatusMut = useMutation({
    mutationFn: ({ id, status: st }: { id: string; status: string; quiet?: boolean }) =>
      api(`/api/tickets/${id}/status`, { method: "PATCH", body: JSON.stringify({ status: st }) }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ["tickets"] });
      qc.invalidateQueries({ queryKey: ["ticket-messages"] });
      if (detail) {
        void api<TicketRow>(`/api/tickets/${detail.id}`).then(setDetail).catch(() => undefined);
      }
      if (!vars.quiet) void toastSuccess("Status diperbarui");
    },
    onError: (e: Error) => {
      qc.invalidateQueries({ queryKey: ["tickets"] });
      void toastError(e.message);
    },
  });

  const assignMut = useMutation({
    mutationFn: ({ id, assigned_to }: { id: string; assigned_to?: string }) =>
      api(`/api/tickets/${id}/assign`, {
        method: "PATCH",
        body: JSON.stringify({ assigned_to: assigned_to || undefined }),
      }),
    onSuccess: () => {
      setAssignOpen(null);
      setAssignUser("");
      qc.invalidateQueries({ queryKey: ["tickets"] });
      void toastSuccess("Tiket di-assign");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const sendMsg = useMutation({
    mutationFn: () =>
      api(`/api/tickets/${detail!.id}/messages`, {
        method: "POST",
        body: JSON.stringify({
          message: msg.trim(),
          image_urls: pendingImages.length ? pendingImages : undefined,
        }),
      }),
    onSuccess: () => {
      setMsg("");
      setPendingImages([]);
      qc.invalidateQueries({ queryKey: ["ticket-messages", detail?.id] });
      qc.invalidateQueries({ queryKey: ["tickets"] });
      if (detail?.status === "open") {
        setDetail({ ...detail, status: "in_progress" });
      }
      void toastSuccess("Balasan terkirim");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => api(`/api/tickets/${id}`, { method: "DELETE" }),
    onSuccess: (_data, id) => {
      if (detail?.id === id) setDetail(null);
      qc.invalidateQueries({ queryKey: ["tickets"] });
      void toastSuccess("Tiket dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  function openDetail(t: TicketRow) {
    setDetail(t);
    setMsg("");
    setPendingImages([]);
  }

  function ticketActions(t: TicketRow) {
    return (
      <>
        {t.status === "open" ? (
          <IconButton label="Mulai proses" onClick={() => setStatusMut.mutate({ id: t.id, status: "in_progress" })}>
            <IconPencil />
          </IconButton>
        ) : null}
        {t.status === "open" || t.status === "in_progress" ? (
          <IconButton
            label="Resolve"
            onClick={async () => {
              const ok = await confirm({
                title: "Resolve tiket",
                description: `Tandai "${t.subject}" sebagai resolved?`,
                confirmLabel: "Resolve",
              });
              if (!ok) return;
              setStatusMut.mutate({ id: t.id, status: "resolved" });
            }}
          >
            <IconCheck />
          </IconButton>
        ) : null}
        {canDispatch ? (
          <IconButton
            label="Assign"
            onClick={() => {
              setAssignOpen(t);
              setAssignUser(t.assigned_to || "");
            }}
          >
            <IconUserCheck />
          </IconButton>
        ) : null}
        {t.status === "open" || t.status === "in_progress" ? (
          <IconButton
            label="Batalkan"
            onClick={async () => {
              const ok = await confirm({
                title: "Batalkan tiket",
                description: `Batalkan "${t.subject}"? Tiket tetap tersimpan dengan status batal.`,
                confirmLabel: "Batalkan",
              });
              if (!ok) return;
              setStatusMut.mutate({ id: t.id, status: "cancelled" });
            }}
          >
            <IconBan />
          </IconButton>
        ) : null}
        {canDispatch ? (
          <IconButton
            label="Hapus"
            onClick={async () => {
              const ok = await confirm({
                title: "Hapus tiket",
                description: `Hapus "${t.subject}" beserta percakapannya? Tindakan ini tidak bisa dibatalkan.`,
                confirmLabel: "Hapus",
              });
              if (!ok) return;
              deleteMut.mutate(t.id);
            }}
          >
            <IconTrash />
          </IconButton>
        ) : null}
      </>
    );
  }

  function onValueCommit(next: Record<string, TicketRow[]>, meta: KanbanCommitMeta<TicketRow>) {
    const ticket = next[meta.overContainer]?.[meta.overIndex];
    if (!ticket) {
      setColumns(meta.previousValue);
      return;
    }
    if (meta.activeContainer === meta.overContainer) {
      return;
    }
    setColumns(
      Object.fromEntries(
        Object.entries(next).map(([k, rows]) => [
          k,
          rows.map((r) => (r.id === ticket.id ? { ...r, status: k } : r)),
        ]),
      ),
    );
    setStatusMut.mutate({ id: ticket.id, status: meta.overContainer, quiet: true });
  }

  return (
    <Section
      title="Tiket"
      actions={
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center rounded-md border border-[var(--border)] p-0.5">
            <IconButton
              label="Tampilan kanban"
              className={view === "kanban" ? "bg-[var(--panel-muted)]" : undefined}
              onClick={() => setViewMode("kanban")}
            >
              <LayoutGrid className="size-4" />
            </IconButton>
            <IconButton
              label="Tampilan daftar"
              className={view === "list" ? "bg-[var(--panel-muted)]" : undefined}
              onClick={() => setViewMode("list")}
            >
              <List className="size-4" />
            </IconButton>
          </div>
          {canDispatch ? (
            <Button
              type="button"
              onClick={() => {
                setForm(emptyForm);
                setCreateImages([]);
                setFormErr("");
                setOpen(true);
              }}
            >
              + Tambah
            </Button>
          ) : null}
        </div>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        {canDispatch
          ? view === "kanban"
            ? "Kanban: seret kartu antar kolom status. Assign ke teknisi, catat balasan di detail."
            : "Daftar tiket support & instalasi. Filter status, assign teknisi, ubah status, catat balasan."
          : view === "kanban"
            ? "Kanban tiket yang di-assign ke Anda. Seret untuk ubah status."
            : "Tiket yang di-assign ke Anda. Ubah status dan balas pesan."}
      </p>

      {view === "list" ? (
        <>
          <ListToolbar
            search={search}
            onSearchChange={(v) => {
              setSearch(v);
              setPage(0);
            }}
            searchPlaceholder="Subjek, pelanggan, atau teknisi…"
            filters={[
              {
                key: "status",
                label: "Status",
                value: status,
                onChange: (v) => {
                  setStatus(v);
                  setPage(0);
                },
                options: COLUMNS.map((c) => ({ value: c.id, label: c.label })),
              },
            ]}
            page={page}
            pageCount={pages}
            onPageChange={setPage}
            total={total}
          />

          <Table
            rowNumberStart={page * limit + 1}
            columns={["Subjek", "Pelanggan", "Teknisi", "Kategori", "Prioritas", "Status", "SLA", "Aksi"]}
            onRowClick={(i) => openDetail(list[i])}
            rows={list.map((t) => [
              <div key={`${t.id}-sub`} className="max-w-[220px]">
                <p className="font-medium text-[var(--text)]">{t.subject}</p>
                <p className="text-xs text-[var(--muted)]">{formatWhen(t.created_at)}</p>
              </div>,
              t.customer_name || "—",
              t.assignee_name || "—",
              categoryLabels[t.category] || t.category,
              <Badge key={`${t.id}-p`} variant={priorityVariant(t.priority)}>
                {priorityLabels[t.priority] || t.priority}
              </Badge>,
              <Badge key={`${t.id}-s`} variant={statusVariant(t.status)}>
                {statusLabels[t.status] || t.status}
              </Badge>,
              formatWhen(t.sla_due_at),
              <span key={`${t.id}-a`} className="flex flex-wrap items-center gap-1.5">
                {ticketActions(t)}
              </span>,
            ])}
          />
        </>
      ) : (
        <>
          <ListToolbar
            search={search}
            onSearchChange={setSearch}
            searchPlaceholder="Filter kanban: subjek, pelanggan…"
          />
          <Kanban
          value={columns}
          onValueChange={setColumns}
          getItemValue={(item) => item.id}
          onValueCommit={onValueCommit}
          restoreOnCancel
          className="min-w-0"
        >
          <KanbanBoard>
            {COLUMNS.map((col) => (
              <KanbanColumn key={col.id} value={col.id}>
                <KanbanColumnHeader>
                  <div>
                    <p className="text-sm font-semibold text-[var(--text)]">{col.label}</p>
                  </div>
                  <Badge variant="outline">{(columns[col.id] ?? []).length}</Badge>
                </KanbanColumnHeader>
                <KanbanColumnContent>
                  {(columns[col.id] ?? []).map((t) => (
                    <KanbanItem key={t.id} value={t.id}>
                      <TicketCardBody ticket={t} onOpen={() => openDetail(t)} actions={ticketActions(t)} />
                    </KanbanItem>
                  ))}
                  {(columns[col.id] ?? []).length === 0 ? (
                    <p className="px-2 py-6 text-center text-xs text-[var(--muted)]">Kosong</p>
                  ) : null}
                </KanbanColumnContent>
              </KanbanColumn>
            ))}
          </KanbanBoard>

          <KanbanOverlay>
            {({ value, variant }) => {
              if (variant !== "item") return null;
              const ticket = ticketById.get(String(value));
              if (!ticket) return null;
              return (
                <div className="w-[244px]">
                  <TicketCardBody ticket={ticket} overlay />
                </div>
              );
            }}
          </KanbanOverlay>
        </Kanban>
        </>
      )}

      <FormDialog
        open={open}
        wide
        title="Buat tiket"
        onClose={() => {
          setOpen(false);
          setFormErr("");
          setCreateImages([]);
        }}
      >
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (!form.subject.trim() || !form.description.trim()) {
              setFormErr("Subjek dan deskripsi wajib diisi");
              return;
            }
            create.mutate();
          }}
        >
          <div className="sm:col-span-2">
            <Label className="mb-1.5 block">Subjek</Label>
            <Input
              value={form.subject}
              onChange={(e) => setForm({ ...form, subject: e.target.value })}
              required
              placeholder="Ringkas masalah"
            />
          </div>
          <div className="sm:col-span-2">
            <Label className="mb-1.5 block">Deskripsi</Label>
            <Textarea
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              required
              rows={4}
              placeholder="Jelaskan masalah atau permintaan"
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Kategori</Label>
            <Select value={form.category} onValueChange={(v) => setForm({ ...form, category: v })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(categoryLabels).map(([k, label]) => (
                  <SelectItem key={k} value={k}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <Label className="mb-1.5 block">Prioritas</Label>
            <Select value={form.priority} onValueChange={(v) => setForm({ ...form, priority: v })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(priorityLabels).map(([k, label]) => (
                  <SelectItem key={k} value={k}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="sm:col-span-2">
            <Label className="mb-1.5 block">Pelanggan (opsional)</Label>
            <SearchableSelect
              allowClear
              clearLabel="— Tidak terkait —"
              placeholder="Pelanggan"
              searchPlaceholder="Cari nama / kode pelanggan…"
              value={form.customer_id}
              onValueChange={(v) => setForm({ ...form, customer_id: v })}
              options={customers.map((c) => ({
                value: c.id,
                label: `${c.full_name} (${c.customer_code})`,
                keywords: `${c.full_name} ${c.customer_code}`,
              }))}
            />
          </div>
          <div className="sm:col-span-2">
            <Label className="mb-1.5 block">Lampiran gambar (opsional)</Label>
            {createImages.length > 0 ? (
              <div className="mb-2 flex flex-wrap gap-2">
                {createImages.map((url) => (
                  <div key={url} className="relative">
                    <img
                      src={url}
                      alt=""
                      className="h-16 w-16 rounded-md border border-[var(--border)] object-cover"
                    />
                    <button
                      type="button"
                      className="absolute -right-1 -top-1 rounded-full bg-[var(--danger)] px-1 text-[10px] text-white"
                      title="Hapus lampiran"
                      aria-label="Hapus lampiran"
                      onClick={() => setCreateImages((prev) => prev.filter((u) => u !== url))}
                    >
                      ×
                    </button>
                  </div>
                ))}
              </div>
            ) : null}
            <ProgressFileUpload
              label="Lampirkan gambar"
              hint="Bisa beberapa file; progress per file"
              disabled={create.isPending}
              uploadFile={async (file, onProgress) => {
                const res = await apiUpload<{ url: string }>("/api/tickets/photos", file, { onProgress });
                return res.url;
              }}
              onBatchComplete={(urls) => {
                setCreateImages((prev) => [...prev, ...urls]);
                void toastSuccess(
                  urls.length > 1 ? `${urls.length} gambar siap dilampirkan` : "Gambar siap dilampirkan",
                );
              }}
            />
          </div>
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setOpen(false);
                setCreateImages([]);
              }}
            >
              Batal
            </Button>
          </div>
          {formErr ? <p className="text-sm text-[var(--danger)] sm:col-span-2">{formErr}</p> : null}
        </form>
      </FormDialog>

      <FormDialog
        open={Boolean(detail)}
        wide
        title={detail ? `Tiket: ${detail.subject}` : "Detail tiket"}
        onClose={() => setDetail(null)}
      >
        {detail ? (
          <div className="grid gap-4">
            <div className="grid gap-2 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/40 p-3 text-sm sm:grid-cols-2">
              <p>
                <span className="text-[var(--muted)]">Status:</span>{" "}
                {statusLabels[detail.status] || detail.status}
              </p>
              <p>
                <span className="text-[var(--muted)]">Prioritas:</span>{" "}
                {priorityLabels[detail.priority] || detail.priority}
              </p>
              <p>
                <span className="text-[var(--muted)]">Kategori:</span>{" "}
                {categoryLabels[detail.category] || detail.category}
              </p>
              <p>
                <span className="text-[var(--muted)]">Pelanggan:</span> {detail.customer_name || "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">Assignee:</span> {detail.assignee_name || "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">SLA:</span> {formatWhen(detail.sla_due_at)}
              </p>
              {detail.description ? (
                <p className="sm:col-span-2">
                  <span className="text-[var(--muted)]">Deskripsi:</span> {detail.description}
                </p>
              ) : null}
            </div>

            <div className="flex flex-wrap gap-2">
              {detail.status === "open" ? (
                <Button type="button" size="sm" onClick={() => setStatusMut.mutate({ id: detail.id, status: "in_progress" })}>
                  Mulai proses
                </Button>
              ) : null}
              {detail.status === "open" || detail.status === "in_progress" ? (
                <Button type="button" size="sm" onClick={() => setStatusMut.mutate({ id: detail.id, status: "resolved" })}>
                  Resolve
                </Button>
              ) : null}
              {detail.status === "resolved" ? (
                <Button type="button" size="sm" variant="outline" onClick={() => setStatusMut.mutate({ id: detail.id, status: "closed" })}>
                  Tutup
                </Button>
              ) : null}
              {detail.status === "closed" || detail.status === "resolved" || detail.status === "cancelled" ? (
                <Button type="button" size="sm" variant="secondary" onClick={() => setStatusMut.mutate({ id: detail.id, status: "open" })}>
                  Buka lagi
                </Button>
              ) : null}
              {detail.status === "open" || detail.status === "in_progress" ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={async () => {
                    const ok = await confirm({
                      title: "Batalkan tiket",
                      description: `Batalkan "${detail.subject}"?`,
                      confirmLabel: "Batalkan",
                    });
                    if (!ok) return;
                    setStatusMut.mutate({ id: detail.id, status: "cancelled" });
                  }}
                >
                  Batalkan
                </Button>
              ) : null}
              {canDispatch ? (
                <Button
                  type="button"
                  size="sm"
                  variant="danger"
                  onClick={async () => {
                    const ok = await confirm({
                      title: "Hapus tiket",
                      description: `Hapus "${detail.subject}" beserta percakapannya? Tindakan ini tidak bisa dibatalkan.`,
                      confirmLabel: "Hapus",
                    });
                    if (!ok) return;
                    deleteMut.mutate(detail.id);
                  }}
                >
                  Hapus
                </Button>
              ) : null}
            </div>

            <div>
              <h4 className="mb-2 text-sm font-semibold">Percakapan</h4>
              <div className="mb-3 max-h-72 space-y-2 overflow-y-auto">
                {detail.description && !messages.some((m) => m.sender_type === "customer") ? (
                  <div className="rounded-md border border-[var(--border)] bg-[color-mix(in_srgb,var(--secondary)_10%,transparent)] p-2 text-sm">
                    <p className="text-xs font-semibold text-[var(--secondary)]">Pelanggan · keluhan awal</p>
                    <p className="mt-1 whitespace-pre-wrap text-[var(--text)]">{detail.description}</p>
                  </div>
                ) : null}
                {messages.length === 0 && !detail.description ? (
                  <p className="text-sm text-[var(--muted)]">Belum ada percakapan. Tulis balasan di bawah.</p>
                ) : (
                  messages.map((m) => {
                    const isStatus = m.sender_type === "system";
                    const isCustomer = m.sender_type === "customer";
                    const meId = meQ.data?.user_id;
                    const displayName = nameWithSaya(
                      isStatus
                        ? `${m.sender_name || "Sistem"} · pindah status`
                        : isCustomer
                          ? m.sender_name || "Pelanggan"
                          : m.sender_name || "Staf",
                      m.sender_id,
                      meId,
                    );
                    return (
                      <div
                        key={m.id}
                        className={
                          isStatus
                            ? "rounded-md border border-dashed border-[var(--border)] bg-[var(--panel-muted)]/50 px-2 py-1.5 text-sm"
                            : isCustomer
                              ? "rounded-md border border-[var(--border)] bg-[color-mix(in_srgb,var(--secondary)_10%,transparent)] p-2 text-sm"
                              : "rounded-md border border-[var(--accent)]/25 bg-[color-mix(in_srgb,var(--accent)_8%,transparent)] p-2 text-sm"
                        }
                      >
                        <p className="flex flex-wrap items-center gap-1.5 text-xs text-[var(--muted)]">
                          {!isStatus ? <UserAvatar name={m.sender_name} avatarUrl={m.avatar_url} /> : null}
                          <span className="min-w-0 flex-1 truncate">
                            {isCustomer ? "Pelanggan · " : isStatus ? "" : "Staf · "}
                            {displayName} · {formatWhen(m.created_at)}
                          </span>
                        </p>
                        {m.message ? (
                          <p className={isStatus ? "mt-1 text-[var(--muted)]" : "mt-1 whitespace-pre-wrap text-[var(--text)]"}>
                            {m.message}
                          </p>
                        ) : null}
                        {!isStatus && (m.image_urls?.length ?? 0) > 0 ? (
                          <div className="mt-2 flex flex-wrap gap-2">
                            {m.image_urls!.map((url) => (
                              <a
                                key={url}
                                href={url}
                                target="_blank"
                                rel="noreferrer"
                                className="block overflow-hidden rounded border border-[var(--border)]"
                              >
                                <img src={url} alt="" className="h-20 w-20 object-cover" />
                              </a>
                            ))}
                          </div>
                        ) : null}
                      </div>
                    );
                  })
                )}
              </div>
              {pendingImages.length > 0 ? (
                <div className="mb-2 flex flex-wrap gap-2">
                  {pendingImages.map((url) => (
                    <div key={url} className="relative">
                      <img src={url} alt="" className="h-16 w-16 rounded border border-[var(--border)] object-cover" />
                      <button
                        type="button"
                        className="absolute -right-1 -top-1 rounded-full bg-[var(--danger)] px-1 text-[10px] text-white"
                        title="Hapus lampiran"
                        aria-label="Hapus lampiran"
                        onClick={() => setPendingImages((prev) => prev.filter((u) => u !== url))}
                      >
                        ×
                      </button>
                    </div>
                  ))}
                </div>
              ) : null}
              <form
                className="grid gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (!msg.trim() && pendingImages.length === 0) return;
                  sendMsg.mutate();
                }}
              >
                <Label htmlFor="ticket-reply">Balas ke pelanggan</Label>
                <Textarea
                  id="ticket-reply"
                  className="min-h-24"
                  placeholder="Tulis balasan untuk pelanggan…"
                  value={msg}
                  onChange={(e) => setMsg(e.target.value)}
                />
                <ProgressFileUpload
                  label="Lampirkan gambar"
                  hint="Progress upload per file"
                  uploadFile={async (file, onProgress) => {
                    if (!detail) throw new Error("Tiket tidak dipilih");
                    const res = await apiUpload<{ url: string }>(
                      `/api/tickets/${detail.id}/messages/photos`,
                      file,
                      { onProgress },
                    );
                    return res.url;
                  }}
                  onBatchComplete={(urls) => {
                    setPendingImages((prev) => [...prev, ...urls]);
                    void toastSuccess(
                      urls.length > 1 ? `${urls.length} gambar siap dilampirkan` : "Gambar siap dilampirkan",
                    );
                  }}
                />
                <Button type="submit" disabled={sendMsg.isPending || (!msg.trim() && pendingImages.length === 0)}>
                  {sendMsg.isPending ? "Mengirim…" : "Kirim balasan"}
                </Button>
              </form>
            </div>
          </div>
        ) : null}
      </FormDialog>

      <FormDialog
        open={Boolean(assignOpen)}
        title="Assign tiket"
        onClose={() => {
          setAssignOpen(null);
          setAssignUser("");
        }}
      >
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!assignOpen) return;
            assignMut.mutate({ id: assignOpen.id, assigned_to: assignUser || undefined });
          }}
        >
          <p className="text-sm text-[var(--muted)]">{assignOpen?.subject}</p>
          <div>
            <Label className="mb-1.5 block">User / tim</Label>
            <SearchableSelect
              allowClear
              clearLabel="Saya (user login)"
              placeholder="Saya (user login)"
              searchPlaceholder="Cari nama user…"
              value={assignUser}
              onValueChange={setAssignUser}
              options={users.map((u) => ({
                value: u.user_id,
                label: u.full_name,
                keywords: u.full_name,
              }))}
            />
            <p className="mt-1 text-xs text-[var(--muted)]">Kosongkan = assign ke akun yang sedang login.</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={assignMut.isPending}>
              {assignMut.isPending ? "Menyimpan…" : "Assign"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setAssignOpen(null)}>
              Batal
            </Button>
          </div>
        </form>
      </FormDialog>
    </Section>
  );
}
