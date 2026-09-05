import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { useAppDialog } from "./confirm";
import { Badge } from "./components/ui/badge";
import { IconCheck, IconEye, IconPencil, IconUserCheck } from "./icons";
import { toastError, toastSuccess } from "./swal";
import {
  Button,
  FormDialog,
  IconButton,
  Input,
  Label,
  Section,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Table,
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
  sender_name?: string;
  message: string;
  created_at: string;
};

type CustomerOpt = { id: string; full_name: string; customer_code: string };

type StaffOpt = { user_id: string; full_name: string; is_active: boolean };

const statusLabels: Record<string, string> = {
  open: "Open",
  in_progress: "Proses",
  resolved: "Resolved",
  closed: "Closed",
};

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
  if (s === "resolved" || s === "closed") return "success";
  if (s === "in_progress") return "default";
  return "outline";
}

export function TicketsPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(0);
  const limit = 20;
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [formErr, setFormErr] = useState("");
  const [detail, setDetail] = useState<TicketRow | null>(null);
  const [msg, setMsg] = useState("");
  const [assignOpen, setAssignOpen] = useState<TicketRow | null>(null);
  const [assignUser, setAssignUser] = useState("");

  const q = useQuery({
    queryKey: ["tickets", status, page],
    queryFn: () =>
      api<{ data: TicketRow[]; total: number }>(
        `/api/tickets?limit=${limit}&offset=${page * limit}${status ? `&status=${encodeURIComponent(status)}` : ""}`,
      ),
  });
  const customersQ = useQuery({
    queryKey: ["customers"],
    queryFn: () => api<{ data: CustomerOpt[] }>("/api/customers?limit=100"),
  });
  const usersQ = useQuery({
    queryKey: ["tenant-users"],
    queryFn: () => api<StaffOpt[]>("/api/settings/users"),
  });
  const messagesQ = useQuery({
    queryKey: ["ticket-messages", detail?.id],
    queryFn: () => api<TicketMessage[]>(`/api/tickets/${detail!.id}/messages`),
    enabled: Boolean(detail?.id),
  });

  const customers = customersQ.data?.data ?? [];
  const users = (Array.isArray(usersQ.data) ? usersQ.data : []).filter((u) => u.is_active);
  const list = q.data?.data ?? [];
  const total = q.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / limit));
  const messages = Array.isArray(messagesQ.data) ? messagesQ.data : [];

  const create = useMutation({
    mutationFn: () =>
      api("/api/tickets", {
        method: "POST",
        body: JSON.stringify({
          subject: form.subject.trim(),
          description: form.description.trim() || undefined,
          category: form.category,
          priority: form.priority,
          customer_id: form.customer_id || undefined,
        }),
      }),
    onSuccess: () => {
      setOpen(false);
      setForm(emptyForm);
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
    mutationFn: ({ id, status: st }: { id: string; status: string }) =>
      api(`/api/tickets/${id}/status`, { method: "PATCH", body: JSON.stringify({ status: st }) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tickets"] });
      if (detail) {
        void api<TicketRow>(`/api/tickets/${detail.id}`).then(setDetail).catch(() => undefined);
      }
      void toastSuccess("Status diperbarui");
    },
    onError: (e: Error) => void toastError(e.message),
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
        body: JSON.stringify({ message: msg.trim() }),
      }),
    onSuccess: () => {
      setMsg("");
      qc.invalidateQueries({ queryKey: ["ticket-messages", detail?.id] });
      void toastSuccess("Pesan terkirim");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  return (
    <Section
      title="Tiket"
      actions={
        <Button
          type="button"
          onClick={() => {
            setForm(emptyForm);
            setFormErr("");
            setOpen(true);
          }}
        >
          + Tambah
        </Button>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Tiket support pelanggan: filter status, assign ke tim, ubah status, dan catat balasan.
      </p>

      <div className="mb-4 flex flex-wrap items-end gap-3">
        <div className="min-w-[180px]">
          <Label className="mb-1.5 block">Status</Label>
          <Select
            value={status || "__all__"}
            onValueChange={(v) => {
              setStatus(v === "__all__" ? "" : v);
              setPage(0);
            }}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all__">Semua</SelectItem>
              {Object.entries(statusLabels).map(([k, label]) => (
                <SelectItem key={k} value={k}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <Table
        columns={["Subjek", "Pelanggan", "Kategori", "Prioritas", "Status", "SLA", "Aksi"]}
        rows={list.map((t) => [
          <div key={`${t.id}-sub`} className="max-w-[220px]">
            <p className="font-medium text-[var(--text)]">{t.subject}</p>
            <p className="text-xs text-[var(--muted)]">{formatWhen(t.created_at)}</p>
          </div>,
          t.customer_name || "—",
          categoryLabels[t.category] || t.category,
          <Badge key={`${t.id}-p`} variant={priorityVariant(t.priority)}>
            {priorityLabels[t.priority] || t.priority}
          </Badge>,
          <Badge key={`${t.id}-s`} variant={statusVariant(t.status)}>
            {statusLabels[t.status] || t.status}
          </Badge>,
          formatWhen(t.sla_due_at),
          <span key={`${t.id}-a`} className="flex flex-wrap items-center gap-1.5">
            <IconButton
              label="Detail tiket"
              onClick={() => {
                setDetail(t);
                setMsg("");
              }}
            >
              <IconEye />
            </IconButton>
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
            <IconButton
              label="Assign"
              onClick={() => {
                setAssignOpen(t);
                setAssignUser(t.assigned_to || "");
              }}
            >
              <IconUserCheck />
            </IconButton>
          </span>,
        ])}
      />

      <div className="mt-3 flex items-center justify-between gap-2 text-sm text-[var(--muted)]">
        <span>
          Halaman {page + 1} / {pages} · {total} tiket
        </span>
        <div className="flex gap-2">
          <Button type="button" variant="outline" size="sm" disabled={page <= 0} onClick={() => setPage((p) => p - 1)}>
            Sebelumnya
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={page + 1 >= pages}
            onClick={() => setPage((p) => p + 1)}
          >
            Berikutnya
          </Button>
        </div>
      </div>

      <FormDialog
        open={open}
        wide
        title="Buat tiket"
        onClose={() => {
          setOpen(false);
          setFormErr("");
        }}
      >
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
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
            <Input
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              placeholder="Detail (opsional)"
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
            <Select
              value={form.customer_id || "__none__"}
              onValueChange={(v) => setForm({ ...form, customer_id: v === "__none__" ? "" : v })}
            >
              <SelectTrigger>
                <SelectValue placeholder="Pelanggan" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__none__">— Tidak terkait —</SelectItem>
                {customers.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.full_name} ({c.customer_code})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>
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
              {detail.status === "closed" || detail.status === "resolved" ? (
                <Button type="button" size="sm" variant="secondary" onClick={() => setStatusMut.mutate({ id: detail.id, status: "open" })}>
                  Buka lagi
                </Button>
              ) : null}
            </div>

            <div>
              <h4 className="mb-2 text-sm font-semibold">Balasan</h4>
              <div className="mb-3 max-h-56 space-y-2 overflow-y-auto">
                {messages.length === 0 ? (
                  <p className="text-sm text-[var(--muted)]">Belum ada pesan.</p>
                ) : (
                  messages.map((m) => (
                    <div key={m.id} className="rounded-md border border-[var(--border)] p-2 text-sm">
                      <p className="text-xs text-[var(--muted)]">
                        {m.sender_name || m.sender_type} · {formatWhen(m.created_at)}
                      </p>
                      <p className="mt-1 text-[var(--text)]">{m.message}</p>
                    </div>
                  ))
                )}
              </div>
              <form
                className="flex flex-wrap gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (!msg.trim()) return;
                  sendMsg.mutate();
                }}
              >
                <Input
                  className="min-w-[200px] flex-1"
                  placeholder="Tulis balasan…"
                  value={msg}
                  onChange={(e) => setMsg(e.target.value)}
                />
                <Button type="submit" disabled={sendMsg.isPending || !msg.trim()}>
                  Kirim
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
            <Select value={assignUser || "__me__"} onValueChange={(v) => setAssignUser(v === "__me__" ? "" : v)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__me__">Saya (user login)</SelectItem>
                {users.map((u) => (
                  <SelectItem key={u.user_id} value={u.user_id}>
                    {u.full_name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="mt-1 text-xs text-[var(--muted)]">Kosongkan pilihan khusus = assign ke akun yang sedang login.</p>
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
