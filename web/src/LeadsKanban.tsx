import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LayoutGrid, List } from "lucide-react";
import { api, apiUpload } from "./api";
import { ProgressFileUpload } from "./ProgressFileUpload";
import {
  AttributionSelects,
  CommissionBasisSelect,
  type ResellerOpt,
  type StaffOpt,
} from "./AdminExtra";
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
import { IconPencil, IconTrash, IconUserCheck, IconWrench } from "./icons";
import { canDispatchOps, type MePermissions } from "./permissions";
import { toastError, toastSuccess } from "./swal";
import { ListToolbar, matchesQuery } from "./ListToolbar";
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
} from "./ui";

type LeadRow = {
  id: string;
  full_name: string;
  phone: string;
  email?: string | null;
  address?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  identity_type?: string | null;
  identity_number?: string | null;
  status: string;
  notes?: string | null;
  odp_id?: string | null;
  odp_code?: string;
  odp_name?: string;
  reseller_id?: string | null;
  reseller_name?: string;
  sales_user_id?: string | null;
  sales_user_name?: string;
  assigned_to?: string | null;
  assigned_to_name?: string;
  customer_id?: string | null;
  created_at?: string;
};

type LeadComment = {
  id: string;
  lead_id: string;
  user_id?: string | null;
  user_name?: string;
  avatar_url?: string | null;
  kind?: string;
  message: string;
  image_urls: string[];
  created_at: string;
};

type LeadDocument = {
  id: string;
  lead_id: string;
  kind: string;
  url: string;
  caption?: string;
  uploader_name?: string;
  created_at: string;
};

const DOC_KINDS: { id: string; label: string }[] = [
  { id: "ktp", label: "KTP / identitas" },
  { id: "rumah", label: "Rumah / lokasi" },
  { id: "odp", label: "ODP" },
  { id: "psb", label: "Proses PSB" },
  { id: "other", label: "Lainnya" },
];

function docKindLabel(kind: string) {
  return DOC_KINDS.find((k) => k.id === kind)?.label || kind;
}

type ClusterOpt = { id: string; name: string; code: string };

type LeadForm = {
  full_name: string;
  phone: string;
  email: string;
  address: string;
  latitude: string;
  longitude: string;
  identity_type: string;
  identity_number: string;
  status: string;
  notes: string;
  reseller_id: string;
  sales_user_id: string;
  assigned_to: string;
};

const emptyForm: LeadForm = {
  full_name: "",
  phone: "",
  email: "",
  address: "",
  latitude: "",
  longitude: "",
  identity_type: "ktp",
  identity_number: "",
  status: "new",
  notes: "",
  reseller_id: "",
  sales_user_id: "",
  assigned_to: "",
};

const IDENTITY_TYPES: { id: string; label: string }[] = [
  { id: "ktp", label: "KTP" },
  { id: "sim", label: "SIM" },
  { id: "passport", label: "Paspor" },
  { id: "other", label: "Lainnya" },
];

export function identityLabel(t?: string | null) {
  return IDENTITY_TYPES.find((x) => x.id === (t || "").toLowerCase())?.label || "—";
}

const COLUMNS: { id: string; label: string; hint?: string }[] = [
  { id: "new", label: "Baru" },
  { id: "contacted", label: "Dihubungi", hint: "Assign teknisi" },
  { id: "survey", label: "Survey" },
  { id: "qualified", label: "Proses pasang" },
  { id: "converted", label: "Converted", hint: "Drop untuk convert" },
  { id: "lost", label: "Lost" },
];

const COLUMN_IDS = COLUMNS.map((c) => c.id);

const ASSIGN_STATUSES = new Set(["contacted", "survey", "qualified"]);

function needsAssignee(status: string) {
  return ASSIGN_STATUSES.has(status);
}

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

function formatWhen(iso?: string | null) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("id-ID");
  } catch {
    return iso;
  }
}

function statusLabel(status: string) {
  return COLUMNS.find((c) => c.id === status)?.label || status;
}

const LEADS_VIEW_KEY = "drp_leads_view";

