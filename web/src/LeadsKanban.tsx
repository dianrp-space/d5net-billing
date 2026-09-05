import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import {
  AttributionSelects,
  CommissionBasisSelect,
  type ResellerOpt,
  type StaffOpt,
} from "./AdminExtra";
import { useAppDialog } from "./confirm";
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
import { IconPencil, IconTrash, IconUserCheck } from "./icons";
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
} from "./ui";

type LeadRow = {
  id: string;
  full_name: string;
  phone: string;
  email?: string | null;
  address?: string | null;
  status: string;
  notes?: string | null;
  odp_id?: string | null;
  odp_code?: string;
  odp_name?: string;
  reseller_id?: string | null;
  reseller_name?: string;
  sales_user_id?: string | null;
  sales_user_name?: string;
  customer_id?: string | null;
  created_at?: string;
};

type ClusterOpt = { id: string; name: string; code: string };

type LeadForm = {
  full_name: string;
  phone: string;
  email: string;
  address: string;
  status: string;
  notes: string;
  reseller_id: string;
  sales_user_id: string;
};

const emptyForm: LeadForm = {
  full_name: "",
  phone: "",
  email: "",
  address: "",
  status: "new",
  notes: "",
  reseller_id: "",
  sales_user_id: "",
};

const COLUMNS: { id: string; label: string; hint?: string }[] = [
  { id: "new", label: "Baru" },
  { id: "contacted", label: "Dihubungi" },
  { id: "survey", label: "Survey" },
  { id: "qualified", label: "Siap pasang" },
  { id: "converted", label: "Converted", hint: "Drop untuk convert" },
  { id: "lost", label: "Lost" },
];

const COLUMN_IDS = COLUMNS.map((c) => c.id);

function emptyBoard(): Record<string, LeadRow[]> {
  return Object.fromEntries(COLUMN_IDS.map((id) => [id, []]));
}

function boardFromList(list: LeadRow[]): Record<string, LeadRow[]> {
  const board = emptyBoard();
  for (const l of list) {
    const st = COLUMN_IDS.includes(l.status) ? l.status : "new";
    board[st].push(l);
  }
  return board;
}

function attributionLabel(l: LeadRow) {
  if (l.reseller_name) return `Reseller: ${l.reseller_name}`;
  if (l.sales_user_name) return `Sales: ${l.sales_user_name}`;
  return "";
}

