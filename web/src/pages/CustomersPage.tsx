import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiDownload } from "../api";
import { ListToolbar, useDebouncedValue } from "../ListToolbar";
import { IconBanknote, IconDownload, IconImage, IconLock, IconPencil, IconTrash, IconUnplug, IconUpload } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import { Button, FormDialog, IconButton, Section, Table, formatRp } from "../ui";
import { AttributionSelects, CommissionBasisSelect } from "../AdminExtra";

const IDENTITY_TYPES: { id: string; label: string }[] = [
  { id: "ktp", label: "KTP" },
  { id: "sim", label: "SIM" },
  { id: "passport", label: "Paspor" },
  { id: "other", label: "Lainnya" },
];

const CUSTOMER_STATUS_LABEL: Record<string, string> = {
  active: "aktif",
  isolir: "isolir",
  overdue: "tunggakan",
  inactive: "nonaktif",
  dismantled: "cabut",
};

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
    service_status?: string;
    dismantled_at?: string | null;
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
    status: "active" | "isolir" | "inactive";
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
    status: "active",
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
  const [limit, setLimit] = useState(25);
  type CustomerWallet = {
    enabled: boolean;
    balance: number;
    min_topup: number;
    transactions: { id: string; amount: number; type: string; reference?: string; description?: string; created_at: string }[];
  };
  const [walletCustomer, setWalletCustomer] = useState<CustomerRow | null>(null);
  const [topupAmount, setTopupAmount] = useState("");
  const walletQ = useQuery({
    queryKey: ["customer-wallet", walletCustomer?.id],
    queryFn: () => api<CustomerWallet>(`/api/customers/${walletCustomer!.id}/wallet`),
    enabled: Boolean(walletCustomer?.id),
  });
  const featuresQ = useQuery({
    queryKey: ["features"],
    queryFn: () => api<{ wallet_enabled: boolean; wallet_min_topup: number }>("/api/features"),
  });
  const topupWallet = useMutation({
    mutationFn: () =>
      api(`/api/customers/${walletCustomer!.id}/wallet/topup`, {
        method: "POST",
        body: JSON.stringify({ amount: Math.floor(Number(topupAmount) || 0) }),
      }),
    onSuccess: () => {
      setTopupAmount("");
      void qc.invalidateQueries({ queryKey: ["customer-wallet", walletCustomer?.id] });
      void toastSuccess("Saldo ditambahkan");
    },
    onError: (e: Error) => void toastError(e.message),
  });
  function setPageSize(n: number) {
    setLimit(n);
    setPage(0);
  }
  const q = useQuery({
    queryKey: ["customers", debouncedSearch, clusterFilter, statusFilter, page, limit],
    queryFn: () => {
      const params = new URLSearchParams({
        limit: String(limit),
        offset: String(page * limit),
      });
      if (debouncedSearch.trim()) params.set("search", debouncedSearch.trim());
      if (clusterFilter) params.set("cluster_id", clusterFilter);
      if (statusFilter) params.set("status", statusFilter);
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
  const [importOpen, setImportOpen] = useState(false);
  const [importMsg, setImportMsg] = useState("");
  const [importBusy, setImportBusy] = useState(false);

  const previewQ = useQuery({
    queryKey: ["cluster-preview", form.cluster_id],
    queryFn: () => api<{ preview: string }>(`/api/clusters/${form.cluster_id}/next-customer-code`),
    enabled: createOpen && !editId && Boolean(form.cluster_id) && !form.customer_code.trim(),
  });

  const buildBody = () => {
    const body: Record<string, unknown> = {
      full_name: form.full_name,
      phone: form.phone,
      is_active: form.status !== "inactive",
      status: form.status,
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

  const cabut = useMutation({
    mutationFn: (id: string) => api(`/api/customers/${id}/dismantle`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["customers"] });
      qc.invalidateQueries({ queryKey: ["subs"] });
      qc.invalidateQueries({ queryKey: ["ip-pools"] });
      qc.invalidateQueries({ queryKey: ["churn"] });
      qc.invalidateQueries({ queryKey: ["ftth-map"] });
      void toastSuccess("Pelanggan dicabut");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  function isCabut(c: Pick<CustomerRow, "service_status" | "dismantled_at">) {
    return c.service_status === "dismantled" || Boolean(c.dismantled_at);
  }

  function statusLabel(c: CustomerRow) {
    if (isCabut(c)) return "cabut";
    return CUSTOMER_STATUS_LABEL[c.service_status || ""] || (c.is_active ? "aktif" : "nonaktif");
  }

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
      status:
        c.service_status === "isolir"
          ? "isolir"
          : c.service_status === "inactive"
            ? "inactive"
            : "active",
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

  async function exportCsv() {
    const ok = await confirm({
      title: "Export pelanggan",
      description: "Unduh data pelanggan sebagai CSV?",
      confirmLabel: "Unduh",
    });
    if (!ok) return;
    try {
      const params = new URLSearchParams();
      if (clusterFilter) params.set("cluster_id", clusterFilter);
      if (statusFilter) params.set("status", statusFilter);
      const qs = params.toString();
      await apiDownload(`/api/customers/export.csv${qs ? `?${qs}` : ""}`, "customers.csv");
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Export gagal");
    }
  }

  async function downloadTemplate() {
    const ok = await confirm({
      title: "Unduh template import",
      description: "Unduh template CSV untuk import pelanggan?",
      confirmLabel: "Unduh",
    });
    if (!ok) return;
    const sample = [
      "customer_code,full_name,phone,email,address,cluster_code,latitude,longitude,identity_type,identity_number,is_active,portal_enabled",
      ",Budi Santoso,081234567890,budi@example.com,Jl. Merdeka No. 10,,-6.2,106.81667,ktp,3201234567890123,true,true",
      ",Siti Aminah,081987654321,,,,,,sim,,true,true",
    ].join("\n");
    const blob = new Blob([sample + "\n"], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "customers-template.csv";
    a.click();
    URL.revokeObjectURL(url);
  }

  async function onImportFile(file: File) {
    setImportBusy(true);
    setImportMsg("");
    try {
      const csv = await file.text();
      const res = await api<{ created: number; updated: number; skipped: number; errors?: string[] }>(
        "/api/customers/import",
        { method: "POST", body: JSON.stringify({ csv }) },
      );
      const errHint = res.errors?.length ? ` · ${res.errors.slice(0, 3).join("; ")}` : "";
      setImportMsg(`Import selesai: ${res.created} baru, ${res.updated} diupdate, ${res.skipped} dilewati${errHint}`);
      qc.invalidateQueries({ queryKey: ["customers"] });
      qc.invalidateQueries({ queryKey: ["ftth-map"] });
    } catch (e: unknown) {
      setImportMsg(e instanceof Error ? e.message : "Import gagal");
    } finally {
      setImportBusy(false);
    }
  }

  const clusters = Array.isArray(clustersQ.data) ? clustersQ.data : [];
  const resellers = Array.isArray(resellersQ.data) ? resellersQ.data : [];
  const users = Array.isArray(usersQ.data) ? usersQ.data : [];
  const dialogOpen = createOpen || Boolean(editId);
  const saving = create.isPending || update.isPending;
  const rows = q.data?.data ?? [];
  const total = q.data?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / limit));

  // Batch select: ubah status aktif, cabut, hapus permanen.
  const [selected, setSelected] = useState<string[]>([]);
  useEffect(() => {
    setSelected([]);
  }, [debouncedSearch, clusterFilter, statusFilter, page]);
  const isSelected = (id: string) => selected.includes(id);
  function toggleSelect(id: string) {
    setSelected((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }
  function toggleSelectPage() {
    setSelected((prev) => {
      const pageIds = rows.map((r) => r.id);
      const allIn = pageIds.length > 0 && pageIds.every((id) => prev.includes(id));
      if (allIn) return prev.filter((id) => !pageIds.includes(id));
      return Array.from(new Set([...prev, ...pageIds]));
    });
  }
  const [batchBusy, setBatchBusy] = useState("");

  function refreshCustomers() {
    qc.invalidateQueries({ queryKey: ["customers"] });
    qc.invalidateQueries({ queryKey: ["ftth-map"] });
  }

  async function runCustomerStatus(isActive: boolean) {
    const label = isActive ? "Aktifkan" : "Nonaktifkan";
    const ok = await confirm({
      title: `${label} ${selected.length} pelanggan?`,
      description: isActive
        ? "Pelanggan yang dipilih menjadi aktif (kecuali yang sudah cabut)."
        : "Pelanggan yang dipilih menjadi nonaktif.",
      confirmLabel: label,
    });
    if (!ok) return;
    setBatchBusy("status");
    try {
      const res = await api<{ updated: number; skipped_dismantled: number; failed: number }>(
        "/api/customers/batch-status",
        { method: "POST", body: JSON.stringify({ ids: selected, is_active: isActive }) },
      );
      setSelected([]);
      refreshCustomers();
      void toastSuccess(
        `${label}: ${res.updated} berhasil` +
          (res.skipped_dismantled > 0 ? `, ${res.skipped_dismantled} dilewati (sudah cabut)` : "") +
          (res.failed > 0 ? `, ${res.failed} gagal` : ""),
      );
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Gagal ubah status");
    } finally {
      setBatchBusy("");
    }
  }

  async function runCustomerBatch(kind: "cabut" | "delete") {
    let ids = selected;
    if (kind === "cabut") {
      ids = selected.filter((id) => {
        const r = rows.find((x) => x.id === id);
        return r && !isCabut(r);
      });
      if (ids.length === 0) {
        void toastError("Tidak ada pelanggan yang bisa dicabut dalam pilihan");
        return;
      }
    }
    const label = kind === "cabut" ? "Cabut" : "Hapus permanen";
    const ok = await confirm({
      title: `${label} ${ids.length} pelanggan?`,
      description:
        kind === "cabut"
          ? "Secret di router dilepas, port ODP dikosongkan, status menjadi cabut. Data tetap tersimpan."
          : "PERMANEN: data pelanggan, langganan, tagihan, dan pembayaran ikut terhapus. Tidak bisa dikembalikan.",
      confirmLabel: label,
      ...(kind === "delete" ? { danger: true } : {}),
    });
    if (!ok) return;
    setBatchBusy(kind);
    let done = 0;
    let firstErr = "";
    for (const id of ids) {
      try {
        if (kind === "cabut") {
          await api(`/api/customers/${id}/dismantle`, { method: "POST" });
        } else {
          await api(`/api/customers/${id}`, { method: "DELETE" });
        }
        done++;
      } catch (e: unknown) {
        if (!firstErr) firstErr = e instanceof Error ? e.message : "gagal";
      }
    }
    setBatchBusy("");
    setSelected([]);
    refreshCustomers();
    if (done === ids.length) {
      void toastSuccess(`${label}: ${done} berhasil`);
    } else {
      void toastError(`${label}: ${done}/${ids.length} berhasil${firstErr ? ` — ${firstErr}` : ""}`);
    }
  }

  return (
    <Section
      title="Pelanggan"
      actions={
        <span className="flex flex-wrap items-center gap-1.5">
          <IconButton label="Export CSV" onClick={() => void exportCsv()}>
            <IconDownload />
          </IconButton>
          <IconButton label="Import CSV" onClick={() => { setImportMsg(""); setImportOpen(true); }}>
            <IconUpload />
          </IconButton>
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
        </span>
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
              { value: "active", label: "Aktif" },
              { value: "isolir", label: "Isolir" },
              { value: "overdue", label: "Tunggakan" },
              { value: "inactive", label: "Nonaktif" },
              { value: "dismantled", label: "Cabut" },
            ],
          },
        ]}
        page={page}
        pageCount={pageCount}
        onPageChange={setPage}
        total={total}
        pageSize={limit}
        onPageSizeChange={setPageSize}
      >
        <label className="flex cursor-pointer items-center gap-1.5 text-sm text-[var(--muted)]">
          <input
            type="checkbox"
            checked={rows.length > 0 && rows.every((r) => selected.includes(r.id))}
            onChange={toggleSelectPage}
            title="Pilih semua di halaman ini"
          />
          Pilih halaman
        </label>
      </ListToolbar>
      {selected.length > 0 ? (
        <div className="mb-3 flex flex-wrap items-center gap-2 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm">
          <strong>{selected.length} dipilih</strong>
          <Button type="button" size="sm" disabled={batchBusy !== ""} onClick={() => void runCustomerStatus(true)}>
            {batchBusy === "status" ? "Memproses…" : "Aktifkan"}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={batchBusy !== ""}
            onClick={() => void runCustomerStatus(false)}
          >
            Nonaktifkan
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={batchBusy !== ""}
            onClick={() => void runCustomerBatch("cabut")}
          >
            {batchBusy === "cabut" ? "Memproses…" : "Cabut"}
          </Button>
          <Button
            type="button"
            variant="destructive"
            size="sm"
            disabled={batchBusy !== ""}
            onClick={() => void runCustomerBatch("delete")}
          >
            {batchBusy === "delete" ? "Memproses…" : "Hapus"}
          </Button>
          <Button type="button" variant="ghost" size="sm" disabled={batchBusy !== ""} onClick={() => setSelected([])}>
            Batal
          </Button>
        </div>
      ) : null}
      <Table
        rowNumberStart={page * limit + 1}
        columns={["", "Kode", "Cluster", "Nama", "Telepon", "Atribusi", "Status", "Aksi"]}
        rows={rows.map((c) => [
          <input
            key={`sel-${c.id}`}
            type="checkbox"
            checked={isSelected(c.id)}
            onChange={() => toggleSelect(c.id)}
            title={`Pilih ${c.customer_code}`}
          />,
          c.customer_code,
          c.cluster_name || c.cluster_code || "—",
          c.full_name,
          c.phone,
          c.reseller_name ? `Reseller: ${c.reseller_name}` : c.sales_user_name ? `Sales: ${c.sales_user_name}` : "—",
          statusLabel(c),
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            {featuresQ.data?.wallet_enabled ? (
              <IconButton label="Saldo pelanggan" onClick={() => setWalletCustomer(c)}>
                <IconBanknote />
              </IconButton>
            ) : null}
            <IconButton label="Dokumentasi / galeri" onClick={() => onOpenGallery(c.id)}>
              <IconImage />
            </IconButton>
            <IconButton label="Secrets pelanggan" onClick={() => onOpenSecrets(c.id)}>
              <IconLock />
            </IconButton>
            <IconButton label="Edit pelanggan" onClick={() => startEdit(c)}>
              <IconPencil />
            </IconButton>
            {!isCabut(c) ? (
              <IconButton
                label="Cabut pelanggan"
                danger
                disabled={cabut.isPending}
                onClick={async () => {
                  const ok = await confirm({
                    title: "Cabut pelanggan",
                    description: `Cabut "${c.full_name}"? Secret di router dilepas, port ODP dikosongkan, dan status menjadi cabut. Data pelanggan tetap tersimpan.`,
                    confirmLabel: "Cabut",
                  });
                  if (!ok) return;
                  cabut.mutate(c.id);
                }}
              >
                <IconUnplug />
              </IconButton>
            ) : null}
            <IconButton
              label="Hapus pelanggan"
              danger
              disabled={remove.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: `Hapus permanen "${c.full_name}"?`,
                  description:
                    `${c.customer_code} — data pelanggan, langganan, tagihan, dan pembayaran akan DIHAPUS PERMANEN.\n\n` +
                    `Sistem otomatis mencabut layanan dulu:\n` +
                    `• Secret PPPoE/hotspot dilepas dari router\n` +
                    `• Port ODP dikosongkan & IP assignment dibersihkan\n` +
                    `• Semua langganan dibatalkan\n\n` +
                    `Tindakan ini tidak bisa dikembalikan.`,
                  confirmLabel: "Hapus permanen",
                  cancelLabel: "Batal",
                  danger: true,
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

      <FormDialog open={importOpen} title="Import Pelanggan (CSV)" onClose={() => setImportOpen(false)}>
        <div className="grid gap-3">
          <p className="text-sm text-[var(--muted)]">
            Kolom:{" "}
            <code className="text-xs break-all">
              customer_code,full_name,phone,email,address,cluster_code,latitude,longitude,identity_type,identity_number,is_active,portal_enabled
            </code>
            . Upsert berdasarkan <code className="text-xs">customer_code</code>; kode kosong = dibuat otomatis.
            Password portal default = nomor HP.
          </p>
          <div className="flex flex-wrap gap-2">
            <button type="button" className="btn-ghost" onClick={() => void downloadTemplate()}>
              Unduh template
            </button>
            <label className="btn cursor-pointer">
              {importBusy ? "Mengimpor…" : "Pilih file CSV"}
              <input
                type="file"
                accept=".csv,text/csv"
                className="hidden"
                disabled={importBusy}
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  e.target.value = "";
                  if (f) void onImportFile(f);
                }}
              />
            </label>
            <button type="button" className="btn-ghost" onClick={() => setImportOpen(false)}>
              Tutup
            </button>
          </div>
          {importMsg && <p className="text-sm text-[var(--text-body)] whitespace-pre-wrap break-words">{importMsg}</p>}
        </div>
      </FormDialog>

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
          {editId && rows.find((c) => c.id === editId && isCabut(c)) ? (
            <p className="text-sm text-[var(--muted)] sm:col-span-2">
              Status: <strong>cabut</strong>. Layanan sudah dihentikan; data tetap tersimpan.
            </p>
          ) : (
            <>
              <div className="grid gap-1.5 sm:col-span-2">
                <label className="text-sm font-medium" htmlFor="cust-status">
                  Status pelanggan
                </label>
                <select
                  id="cust-status"
                  className="input"
                  value={form.status}
                  onChange={(e) => setForm({ ...form, status: e.target.value as CustForm["status"] })}
                >
                  <option value="active">Aktif</option>
                  <option value="isolir">Isolir</option>
                  <option value="inactive">Nonaktif</option>
                </select>
                <p className="text-xs text-[var(--muted)]">
                  Isolir menaruh semua langganan pelanggan ke profil isolir; Aktif membuka kembali isolir; Nonaktif
                  menonaktifkan pelanggan. Status ini mengikuti status langganan sebenarnya.
                  {!editId ? " Saat pelanggan baru dibuat, Isolir berlaku setelah pelanggan punya langganan." : ""}
                </p>
              </div>
              {editId ? (
                <label className="flex items-center gap-2 text-sm sm:col-span-2">
                  <input
                    type="checkbox"
                    checked={form.portal_enabled}
                    onChange={(e) => setForm({ ...form, portal_enabled: e.target.checked })}
                  />
                  Portal aktif
                </label>
              ) : null}
              {editId ? (
                <p className="text-xs text-[var(--muted)] sm:col-span-2">
                  Password portal default = nomor HP (ikut berubah jika HP diubah).
                </p>
              ) : null}
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

      <FormDialog
        open={Boolean(walletCustomer)}
        title={`Saldo · ${walletCustomer?.full_name || walletCustomer?.customer_code || ""}`}
        onClose={() => {
          setWalletCustomer(null);
          setTopupAmount("");
        }}
      >
        <div className="grid gap-4">
          <div>
            <p className="text-xs text-[var(--muted)]">Saldo saat ini</p>
            <p className="text-2xl font-bold">{formatRp(walletQ.data?.balance ?? 0)}</p>
            {walletQ.data && !walletQ.data.enabled ? (
              <p className="mt-1 text-xs text-[var(--warn,#b7791f)]">
                Fitur saldo sedang nonaktif (aktifkan di Pengaturan).
              </p>
            ) : null}
          </div>
          <div className="flex flex-wrap items-end gap-2">
            <label className="grid flex-1 gap-1 text-sm">
              <span className="font-medium">Tambah saldo (Rp)</span>
              <input
                className="input"
                type="number"
                min={1000}
                step={1000}
                value={topupAmount}
                onChange={(e) => setTopupAmount(e.target.value)}
              />
            </label>
            <Button
              type="button"
              disabled={topupWallet.isPending || !topupAmount}
              onClick={() => topupWallet.mutate()}
            >
              {topupWallet.isPending ? "Menyimpan…" : "Tambah saldo"}
            </Button>
          </div>
          <div>
            <p className="mb-2 text-sm font-medium">Riwayat saldo</p>
            <Table
              columns={["Tanggal", "Jenis", "Keterangan", "Nominal"]}
              hideRowNumber
              rows={(walletQ.data?.transactions ?? []).map((t) => [
                new Date(t.created_at).toLocaleString("id-ID"),
                t.type,
                t.description || t.reference || "—",
                <span
                  key={t.id}
                  style={{ color: t.amount < 0 ? "var(--danger)" : "var(--ok, #2b9a66)" }}
                >
                  {formatRp(t.amount)}
                </span>,
              ])}
            />
          </div>
        </div>
      </FormDialog>
    </Section>
  );
}