function LeadCardBody({
  lead,
  overlay,
  actions,
  onOpen,
}: {
  lead: LeadRow;
  overlay?: boolean;
  actions?: ReactNode;
  onOpen?: () => void;
}) {
  const attr = attributionLabel(lead);
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
        {lead.assigned_to_name ? (
          <Badge variant="outline">Teknisi: {lead.assigned_to_name}</Badge>
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

export function LeadsPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();

  const meQ = useQuery({
    queryKey: ["me"],
    queryFn: () => api<MePermissions & { user_id: string }>("/api/me"),
  });
  const canDispatch = canDispatchOps(meQ.data?.permissions);
  const [showHistory, setShowHistory] = useState(false);

  const q = useQuery({
    queryKey: ["leads", "kanban", showHistory],
    queryFn: () =>
      api<{ data: LeadRow[]; total: number }>(
        `/api/leads?limit=200&offset=0${showHistory ? "" : "&hide_converted=true"}`,
      ),
  });
  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterOpt[]>("/api/clusters"),
    enabled: canDispatch,
  });
  const resellersQ = useQuery({
    queryKey: ["resellers"],
    queryFn: () => api<ResellerOpt[]>("/api/resellers"),
    enabled: canDispatch,
  });
  const usersQ = useQuery({
    queryKey: ["tenant-users"],
    queryFn: () => api<StaffOpt[]>("/api/users/options"),
    enabled: canDispatch,
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
  const [convertLatitude, setConvertLatitude] = useState("");
  const [convertLongitude, setConvertLongitude] = useState("");
  const [convertIdentityType, setConvertIdentityType] = useState("ktp");
  const [convertIdentityNumber, setConvertIdentityNumber] = useState("");
  const [assignLead, setAssignLead] = useState<LeadRow | null>(null);
  const [assignUser, setAssignUser] = useState("");
  const [assignTargetStatus, setAssignTargetStatus] = useState("contacted");
  const [view, setView] = useState<"kanban" | "list">(() => {
    try {
      const v = localStorage.getItem(LEADS_VIEW_KEY);
      return v === "list" ? "list" : "kanban";
    } catch {
      return "kanban";
    }
  });
  const [listStatus, setListStatus] = useState("");
  const [leadSearch, setLeadSearch] = useState("");
  const [detail, setDetail] = useState<LeadRow | null>(null);
  const [commentText, setCommentText] = useState("");
  const [pendingImages, setPendingImages] = useState<string[]>([]);
  const [docKind, setDocKind] = useState("psb");

  function setViewMode(next: "kanban" | "list") {
    setView(next);
    try {
      localStorage.setItem(LEADS_VIEW_KEY, next);
    } catch {
      /* ignore */
    }
  }

  const clusters = Array.isArray(clustersQ.data) ? clustersQ.data : [];
  const resellers = Array.isArray(resellersQ.data) ? resellersQ.data : [];
  const users = Array.isArray(usersQ.data) ? usersQ.data : [];
  const assignCandidates = (() => {
    const active = users.filter((u) => u.is_active);
    const techs = active.filter((u) => u.role_slug === "teknisi");
    return techs.length > 0 ? techs : active;
  })();
  const list = q.data?.data ?? [];
  const filteredList = useMemo(
    () =>
      list.filter((l) => {
        if (listStatus && l.status !== listStatus) return false;
        return matchesQuery(
          leadSearch,
          l.full_name,
          l.phone,
          l.address,
          l.notes,
          l.assigned_to_name,
          l.reseller_name,
          l.sales_user_name,
        );
      }),
    [list, listStatus, leadSearch],
  );

  const commentsQ = useQuery({
    queryKey: ["lead-comments", detail?.id],
    queryFn: () => api<LeadComment[]>(`/api/leads/${detail!.id}/comments`),
    enabled: Boolean(detail?.id),
  });
  const comments = Array.isArray(commentsQ.data) ? commentsQ.data : [];

  const docsQ = useQuery({
    queryKey: ["lead-documents", detail?.id],
    queryFn: () => api<LeadDocument[]>(`/api/leads/${detail!.id}/documents`),
    enabled: Boolean(detail?.id),
  });
  const leadDocs = Array.isArray(docsQ.data) ? docsQ.data : [];
  const canEditLeadDocs = detail?.status === "survey" || detail?.status === "qualified";

  useEffect(() => {
    setColumns(boardFromList(list));
  }, [list]);

  useEffect(() => {
    if (!detail) {
      setCommentText("");
      setPendingImages([]);
    }
  }, [detail]);

  const leadById = useMemo(() => {
    const map = new Map<string, LeadRow>();
    for (const col of Object.values(columns)) {
      for (const l of col) map.set(l.id, l);
    }
    return map;
  }, [columns]);

  function openDetail(l: LeadRow) {
    setDetail(l);
    setCommentText("");
    setPendingImages([]);
  }

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
      latitude: l.latitude != null ? String(l.latitude) : "",
      longitude: l.longitude != null ? String(l.longitude) : "",
      identity_type: l.identity_type || "ktp",
      identity_number: l.identity_number || "",
      status: l.status || "new",
      notes: l.notes || "",
      reseller_id: l.reseller_id || "",
      sales_user_id: l.sales_user_id || "",
      assigned_to: l.assigned_to || "",
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
  function openAssign(l: LeadRow, targetStatus = "contacted") {
    setAssignLead(l);
    setAssignUser(l.assigned_to || "");
    setAssignTargetStatus(targetStatus);
  }
  function closeAssign() {
    setAssignLead(null);
    setAssignUser("");
    setAssignTargetStatus("contacted");
  }
  function openConvert(l: LeadRow) {
    setConvertLead(l);
    setConvertCluster("");
    setConvertReseller(l.reseller_id || "");
    setConvertStaff(l.sales_user_id || "");
    setConvertBasis("new_customer_flat");
    setConvertLatitude(l.latitude != null ? String(l.latitude) : "");
    setConvertLongitude(l.longitude != null ? String(l.longitude) : "");
    setConvertIdentityType(l.identity_type || "ktp");
    setConvertIdentityNumber(l.identity_number || "");
  }

  function resetConvertForm() {
    setConvertLead(null);
    setConvertCluster("");
    setConvertReseller("");
    setConvertStaff("");
    setConvertBasis("new_customer_flat");
    setConvertLatitude("");
    setConvertLongitude("");
    setConvertIdentityType("ktp");
    setConvertIdentityNumber("");
  }

  const save = useMutation({
    mutationFn: () => {
      if (needsAssignee(form.status) && !form.assigned_to) {
        throw new Error("Pilih teknisi mulai status dihubungi");
      }
      const body: Record<string, unknown> = {
        full_name: form.full_name.trim(),
        phone: form.phone.trim(),
        email: form.email.trim() || undefined,
        address: form.address.trim() || undefined,
        status: form.status,
        notes: form.notes.trim() || undefined,
        reseller_id: form.reseller_id || undefined,
        sales_user_id: form.sales_user_id || undefined,
        assigned_to: needsAssignee(form.status) ? form.assigned_to || undefined : undefined,
      };
      if (form.latitude.trim()) body.latitude = Number(form.latitude);
      if (form.longitude.trim()) body.longitude = Number(form.longitude);
      if (form.identity_type) body.identity_type = form.identity_type;
      if (form.identity_number.trim()) body.identity_number = form.identity_number.trim();
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
    mutationFn: ({ id, status, assigned_to }: { id: string; status: string; assigned_to?: string }) =>
      api<LeadRow>(`/api/leads/${id}/status`, {
        method: "PATCH",
        body: JSON.stringify({ status, assigned_to: assigned_to || undefined }),
      }),
    onSuccess: (updated) => {
      closeAssign();
      qc.invalidateQueries({ queryKey: ["leads"] });
      qc.invalidateQueries({ queryKey: ["lead-comments"] });
      if (detail && updated?.id === detail.id) {
        setDetail(updated);
      }
      void toastSuccess("Status lead diperbarui");
    },
    onError: (e: Error) => {
      qc.invalidateQueries({ queryKey: ["leads"] });
      void toastError(e.message);
    },
  });

  const reassign = useMutation({
    mutationFn: ({ id, assigned_to }: { id: string; assigned_to: string }) =>
      api(`/api/leads/${id}/assign`, {
        method: "PATCH",
        body: JSON.stringify({ assigned_to }),
      }),
    onSuccess: () => {
      closeAssign();
      qc.invalidateQueries({ queryKey: ["leads"] });
      void toastSuccess("Teknisi di-assign");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const convert = useMutation({
    mutationFn: async () => {
      const lead = convertLead!;
      const body: Record<string, unknown> = {
        cluster_id: convertCluster || undefined,
        reseller_id: convertReseller || undefined,
        sales_user_id: convertStaff || undefined,
        commission_basis: convertBasis || undefined,
      };
      if (convertLatitude.trim()) body.latitude = Number(convertLatitude);
      if (convertLongitude.trim()) body.longitude = Number(convertLongitude);
      if (convertIdentityType) body.identity_type = convertIdentityType;
      if (convertIdentityNumber.trim()) body.identity_number = convertIdentityNumber.trim();
      return api<{ customer: { id: string; customer_code: string }; lead: LeadRow }>(
        `/api/leads/${lead.id}/convert`,
        { method: "POST", body: JSON.stringify(body) },
      );
    },
    onSuccess: (res) => {
      resetConvertForm();
      qc.invalidateQueries({ queryKey: ["leads"] });
      qc.invalidateQueries({ queryKey: ["customers"] });
      qc.invalidateQueries({ queryKey: ["commissions"] });
      qc.invalidateQueries({ queryKey: ["resellers"] });
      void toastSuccess(`Dikonversi ke pelanggan ${res.customer.customer_code}`);
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const sendComment = useMutation({
    mutationFn: () =>
      api<LeadComment>(`/api/leads/${detail!.id}/comments`, {
        method: "POST",
        body: JSON.stringify({
          message: commentText.trim(),
          image_urls: pendingImages.length ? pendingImages : undefined,
        }),
      }),
    onSuccess: () => {
      setCommentText("");
      setPendingImages([]);
      qc.invalidateQueries({ queryKey: ["lead-comments", detail?.id] });
      void toastSuccess("Komentar dikirim");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const removeLeadDoc = useMutation({
    mutationFn: (docId: string) => api(`/api/leads/${detail!.id}/documents/${docId}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["lead-documents", detail?.id] });
      void toastSuccess("Dokumen dihapus");
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
      if (!canDispatch) {
        void toastError("Hanya dispatcher yang bisa convert lead");
        return;
      }
      openConvert(lead);
      return;
    }
    if (lead.status === "converted") {
      setColumns(meta.previousValue);
      return;
    }
    if (!canDispatch) {
      if (meta.overContainer === "new") {
        setColumns(meta.previousValue);
        void toastError("Teknisi tidak bisa mengembalikan lead ke Baru");
        return;
      }
      setColumns(
        Object.fromEntries(
          Object.entries(next).map(([k, rows]) => [k, rows.map((r) => (r.id === lead.id ? { ...r, status: k } : r))]),
        ),
      );
      moveStatus.mutate({ id: lead.id, status: meta.overContainer });
      return;
    }
    if (meta.overContainer === "qualified" || meta.overContainer === "survey" || meta.overContainer === "contacted") {
      if (!lead.assigned_to) {
        setColumns(meta.previousValue);
        openAssign(lead, meta.overContainer);
        return;
      }
      setColumns(
        Object.fromEntries(
          Object.entries(next).map(([k, rows]) => [k, rows.map((r) => (r.id === lead.id ? { ...r, status: k } : r))]),
        ),
      );
      moveStatus.mutate({ id: lead.id, status: meta.overContainer, assigned_to: lead.assigned_to });
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
        <div className="flex flex-wrap items-center gap-2">
          <label className="inline-flex cursor-pointer items-center gap-1.5 text-xs text-[var(--muted)]">
            <input
              type="checkbox"
              checked={showHistory}
              onChange={(e) => setShowHistory(e.target.checked)}
            />
            Riwayat converted
          </label>
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
            <Button type="button" onClick={openCreate}>
              + Tambah
            </Button>
          ) : null}
        </div>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        {!canDispatch
          ? "Lead yang di-assign ke Anda. Seret antar kolom untuk ubah status, atau buka detail untuk komentar & foto."
          : view === "kanban"
            ? "Kanban: seret ke Dihubungi untuk assign teknisi. Drop ke Converted membuka form konversi. Converted lebih dari 7 hari disembunyikan otomatis (centang Riwayat converted untuk melihat)."
            : "Daftar lead: assign teknisi mulai status dihubungi, komentar & lampiran gambar. Converted lebih dari 7 hari disembunyikan otomatis."}
      </p>

      {view === "list" ? (
        <div className="space-y-4">
          <ListToolbar
            search={leadSearch}
            onSearchChange={setLeadSearch}
            searchPlaceholder="Nama, telepon, alamat, teknisi…"
            filters={[
              {
                key: "status",
                label: "Status",
                value: listStatus,
                onChange: setListStatus,
                options: COLUMNS.map((c) => ({ value: c.id, label: c.label })),
              },
            ]}
            total={filteredList.length}
          />
          <Table
            columns={["Nama", "Telepon", "Status", "Teknisi", "Atribusi", "Dibuat", "Aksi"]}
            onRowClick={(i) => openDetail(filteredList[i])}
            rows={filteredList.map((l) => [
              <div key={`${l.id}-n`}>
                <p className="font-medium">{l.full_name}</p>
                {l.address ? <p className="line-clamp-1 text-xs text-[var(--muted)]">{l.address}</p> : null}
              </div>,
              l.phone,
              <Badge key={`${l.id}-s`} variant="outline">
                {statusLabel(l.status)}
              </Badge>,
              l.assigned_to_name || "—",
              attributionLabel(l) || "—",
              formatWhen(l.created_at),
              <span key={`${l.id}-a`} className="flex flex-wrap items-center gap-1.5">
                {canDispatch && needsAssignee(l.status) ? (
                  <IconButton label="Assign teknisi" onClick={() => openAssign(l, l.status)}>
                    <IconWrench />
                  </IconButton>
                ) : null}
                {canDispatch && l.status !== "converted" ? (
                  <>
                    <IconButton label="Edit" onClick={() => openEdit(l)}>
                      <IconPencil />
                    </IconButton>
                    <IconButton label="Convert ke pelanggan" onClick={() => openConvert(l)}>
                      <IconUserCheck />
                    </IconButton>
                    <IconButton
                      label="Hapus"
                      danger
                      onClick={() => {
                        void confirm({
                          title: "Hapus lead?",
                          description: `Hapus ${l.full_name}?`,
                          confirmLabel: "Hapus",
                          danger: true,
                        }).then((ok) => {
                          if (ok) remove.mutate(l.id);
                        });
                      }}
                    >
                      <IconTrash />
                    </IconButton>
                  </>
                ) : (
                  <span className="text-xs text-[var(--muted)]">Klik baris untuk detail</span>
                )}
              </span>,
            ])}
          />
          {filteredList.length === 0 ? (
            <p className="text-sm text-[var(--muted)]">Tidak ada lead{listStatus ? " dengan status ini" : ""}.</p>
          ) : null}
        </div>
      ) : (
      <>
      <ListToolbar
        search={leadSearch}
        onSearchChange={setLeadSearch}
        searchPlaceholder="Filter kanban: nama, telepon…"
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
          {COLUMNS.map((col) => {
            const cards = (columns[col.id] ?? []).filter((l) =>
              matchesQuery(leadSearch, l.full_name, l.phone, l.address, l.notes, l.assigned_to_name),
            );
            return (
            <KanbanColumn key={col.id} value={col.id}>
              <KanbanColumnHeader>
                <div>
                  <p className="text-sm font-semibold text-[var(--text)]">{col.label}</p>
                  {col.hint ? <p className="text-[10px] text-[var(--muted)]">{col.hint}</p> : null}
                </div>
                <Badge variant="outline">{cards.length}</Badge>
              </KanbanColumnHeader>
              <KanbanColumnContent>
                {cards.map((l) => {
                  const locked = l.status === "converted";
                  return (
                    <KanbanItem key={l.id} value={l.id} disabled={locked}>
                      <LeadCardBody
                        lead={l}
                        onOpen={() => openDetail(l)}
                        actions={
                          <>
                            {canDispatch && needsAssignee(l.status) ? (
                              <IconButton label="Assign teknisi" onClick={() => openAssign(l, l.status)}>
                                <IconWrench />
                              </IconButton>
                            ) : null}
                            {canDispatch && !locked && l.status !== "lost" ? (
                              <IconButton label="Convert ke pelanggan" onClick={() => openConvert(l)}>
                                <IconUserCheck />
                              </IconButton>
                            ) : null}
                            {canDispatch && l.status !== "converted" ? (
                              <IconButton label="Edit lead" onClick={() => openEdit(l)}>
                                <IconPencil />
                              </IconButton>
                            ) : null}
                            {canDispatch ? (
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
                            ) : null}
                          </>
                        }
                      />
                    </KanbanItem>
                  );
                })}
                {cards.length === 0 ? (
                  <p className="px-2 py-6 text-center text-xs text-[var(--muted)]">Kosong</p>
                ) : null}
              </KanbanColumnContent>
            </KanbanColumn>
            );
          })}
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
      </>
      )}

      <FormDialog
        open={Boolean(detail)}
        wide
        title={detail ? `Lead: ${detail.full_name}` : "Detail lead"}
        onClose={() => setDetail(null)}
      >
        {detail ? (
          <div className="grid gap-4">
            <div className="grid gap-2 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/40 p-3 text-sm sm:grid-cols-2">
              <p>
                <span className="text-[var(--muted)]">Status:</span> {statusLabel(detail.status)}
              </p>
              <p>
                <span className="text-[var(--muted)]">Teknisi:</span> {detail.assigned_to_name || "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">Telepon:</span> {detail.phone}
              </p>
              <p>
                <span className="text-[var(--muted)]">Email:</span> {detail.email || "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">Atribusi:</span> {attributionLabel(detail) || "—"}
              </p>
              <p className="sm:col-span-2">
                <span className="text-[var(--muted)]">Alamat:</span> {detail.address || "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">Koordinat:</span>{" "}
                {detail.latitude != null && detail.longitude != null
                  ? `${detail.latitude}, ${detail.longitude}`
                  : "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">Identitas:</span>{" "}
                {detail.identity_number ? `${identityLabel(detail.identity_type)} · ${detail.identity_number}` : "—"}
              </p>
              {detail.notes ? (
                <p className="sm:col-span-2">
                  <span className="text-[var(--muted)]">Catatan:</span> {detail.notes}
                </p>
              ) : null}
            </div>

            {detail.status !== "converted" ? (
              <div className="flex flex-wrap gap-2">
                {(canDispatch
                  ? COLUMNS.filter((c) => c.id !== "converted" && c.id !== detail.status)
                  : COLUMNS.filter((c) => !["converted", "new", detail.status].includes(c.id))
                ).map((c) => (
                  <Button
                    key={c.id}
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={moveStatus.isPending}
                    onClick={() => {
                      if (canDispatch && needsAssignee(c.id) && !detail.assigned_to) {
                        openAssign(detail, c.id);
                        return;
                      }
                      moveStatus.mutate({
                        id: detail.id,
                        status: c.id,
                        assigned_to: detail.assigned_to || undefined,
                      });
                    }}
                  >
                    → {c.label}
                  </Button>
                ))}
              </div>
            ) : null}

            <div>
              <h4 className="mb-2 text-sm font-semibold">Dokumentasi / Galeri PSB</h4>
              <p className="mb-2 text-xs text-[var(--muted)]">
                Foto KTP, lokasi, ODP, dan proses pasang. Saat convert, gambar ini jadi galeri pelanggan.
              </p>
              {docsQ.isLoading ? (
                <p className="text-sm text-[var(--muted)]">Memuat…</p>
              ) : leadDocs.length === 0 ? (
                <p className="mb-2 text-sm text-[var(--muted)]">Belum ada dokumen.</p>
              ) : (
                <div className="mb-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
                  {leadDocs.map((d) => (
                    <div key={d.id} className="relative overflow-hidden rounded-md border border-[var(--border)]">
                      <a href={d.url} target="_blank" rel="noreferrer" className="block">
                        <img src={d.url} alt="" className="h-28 w-full object-cover" />
                      </a>
                      <div className="flex items-center justify-between gap-1 px-1.5 py-1 text-[10px]">
                        <span className="truncate text-[var(--muted)]">{docKindLabel(d.kind)}</span>
                        {canEditLeadDocs ? (
                          <IconButton
                            label="Hapus dokumen"
                            danger
                            onClick={() => {
                              void confirm({
                                title: "Hapus dokumen?",
                                description: docKindLabel(d.kind),
                                confirmLabel: "Hapus",
                                danger: true,
                              }).then((ok) => {
                                if (ok) removeLeadDoc.mutate(d.id);
                              });
                            }}
                          >
                            <IconTrash />
                          </IconButton>
                        ) : null}
                      </div>
                    </div>
                  ))}
                </div>
              )}
              {canEditLeadDocs ? (
                <div className="grid gap-2">
                  <div className="min-w-[140px] max-w-xs">
                    <Label className="mb-1 block text-xs">Jenis</Label>
                    <select className="input" value={docKind} onChange={(e) => setDocKind(e.target.value)}>
                      {DOC_KINDS.map((k) => (
                        <option key={k.id} value={k.id}>
                          {k.label}
                        </option>
                      ))}
                    </select>
                  </div>
                  <ProgressFileUpload
                    label="Unggah gambar"
                    hint="Progress per file terlihat di bawah"
                    uploadFile={async (file, onProgress) => {
                      if (!detail) throw new Error("Lead tidak dipilih");
                      const up = await apiUpload<{ url: string }>(
                        `/api/leads/${detail.id}/documents/photos`,
                        file,
                        { onProgress },
                      );
                      await api(`/api/leads/${detail.id}/documents`, {
                        method: "POST",
                        body: JSON.stringify({ kind: docKind, url: up.url }),
                      });
                      return up.url;
                    }}
                    onBatchComplete={(urls) => {
                      void qc.invalidateQueries({ queryKey: ["lead-documents", detail?.id] });
                      void toastSuccess(urls.length > 1 ? `${urls.length} dokumen diunggah` : "Dokumen diunggah");
                    }}
                  />
                </div>
              ) : (
                <p className="text-xs text-[var(--muted)]">Unggah dokumen saat status Survey / Proses pasang.</p>
              )}
            </div>

            <div>
              <h4 className="mb-2 text-sm font-semibold">Aktivitas</h4>
              <div className="mb-3 max-h-56 space-y-3 overflow-y-auto">
                {commentsQ.isLoading ? (
                  <p className="text-sm text-[var(--muted)]">Memuat…</p>
                ) : comments.length === 0 ? (
                  <p className="text-sm text-[var(--muted)]">Belum ada aktivitas.</p>
                ) : (
                  comments.map((c) => {
                    const isStatus = c.kind === "status_change";
                    const meId = meQ.data?.user_id;
                    const displayName = nameWithSaya(
                      isStatus ? `${c.user_name || "Sistem"} · pindah status` : c.user_name || "Tim",
                      c.user_id,
                      meId,
                    );
                    return (
                      <div
                        key={c.id}
                        className={
                          isStatus
                            ? "rounded-md border border-dashed border-[var(--border)] bg-[var(--panel-muted)]/50 px-2 py-1.5 text-sm"
                            : "rounded-md border border-[var(--border)] p-2 text-sm"
                        }
                      >
                        <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
                          <span className="inline-flex min-w-0 items-center gap-1.5 font-medium">
                            <UserAvatar name={c.user_name} avatarUrl={c.avatar_url} />
                            <span className="truncate">{displayName}</span>
                          </span>
                          <span className="text-[10px] text-[var(--muted)]">{formatWhen(c.created_at)}</span>
                        </div>
                        {c.message ? (
                          <p className={isStatus ? "text-[var(--muted)]" : "whitespace-pre-wrap"}>{c.message}</p>
                        ) : null}
                        {!isStatus && (c.image_urls?.length ?? 0) > 0 ? (
                          <div className="mt-2 flex flex-wrap gap-2">
                            {c.image_urls.map((url) => (
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

              <div className="grid gap-2">
                <Input
                  value={commentText}
                  onChange={(e) => setCommentText(e.target.value)}
                  placeholder="Tulis komentar…"
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      if (commentText.trim() || pendingImages.length) sendComment.mutate();
                    }
                  }}
                />
                <ProgressFileUpload
                  label="Lampirkan gambar"
                  hint="Gambar masuk antrian lampiran komentar"
                  uploadFile={async (file, onProgress) => {
                    if (!detail) throw new Error("Lead tidak dipilih");
                    const res = await apiUpload<{ url: string }>(
                      `/api/leads/${detail.id}/comments/photos`,
                      file,
                      { onProgress },
                    );
                    return res.url;
                  }}
                  onBatchComplete={(urls) => {
                    setPendingImages((prev) => [...prev, ...urls]);
                    void toastSuccess(urls.length > 1 ? `${urls.length} gambar siap dilampirkan` : "Gambar siap dilampirkan");
                  }}
                />
                <Button
                  type="button"
                  disabled={sendComment.isPending || (!commentText.trim() && pendingImages.length === 0)}
                  onClick={() => sendComment.mutate()}
                >
                  {sendComment.isPending ? "Mengirim…" : "Kirim"}
                </Button>
              </div>
            </div>
          </div>
        ) : null}
      </FormDialog>

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
          <Select
            value={form.status}
            onValueChange={(v) =>
              setForm({
                ...form,
                status: v,
                assigned_to: needsAssignee(v) ? form.assigned_to : "",
              })
            }
          >
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
          {needsAssignee(form.status) ? (
            <div>
              <Label className="mb-1.5 block">Teknisi (wajib)</Label>
              <SearchableSelect
                required
                placeholder="Pilih teknisi…"
                searchPlaceholder="Cari nama teknisi…"
                value={form.assigned_to}
                onValueChange={(v) => setForm({ ...form, assigned_to: v })}
                options={assignCandidates.map((u) => ({
                  value: u.user_id,
                  label: `${u.full_name || u.email}${u.role_slug ? ` · ${u.role_slug}` : ""}`,
                  keywords: `${u.full_name || ""} ${u.email || ""} ${u.role_slug || ""}`,
                }))}
              />
            </div>
          ) : null}
          <Input
            className="sm:col-span-2"
            placeholder="Alamat"
            value={form.address}
            onChange={(e) => setForm({ ...form, address: e.target.value })}
          />
          <Input
            type="number"
            step="any"
            placeholder="Latitude (peta)"
            value={form.latitude}
            onChange={(e) => setForm({ ...form, latitude: e.target.value })}
          />
          <Input
            type="number"
            step="any"
            placeholder="Longitude (peta)"
            value={form.longitude}
            onChange={(e) => setForm({ ...form, longitude: e.target.value })}
          />
          <Select value={form.identity_type} onValueChange={(v) => setForm({ ...form, identity_type: v })}>
            <SelectTrigger>
              <SelectValue placeholder="Jenis identitas" />
            </SelectTrigger>
            <SelectContent>
              {IDENTITY_TYPES.map((t) => (
                <SelectItem key={t.id} value={t.id}>
                  {t.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input
            placeholder="Nomor identitas"
            value={form.identity_number}
            onChange={(e) => setForm({ ...form, identity_number: e.target.value })}
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
        onClose={resetConvertForm}
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
            <p className="mb-1.5 text-sm font-medium">Lokasi & identitas <span className="font-normal text-[var(--muted)]">(masuk ke data pelanggan)</span></p>
            <div className="grid gap-3 sm:grid-cols-2">
              <Input
                type="number"
                step="any"
                placeholder="Latitude (peta)"
                value={convertLatitude}
                onChange={(e) => setConvertLatitude(e.target.value)}
              />
              <Input
                type="number"
                step="any"
                placeholder="Longitude (peta)"
                value={convertLongitude}
                onChange={(e) => setConvertLongitude(e.target.value)}
              />
              <Select value={convertIdentityType} onValueChange={setConvertIdentityType}>
                <SelectTrigger>
                  <SelectValue placeholder="Jenis identitas" />
                </SelectTrigger>
                <SelectContent>
                  {IDENTITY_TYPES.map((t) => (
                    <SelectItem key={t.id} value={t.id}>
                      {t.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Input
                placeholder="Nomor identitas"
                value={convertIdentityNumber}
                onChange={(e) => setConvertIdentityNumber(e.target.value)}
              />
            </div>
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
            <Button type="button" variant="secondary" onClick={resetConvertForm}>
              Batal
            </Button>
          </div>
        </form>
      </FormDialog>

      <FormDialog
        open={Boolean(assignLead)}
        title={
          assignLead && needsAssignee(assignLead.status) && assignLead.status === assignTargetStatus
            ? "Assign teknisi"
            : `${statusLabel(assignTargetStatus)} — assign teknisi`
        }
        onClose={closeAssign}
      >
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!assignLead || !assignUser) {
              void toastError("Pilih teknisi");
              return;
            }
            if (needsAssignee(assignLead.status) && assignLead.status === assignTargetStatus) {
              reassign.mutate({ id: assignLead.id, assigned_to: assignUser });
            } else {
              moveStatus.mutate({ id: assignLead.id, status: assignTargetStatus, assigned_to: assignUser });
            }
          }}
        >
          <div className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/50 p-3">
            <p className="text-sm font-semibold">{assignLead?.full_name}</p>
            <p className="text-sm text-[var(--muted)]">{assignLead?.phone}</p>
          </div>
          <div>
            <Label className="mb-1.5 block">Teknisi</Label>
            <SearchableSelect
              required
              placeholder="Pilih teknisi…"
              searchPlaceholder="Cari teknisi…"
              value={assignUser}
              onValueChange={setAssignUser}
              options={assignCandidates.map((u) => ({
                value: u.user_id,
                label: `${u.full_name || u.email}${u.role_slug ? ` · ${u.role_slug}` : ""}`,
                keywords: `${u.full_name || ""} ${u.email || ""} ${u.role_slug || ""}`,
              }))}
            />
            <p className="mt-1.5 text-xs text-[var(--muted)]">
              Assign wajib mulai Dihubungi. Hanya teknisi yang di-assign yang melihat lead ini.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={moveStatus.isPending || reassign.isPending || !assignUser}>
              {moveStatus.isPending || reassign.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button type="button" variant="secondary" onClick={closeAssign}>
              Batal
            </Button>
          </div>
        </form>
      </FormDialog>
    </Section>
  );
}
