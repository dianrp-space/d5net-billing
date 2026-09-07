import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { ListToolbar, useDebouncedValue } from "../ListToolbar";
import { IconImage, IconLock, IconPencil, IconTrash } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import { FormDialog, IconButton, Section, Table } from "../ui";
import { AttributionSelects, CommissionBasisSelect } from "../AdminExtra";

const IDENTITY_TYPES: { id: string; label: string }[] = [
  { id: "ktp", label: "KTP" },
  { id: "sim", label: "SIM" },
  { id: "passport", label: "Paspor" },
  { id: "other", label: "Lainnya" },
];

export function CustomersPage({
  onOpenSecrets,
  onOpenGallery,
}: {
  onOpenSecrets: (customerId: string) => void;
  onOpenGallery: (customerId: string) => void;
}) {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  type CustomerRow = {
    id: string;
    customer_code: string;
    full_name: string;
    phone: string;
    email?: string | null;
    address?: string | null;
    latitude?: number | null;
    longitude?: number | null;
    identity_type?: string | null;
    identity_number?: string | null;
    is_active: boolean;
    portal_enabled?: boolean;
    cluster_id?: string | null;
    cluster_name?: string;
    cluster_code?: string;
    reseller_id?: string | null;
    reseller_name?: string;
    sales_user_id?: string | null;
    sales_user_name?: string;
  };
  type ClusterOpt = { id: string; name: string; code: string; customer_code_prefix: string };
  type CustForm = {
    full_name: string;
    phone: string;
    email: string;
    address: string;
    cluster_id: string;
    customer_code: string;
    latitude: string;
    longitude: string;
    identity_type: string;
    identity_number: string;
    is_active: boolean;
    portal_enabled: boolean;
    reseller_id: string;
    sales_user_id: string;
    commission_basis: string;
  };
  const emptyForm: CustForm = {
    full_name: "",
    phone: "",
    email: "",
    address: "",
    cluster_id: "",
    customer_code: "",
    latitude: "",
    longitude: "",
    identity_type: "ktp",
    identity_number: "",
    is_active: true,
    portal_enabled: true,
    reseller_id: "",
    sales_user_id: "",
    commission_basis: "new_customer_flat",
  };
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, 300);
  const [clusterFilter, setClusterFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [page, setPage] = useState(0);
  const limit = 25;
  const q = useQuery({
    queryKey: ["customers", debouncedSearch, clusterFilter, statusFilter, page],
    queryFn: () => {
      const params = new URLSearchParams({
        limit: String(limit),
        offset: String(page * limit),
      });
      if (debouncedSearch.trim()) params.set("search", debouncedSearch.trim());
      if (clusterFilter) params.set("cluster_id", clusterFilter);
      if (statusFilter) params.set("is_active", statusFilter);
      return api<{ data: CustomerRow[]; total: number }>(`/api/customers?${params}`);
    },
  });
  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterOpt[]>("/api/clusters"),
  });
  const resellersQ = useQuery({
    queryKey: ["resellers"],
    queryFn: () => api<{ id: string; name: string }[]>("/api/resellers"),
  });
  const usersQ = useQuery({
    queryKey: ["tenant-users"],
    queryFn: () => api<{ user_id: string; full_name: string; email: string; is_active: boolean }[]>("/api/users/options"),
  });
  const [form, setForm] = useState<CustForm>(emptyForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [formErr, setFormErr] = useState("");

  const previewQ = useQuery({
    queryKey: ["cluster-preview", form.cluster_id],
    queryFn: () => api<{ preview: string }>(`/api/clusters/${form.cluster_id}/next-customer-code`),
    enabled: createOpen && !editId && Boolean(form.cluster_id) && !form.customer_code.trim(),
  });

  const buildBody = () => {
    const body: Record<string, unknown> = {
      full_name: form.full_name,
      phone: form.phone,
      is_active: form.is_active,
      portal_enabled: form.portal_enabled,
    };
    if (form.email.trim()) body.email = form.email.trim();
    else body.email = null;
    if (form.address.trim()) body.address = form.address.trim();
    else body.address = null;
    if (form.cluster_id) body.cluster_id = form.cluster_id;
    else body.cluster_id = null;
    if (form.customer_code.trim()) body.customer_code = form.customer_code.trim();
    if (form.latitude.trim()) body.latitude = Number(form.latitude);
    else body.latitude = null;
    if (form.longitude.trim()) body.longitude = Number(form.longitude);
    else body.longitude = null;
    if (form.identity_type) body.identity_type = form.identity_type;
    else body.identity_type = null;
    if (form.identity_number.trim()) body.identity_number = form.identity_number.trim();
    else body.identity_number = null;
    if (form.reseller_id) body.reseller_id = form.reseller_id;
    else body.reseller_id = null;
    if (form.sales_user_id) body.sales_user_id = form.sales_user_id;
    else body.sales_user_id = null;
    if (!editId && form.commission_basis) body.commission_basis = form.commission_basis;
    return body;
  };

  const create = useMutation({
    mutationFn: () => api("/api/customers", { method: "POST", body: JSON.stringify(buildBody()) }),
    onSuccess: () => {
      setForm({ ...emptyForm, cluster_id: form.cluster_id });
      setCreateOpen(false);
      setFormErr("");
      qc.invalidateQueries({ queryKey: ["customers"] });
      qc.invalidateQueries({ queryKey: ["ftth-map"] });
      qc.invalidateQueries({ queryKey: ["cluster-preview"] });
      qc.invalidateQueries({ queryKey: ["commissions"] });
      qc.invalidateQueries({ queryKey: ["resellers"] });
      void toastSuccess("Pelanggan ditambahkan");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const update = useMutation({
    mutationFn: () => {
      const body = buildBody();
      if (!body.customer_code) throw new Error("kode pelanggan wajib saat edit");
      return api(`/api/customers/${editId}`, { method: "PUT", body: JSON.stringify(body) });
    },
    onSuccess: () => {
      setEditId(null);
      setForm(emptyForm);
      setFormErr("");
      qc.invalidateQueries({ queryKey: ["customers"] });
      qc.invalidateQueries({ queryKey: ["ftth-map"] });
      void toastSuccess("Pelanggan diperbarui");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/customers/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      if (editId === remove.variables) {
        setEditId(null);
        setForm(emptyForm);
      }
      qc.invalidateQueries({ queryKey: ["customers"] });
      qc.invalidateQueries({ queryKey: ["ftth-map"] });
      void toastSuccess("Pelanggan dihapus");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  function startEdit(c: CustomerRow) {
    setCreateOpen(false);
    setEditId(c.id);
    setFormErr("");
    setForm({
      full_name: c.full_name,
      phone: c.phone,
      email: c.email || "",
      address: c.address || "",
      cluster_id: c.cluster_id || "",
      customer_code: c.customer_code,
      latitude: c.latitude != null ? String(c.latitude) : "",
      longitude: c.longitude != null ? String(c.longitude) : "",
      identity_type: c.identity_type || "ktp",
      identity_number: c.identity_number || "",
      is_active: c.is_active,
      portal_enabled: c.portal_enabled ?? true,
      reseller_id: c.reseller_id || "",
      sales_user_id: c.sales_user_id || "",
      commission_basis: "new_customer_flat",
    });
  }

  function closeForm() {
    setEditId(null);
    setCreateOpen(false);
    setForm(emptyForm);
    setFormErr("");
  }

  const clusters = Array.isArray(clustersQ.data) ? clustersQ.data : [];
  const resellers = Array.isArray(resellersQ.data) ? resellersQ.data : [];
  const users = Array.isArray(usersQ.data) ? usersQ.data : [];
  const dialogOpen = createOpen || Boolean(editId);
  const saving = create.isPending || update.isPending;
  const rows = q.data?.data ?? [];
  const total = q.data?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / limit));

  return (
    <Section
      title="Pelanggan"
      actions={
        <button
          type="button"
          className="btn"
          onClick={() => {
            setEditId(null);
            setForm(emptyForm);
            setFormErr("");
            setCreateOpen(true);
          }}
        >
          + Tambah
        </button>
      }
    >
      {formErr && !dialogOpen && <p className="mb-3 text-sm text-[var(--danger)]">{formErr}</p>}
      <ListToolbar
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(0);
        }}
        searchPlaceholder="Nama, kode, telepon, email…"
        filters={[
          {
            key: "cluster",
            label: "Cluster",
            value: clusterFilter,
            onChange: (v) => {
              setClusterFilter(v);
              setPage(0);
            },
            options: clusters.map((c) => ({ value: c.id, label: `${c.name} (${c.code})` })),
            className: "min-w-[200px]",
          },
          {
            key: "status",
            label: "Status",
            value: statusFilter,
            onChange: (v) => {
              setStatusFilter(v);
              setPage(0);
            },
            options: [
              { value: "true", label: "Aktif" },
              { value: "false", label: "Nonaktif" },
            ],
          },
        ]}
        page={page}
        pageCount={pageCount}
        onPageChange={setPage}
        total={total}
      />
      <Table
        columns={["Kode", "Cluster", "Nama", "Telepon", "Atribusi", "Status", "Aksi"]}
        rows={rows.map((c) => [
          c.customer_code,
          c.cluster_name || c.cluster_code || "—",
          c.full_name,
          c.phone,
          c.reseller_name ? `Reseller: ${c.reseller_name}` : c.sales_user_name ? `Sales: ${c.sales_user_name}` : "—",
          c.is_active ? "aktif" : "nonaktif",
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton label="Dokumentasi / galeri" onClick={() => onOpenGallery(c.id)}>
              <IconImage />
            </IconButton>
            <IconButton label="Secrets pelanggan" onClick={() => onOpenSecrets(c.id)}>
              <IconLock />
            </IconButton>
            <IconButton label="Edit pelanggan" onClick={() => startEdit(c)}>
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus pelanggan"
              danger
              disabled={remove.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus pelanggan",
                  description: `Hapus pelanggan "${c.full_name}"?`,
                  confirmLabel: "Hapus",
                });
                if (!ok) return;
                remove.mutate(c.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />

      <FormDialog open={dialogOpen} wide title={editId ? "Edit pelanggan" : "Tambah pelanggan"} onClose={closeForm}>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (editId) update.mutate();
            else create.mutate();
          }}
        >
          <select
            className="input sm:col-span-2"
            value={form.cluster_id}
            onChange={(e) => setForm({ ...form, cluster_id: e.target.value })}
          >
            <option value="">— Cluster / POP (opsional) —</option>
            {clusters.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name} ({c.customer_code_prefix || c.code})
              </option>
            ))}
          </select>
          <input className="input" placeholder="Nama" value={form.full_name} onChange={(e) => setForm({ ...form, full_name: e.target.value })} required />
          <input className="input" placeholder="Telepon" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} required />
          <input className="input" placeholder="Email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
          <input
            className="input"
            placeholder={previewQ.data?.preview ? `Kode (kosong = ${previewQ.data.preview})` : "Kode pelanggan"}
            value={form.customer_code}
            onChange={(e) => setForm({ ...form, customer_code: e.target.value })}
            required={Boolean(editId)}
          />
          <input className="input sm:col-span-2" placeholder="Alamat" value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} />
          <select
            className="input"
            value={form.identity_type}
            onChange={(e) => setForm({ ...form, identity_type: e.target.value })}
          >
            {IDENTITY_TYPES.map((t) => (
              <option key={t.id} value={t.id}>
                Jenis identitas: {t.label}
              </option>
            ))}
          </select>
          <input
            className="input"
            placeholder="Nomor identitas"
            value={form.identity_number}
            onChange={(e) => setForm({ ...form, identity_number: e.target.value })}
          />
          <input className="input" type="number" step="any" placeholder="Latitude (peta)" value={form.latitude} onChange={(e) => setForm({ ...form, latitude: e.target.value })} />
          <input className="input" type="number" step="any" placeholder="Longitude (peta)" value={form.longitude} onChange={(e) => setForm({ ...form, longitude: e.target.value })} />
          <AttributionSelects
            resellerId={form.reseller_id}
            salesUserId={form.sales_user_id}
            resellers={resellers}
            users={users}
            onChange={(next) => setForm({ ...form, reseller_id: next.reseller_id, sales_user_id: next.sales_user_id })}
          />
          {!editId ? (
            <CommissionBasisSelect
              value={form.commission_basis}
              onChange={(v) => setForm({ ...form, commission_basis: v })}
            />
          ) : null}
          {form.cluster_id && !form.customer_code.trim() && !editId && previewQ.data?.preview && (
            <p className="text-xs text-[var(--muted)] sm:col-span-2">
              Kode otomatis: <span className="font-semibold text-[var(--text)]">{previewQ.data.preview}</span>
            </p>
          )}
          {editId && (
            <>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={form.is_active} onChange={(e) => setForm({ ...form, is_active: e.target.checked })} />
                Aktif
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={form.portal_enabled} onChange={(e) => setForm({ ...form, portal_enabled: e.target.checked })} />
                Portal aktif
              </label>
              <p className="text-xs text-[var(--muted)] sm:col-span-2">Password portal default = nomor HP (ikut berubah jika HP diubah).</p>
            </>
          )}
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <button className="btn" disabled={saving}>
              {saving ? "Menyimpan…" : "Simpan"}
            </button>
            <button type="button" className="btn-ghost" onClick={closeForm}>
              Batal
            </button>
          </div>
          {formErr && <p className="text-sm text-[var(--danger)] sm:col-span-2">{formErr}</p>}
        </form>
      </FormDialog>
    </Section>
  );
}

