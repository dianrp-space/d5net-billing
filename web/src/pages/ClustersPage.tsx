import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { ListToolbar, matchesQuery } from "../ListToolbar";
import { IconPencil, IconTrash } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import { FormDialog, IconButton, Section, Table } from "../ui";

export function ClustersPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  type ClusterRow = {
    id: string;
    name: string;
    code: string;
    customer_code_prefix: string;
    customer_code_pattern: string;
    seq_width: number;
    address?: string | null;
    latitude?: number | null;
    longitude?: number | null;
    coverage_radius_km?: number | null;
    notes?: string | null;
    is_active: boolean;
  };
  type ClusterForm = {
    name: string;
    code: string;
    customer_code_prefix: string;
    customer_code_pattern: string;
    seq_width: number;
    address: string;
    latitude: string;
    longitude: string;
    coverage_radius_km: string;
    notes: string;
    is_active: boolean;
  };
  const emptyForm: ClusterForm = {
    name: "",
    code: "",
    customer_code_prefix: "",
    customer_code_pattern: "{prefix}-{yyyymm}{seq}",
    seq_width: 4,
    address: "",
    latitude: "",
    longitude: "",
    coverage_radius_km: "",
    notes: "",
    is_active: true,
  };
  const q = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterRow[]>("/api/clusters"),
  });
  const [form, setForm] = useState<ClusterForm>(emptyForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [formErr, setFormErr] = useState("");

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["clusters"] });
    qc.invalidateQueries({ queryKey: ["ftth-map"] });
  };

  function coordsBody() {
    const km = form.coverage_radius_km.trim() ? Number(form.coverage_radius_km) : null;
    return {
      latitude: form.latitude.trim() ? Number(form.latitude) : null,
      longitude: form.longitude.trim() ? Number(form.longitude) : null,
      coverage_radius_km: km && km > 0 ? km : null,
    };
  }

  const create = useMutation({
    mutationFn: () =>
      api("/api/clusters", {
        method: "POST",
        body: JSON.stringify({
          name: form.name,
          code: form.code,
          customer_code_prefix: form.customer_code_prefix || undefined,
          customer_code_pattern: form.customer_code_pattern || undefined,
          seq_width: form.seq_width,
          address: form.address.trim() || undefined,
          notes: form.notes.trim() || undefined,
          ...coordsBody(),
        }),
      }),
    onSuccess: () => {
      setForm(emptyForm);
      setFormErr("");
      setCreateOpen(false);
      refresh();
      void toastSuccess("Cluster ditambahkan");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const update = useMutation({
    mutationFn: () =>
      api(`/api/clusters/${editId}`, {
        method: "PUT",
        body: JSON.stringify({
          name: form.name,
          code: form.code,
          customer_code_prefix: form.customer_code_prefix,
          customer_code_pattern: form.customer_code_pattern,
          seq_width: form.seq_width,
          address: form.address.trim() || null,
          notes: form.notes.trim() || null,
          is_active: form.is_active,
          ...coordsBody(),
        }),
      }),
    onSuccess: () => {
      setEditId(null);
      setForm(emptyForm);
      setFormErr("");
      setCreateOpen(false);
      refresh();
      void toastSuccess("Cluster diperbarui");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/clusters/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      if (editId === remove.variables) {
        setEditId(null);
        setForm(emptyForm);
      }
      refresh();
      void toastSuccess("Cluster dihapus");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  function startEdit(c: ClusterRow) {
    setCreateOpen(false);
    setEditId(c.id);
    setFormErr("");
    setForm({
      name: c.name,
      code: c.code,
      customer_code_prefix: c.customer_code_prefix,
      customer_code_pattern: c.customer_code_pattern || "{prefix}-{yyyymm}{seq}",
      seq_width: c.seq_width || 4,
      address: c.address || "",
      latitude: c.latitude != null ? String(c.latitude) : "",
      longitude: c.longitude != null ? String(c.longitude) : "",
      coverage_radius_km: c.coverage_radius_km != null && c.coverage_radius_km > 0 ? String(c.coverage_radius_km) : "",
      notes: c.notes || "",
      is_active: c.is_active,
    });
  }

  function closeForm() {
    setEditId(null);
    setCreateOpen(false);
    setForm(emptyForm);
    setFormErr("");
  }

  const list = Array.isArray(q.data) ? q.data : [];
  const [search, setSearch] = useState("");
  const filtered = useMemo(
    () => list.filter((c) => matchesQuery(search, c.name, c.code, c.customer_code_prefix, c.address, c.notes)),
    [list, search],
  );
  const saving = create.isPending || update.isPending;
  const dialogOpen = createOpen || Boolean(editId);
  const examplePrefix = (form.customer_code_prefix || form.code || "D5N").toUpperCase().replace(/[^A-Z0-9]/g, "") || "D5N";
  const yyyymm = new Date().toISOString().slice(0, 7).replace("-", "");
  const exampleCode = (form.customer_code_pattern || "{prefix}-{yyyymm}{seq}")
    .replace("{prefix}", examplePrefix)
    .replace("{yyyymm}", yyyymm)
    .replace("{yyyy}", yyyymm.slice(0, 4))
    .replace("{yy}", yyyymm.slice(2, 4))
    .replace("{mm}", yyyymm.slice(4, 6))
    .replace("{seq}", String(1).padStart(form.seq_width || 4, "0"));

  return (
    <Section
      title="Cluster / POP"
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
      <p className="mb-4 text-sm text-[var(--muted)]">
        Bisa punya banyak POP/cluster. Isi lat/long agar muncul di peta, dan radius coverage (km) untuk
        cek jangkauan sales di menu Coverage.
        Placeholder kode: <code className="text-xs">{"{prefix}"}</code>, <code className="text-xs">{"{yyyymm}"}</code>,{" "}
        <code className="text-xs">{"{seq}"}</code>.
      </p>

      <ListToolbar
        search={search}
        onSearchChange={setSearch}
        searchPlaceholder="Nama, kode, prefix…"
        total={filtered.length}
      />
      <Table
        columns={["Nama", "Kode", "Prefix", "Koordinat", "Coverage", "Status", "Aksi"]}
        rows={filtered.map((c) => [
          c.name,
          c.code,
          c.customer_code_prefix,
          c.latitude != null && c.longitude != null ? `${c.latitude.toFixed(5)}, ${c.longitude.toFixed(5)}` : "—",
          c.coverage_radius_km ? `${c.coverage_radius_km} km` : "—",
          c.is_active ? "aktif" : "nonaktif",
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton label="Edit cluster" onClick={() => startEdit(c)}>
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus cluster"
              danger
              disabled={remove.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus cluster",
                  description: `Hapus cluster "${c.name}"? Router/pelanggan tetap ada (cluster dikosongkan).`,
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

      <FormDialog open={dialogOpen} wide title={editId ? "Edit cluster / POP" : "Tambah cluster / POP"} onClose={closeForm}>
        <p className="mb-3 text-xs text-[var(--muted)]">
          Contoh kode: <span className="font-semibold text-[var(--text)]">{exampleCode}</span>
        </p>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (editId) update.mutate();
            else create.mutate();
          }}
        >
          <input
            className="input"
            placeholder="Nama (mis. Delima)"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            required
          />
          <input
            className="input"
            placeholder="Kode cluster (mis. DLMA)"
            value={form.code}
            onChange={(e) => {
              const code = e.target.value.toUpperCase();
              setForm({
                ...form,
                code,
                customer_code_prefix: form.customer_code_prefix || code.replace(/[^A-Z0-9]/g, ""),
              });
            }}
            required
          />
          <input
            className="input"
            placeholder="Prefix kode pelanggan"
            value={form.customer_code_prefix}
            onChange={(e) => setForm({ ...form, customer_code_prefix: e.target.value.toUpperCase() })}
          />
          <input
            className="input"
            type="number"
            min={1}
            max={8}
            placeholder="Lebar nomor urut"
            value={form.seq_width}
            onChange={(e) => setForm({ ...form, seq_width: Number(e.target.value) || 4 })}
          />
          <input
            className="input sm:col-span-2"
            placeholder="Pola kode ({prefix}-{yyyymm}{seq})"
            value={form.customer_code_pattern}
            onChange={(e) => setForm({ ...form, customer_code_pattern: e.target.value })}
          />
          <input
            className="input sm:col-span-2"
            placeholder="Alamat (opsional)"
            value={form.address}
            onChange={(e) => setForm({ ...form, address: e.target.value })}
          />
          <input
            className="input"
            type="number"
            step="any"
            placeholder="Latitude POP (peta)"
            value={form.latitude}
            onChange={(e) => setForm({ ...form, latitude: e.target.value })}
          />
          <input
            className="input"
            type="number"
            step="any"
            placeholder="Longitude POP (peta)"
            value={form.longitude}
            onChange={(e) => setForm({ ...form, longitude: e.target.value })}
          />
          <input
            className="input"
            type="number"
            min={0}
            max={50}
            step={0.05}
            placeholder="Coverage (km)"
            value={form.coverage_radius_km}
            onChange={(e) => setForm({ ...form, coverage_radius_km: e.target.value })}
          />
          <p className="text-xs text-[var(--muted)] sm:col-span-2">
            Radius coverage untuk cek calon pelanggan di menu Coverage. Kosong = belum di-set.
          </p>
          <textarea
            className="input sm:col-span-2 min-h-[72px]"
            placeholder="Catatan"
            value={form.notes}
            onChange={(e) => setForm({ ...form, notes: e.target.value })}
          />
          {editId && (
            <label className="flex items-center gap-2 text-sm text-[var(--text-body)] sm:col-span-2">
              <input type="checkbox" checked={form.is_active} onChange={(e) => setForm({ ...form, is_active: e.target.checked })} />
              Cluster aktif
            </label>
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