function LeadCardBody({
  lead,
  overlay,
  actions,
}: {
  lead: LeadRow;
  overlay?: boolean;
  actions?: ReactNode;
}) {
  const attr = attributionLabel(lead);
  return (
    <Card
      className={`shadow-none ${
        overlay
          ? "rotate-1 scale-[1.02] shadow-[var(--shadow-md)] ring-1 ring-[var(--accent)]"
          : lead.status !== "converted"
            ? "hover:border-[var(--accent)]/40"
            : ""
      }`}
    >
      <CardHeader className="space-y-1 p-3 pb-1">
        <CardTitle className="text-sm leading-snug">{lead.full_name}</CardTitle>
        <p className="text-xs text-[var(--muted)]">{lead.phone}</p>
      </CardHeader>
      <CardContent className="space-y-2 p-3 pt-1">
        {lead.address ? <p className="line-clamp-2 text-xs text-[var(--muted)]">{lead.address}</p> : null}
        {attr ? <Badge variant="default">{attr}</Badge> : null}
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

export function LeadsPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();

  const q = useQuery({
    queryKey: ["leads", "kanban"],
    queryFn: () => api<{ data: LeadRow[]; total: number }>("/api/leads?limit=200&offset=0"),
  });
  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterOpt[]>("/api/clusters"),
  });
  const resellersQ = useQuery({
    queryKey: ["resellers"],
    queryFn: () => api<ResellerOpt[]>("/api/resellers"),
  });
  const usersQ = useQuery({
    queryKey: ["tenant-users"],
    queryFn: () => api<StaffOpt[]>("/api/settings/users"),
  });

  const [columns, setColumns] = useState<Record<string, LeadRow[]>>(emptyBoard);
  const [form, setForm] = useState<LeadForm>(emptyForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [formErr, setFormErr] = useState("");
  const [convertLead, setConvertLead] = useState<LeadRow | null>(null);
  const [convertCluster, setConvertCluster] = useState("");
  const [convertReseller, setConvertReseller] = useState("");
  const [convertStaff, setConvertStaff] = useState("");
  const [convertBasis, setConvertBasis] = useState("new_customer_flat");

  const clusters = Array.isArray(clustersQ.data) ? clustersQ.data : [];
  const resellers = Array.isArray(resellersQ.data) ? resellersQ.data : [];
  const users = Array.isArray(usersQ.data) ? usersQ.data : [];
  const list = q.data?.data ?? [];

  useEffect(() => {
    setColumns(boardFromList(list));
  }, [list]);

  const leadById = useMemo(() => {
    const map = new Map<string, LeadRow>();
    for (const col of Object.values(columns)) {
      for (const l of col) map.set(l.id, l);
    }
    return map;
  }, [columns]);

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setFormErr("");
    setOpen(true);
  }
  function openEdit(l: LeadRow) {
    if (l.status === "converted") return;
    setEditId(l.id);
    setForm({
      full_name: l.full_name,
      phone: l.phone,
      email: l.email || "",
      address: l.address || "",
      status: l.status || "new",
      notes: l.notes || "",
      reseller_id: l.reseller_id || "",
      sales_user_id: l.sales_user_id || "",
    });
    setFormErr("");
    setOpen(true);
  }
  function closeForm() {
    setOpen(false);
    setEditId(null);
    setFormErr("");
    setForm(emptyForm);
  }
  function openConvert(l: LeadRow) {
    setConvertLead(l);
    setConvertCluster("");
    setConvertReseller(l.reseller_id || "");
    setConvertStaff(l.sales_user_id || "");
    setConvertBasis("new_customer_flat");
  }

  const save = useMutation({
    mutationFn: () => {
      const body = {
        full_name: form.full_name.trim(),
        phone: form.phone.trim(),
        email: form.email.trim() || undefined,
        address: form.address.trim() || undefined,
        status: form.status,
        notes: form.notes.trim() || undefined,
        reseller_id: form.reseller_id || undefined,
        sales_user_id: form.sales_user_id || undefined,
      };
      if (editId) return api(`/api/leads/${editId}`, { method: "PUT", body: JSON.stringify(body) });
      return api("/api/leads", { method: "POST", body: JSON.stringify(body) });
    },
    onSuccess: () => {
      const wasEdit = Boolean(editId);
      closeForm();
      qc.invalidateQueries({ queryKey: ["leads"] });
      void toastSuccess(wasEdit ? "Lead diperbarui" : "Lead ditambahkan");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/leads/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["leads"] });
      void toastSuccess("Lead dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const moveStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      api(`/api/leads/${id}/status`, { method: "PATCH", body: JSON.stringify({ status }) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["leads"] });
    },
    onError: (e: Error) => {
      qc.invalidateQueries({ queryKey: ["leads"] });
      void toastError(e.message);
    },
  });

  const convert = useMutation({
    mutationFn: () =>
      api<{ customer: { id: string; customer_code: string }; lead: LeadRow }>(`/api/leads/${convertLead!.id}/convert`, {
        method: "POST",
        body: JSON.stringify({
          cluster_id: convertCluster || undefined,
          reseller_id: convertReseller || undefined,
          sales_user_id: convertStaff || undefined,
          commission_basis: convertBasis || undefined,
        }),
      }),
    onSuccess: (res) => {
      setConvertLead(null);
      setConvertCluster("");
      setConvertReseller("");
      setConvertStaff("");
      setConvertBasis("new_customer_flat");
      qc.invalidateQueries({ queryKey: ["leads"] });
      qc.invalidateQueries({ queryKey: ["customers"] });
      qc.invalidateQueries({ queryKey: ["commissions"] });
      qc.invalidateQueries({ queryKey: ["resellers"] });
      void toastSuccess(`Dikonversi ke pelanggan ${res.customer.customer_code}`);
    },
    onError: (e: Error) => void toastError(e.message),
  });

  function onValueCommit(next: Record<string, LeadRow[]>, meta: KanbanCommitMeta<LeadRow>) {
    const lead = next[meta.overContainer]?.[meta.overIndex];
    if (!lead) {
      setColumns(meta.previousValue);
      return;
    }
    if (meta.activeContainer === meta.overContainer) {
      // reorder dalam kolom — tidak perlu persist urutan ke API
      return;
    }
    if (meta.overContainer === "converted") {
      setColumns(meta.previousValue);
      openConvert(lead);
      return;
    }
    if (lead.status === "converted") {
      setColumns(meta.previousValue);
      return;
    }
    // optimistic board already applied; sync status
    setColumns(
      Object.fromEntries(
        Object.entries(next).map(([k, rows]) => [k, rows.map((r) => (r.id === lead.id ? { ...r, status: k } : r))]),
      ),
    );
    moveStatus.mutate({ id: lead.id, status: meta.overContainer });
  }

  return (
    <Section
      title="Lead / pipeline"
      actions={
        <Button type="button" onClick={openCreate}>
          + Tambah
        </Button>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Kanban dengan dynamic overlay: seret kartu antar kolom. Drop ke <strong>Converted</strong> membuka form
        konversi.
      </p>

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
                  {col.hint ? <p className="text-[10px] text-[var(--muted)]">{col.hint}</p> : null}
                </div>
                <Badge variant="outline">{(columns[col.id] ?? []).length}</Badge>
              </KanbanColumnHeader>
              <KanbanColumnContent>
                {(columns[col.id] ?? []).map((l) => {
                  const locked = l.status === "converted";
                  return (
                    <KanbanItem key={l.id} value={l.id} disabled={locked}>
                      <LeadCardBody
                        lead={l}
                        actions={
                          <>
                            {!locked && l.status !== "lost" ? (
                              <IconButton label="Convert ke pelanggan" onClick={() => openConvert(l)}>
                                <IconUserCheck />
                              </IconButton>
                            ) : null}
                            {!locked ? (
                              <IconButton label="Edit lead" onClick={() => openEdit(l)}>
                                <IconPencil />
                              </IconButton>
                            ) : null}
                            <IconButton
                              label="Hapus lead"
                              danger
                              onClick={async () => {
                                const ok = await confirm({
                                  title: "Hapus lead",
                                  description: `Hapus lead "${l.full_name}"?`,
                                  confirmLabel: "Hapus",
                                });
                                if (!ok) return;
                                remove.mutate(l.id);
                              }}
                            >
                              <IconTrash />
                            </IconButton>
                          </>
                        }
                      />
                    </KanbanItem>
                  );
                })}
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
            const lead = leadById.get(String(value));
            if (!lead) return null;
            return (
              <div className="w-[244px]">
                <LeadCardBody lead={lead} overlay />
              </div>
            );
          }}
        </KanbanOverlay>
      </Kanban>

      <FormDialog open={open} wide title={editId ? "Edit lead" : "Tambah lead"} onClose={closeForm}>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <Input
            placeholder="Nama lengkap"
            value={form.full_name}
            onChange={(e) => setForm({ ...form, full_name: e.target.value })}
            required
          />
          <Input
            placeholder="Telepon"
            value={form.phone}
            onChange={(e) => setForm({ ...form, phone: e.target.value })}
            required
          />
          <Input
            placeholder="Email (opsional)"
            value={form.email}
            onChange={(e) => setForm({ ...form, email: e.target.value })}
          />
          <Select value={form.status} onValueChange={(v) => setForm({ ...form, status: v })}>
            <SelectTrigger>
              <SelectValue placeholder="Status" />
            </SelectTrigger>
            <SelectContent>
              {COLUMNS.filter((c) => c.id !== "converted").map((c) => (
                <SelectItem key={c.id} value={c.id}>
                  {c.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input
            className="sm:col-span-2"
            placeholder="Alamat"
            value={form.address}
            onChange={(e) => setForm({ ...form, address: e.target.value })}
          />
          <Input
            className="sm:col-span-2"
            placeholder="Catatan"
            value={form.notes}
            onChange={(e) => setForm({ ...form, notes: e.target.value })}
          />
          <AttributionSelects
            resellerId={form.reseller_id}
            salesUserId={form.sales_user_id}
            resellers={resellers}
            users={users}
            onChange={(next) => setForm({ ...form, reseller_id: next.reseller_id, sales_user_id: next.sales_user_id })}
          />
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button type="button" variant="secondary" onClick={closeForm}>
              Batal
            </Button>
          </div>
          {formErr && <p className="text-sm text-[var(--danger)] sm:col-span-2">{formErr}</p>}
        </form>
      </FormDialog>

      <FormDialog
        open={Boolean(convertLead)}
        wide
        title="Convert ke pelanggan"
        onClose={() => {
          setConvertLead(null);
          setConvertCluster("");
          setConvertReseller("");
          setConvertStaff("");
          setConvertBasis("new_customer_flat");
        }}
      >
        <form
          className="grid gap-4 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            convert.mutate();
          }}
        >
          <div className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/50 p-3 sm:col-span-2">
            <p className="text-xs font-medium uppercase tracking-wide text-[var(--muted)]">Lead</p>
            <p className="mt-1 text-sm font-semibold text-[var(--text)]">{convertLead?.full_name}</p>
            <p className="text-sm text-[var(--muted)]">{convertLead?.phone}</p>
            {convertLead?.address ? (
              <p className="mt-1 text-xs text-[var(--muted)]">{convertLead.address}</p>
            ) : null}
            {convertLead && attributionLabel(convertLead) ? (
              <Badge className="mt-2" variant="default">
                {attributionLabel(convertLead)}
              </Badge>
            ) : null}
          </div>

          <div className="sm:col-span-2">
            <Label className="mb-1.5 block">Cluster (opsional)</Label>
            <Select value={convertCluster || "__none__"} onValueChange={(v) => setConvertCluster(v === "__none__" ? "" : v)}>
              <SelectTrigger>
                <SelectValue placeholder="Cluster" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__none__">— Tanpa cluster —</SelectItem>
                {clusters.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name} ({c.code})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <AttributionSelects
            resellerId={convertReseller}
            salesUserId={convertStaff}
            resellers={resellers}
            users={users}
            onChange={(next) => {
              setConvertReseller(next.reseller_id);
              setConvertStaff(next.sales_user_id);
            }}
          />

          <CommissionBasisSelect value={convertBasis} onChange={setConvertBasis} />

          <div className="flex flex-wrap gap-2 border-t border-[var(--border)] pt-3 sm:col-span-2">
            <Button type="submit" disabled={convert.isPending}>
              {convert.isPending ? "Mengonversi…" : "Convert"}
            </Button>
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setConvertLead(null);
                setConvertCluster("");
                setConvertReseller("");
                setConvertStaff("");
                setConvertBasis("new_customer_flat");
              }}
            >
              Batal
            </Button>
          </div>
        </form>
      </FormDialog>
    </Section>
  );
}
