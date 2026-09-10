import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { ListToolbar, matchesQuery } from "../ListToolbar";
import { IconPencil, IconTrash, IconZap } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import { FormDialog, IconButton, OnlineBadge, Section, SecretInput, StatusDialog, Table } from "../ui";

export function RoutersPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  type RouterRow = {
    id: string;
    name: string;
    address: string;
    port: number;
    username: string;
    use_tls: boolean;
    provisioner: string;
    is_active: boolean;
    cluster_id?: string | null;
    last_seen_at?: string | null;
    last_error?: string | null;
  };
  type ClusterOpt = { id: string; name: string; code: string };
  type RouterForm = {
    name: string;
    address: string;
    port: number;
    username: string;
    password: string;
    provisioner: string;
    use_tls: boolean;
    is_active: boolean;
    cluster_id: string;
  };
  const emptyForm: RouterForm = {
    name: "",
    address: "",
    port: 8728,
    username: "admin",
    password: "",
    provisioner: "routeros",
    use_tls: false,
    is_active: true,
    cluster_id: "",
  };
  const q = useQuery({
    queryKey: ["routers"],
    queryFn: () => api<RouterRow[]>("/api/routers"),
  });
  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterOpt[]>("/api/clusters"),
  });
  const [form, setForm] = useState<RouterForm>(emptyForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [formErr, setFormErr] = useState("");
  const [testMsg, setTestMsg] = useState<Record<string, { ok: boolean; text: string }>>({});
  const [testingId, setTestingId] = useState<string | null>(null);
  const [popup, setPopup] = useState<{ ok: boolean; title: string; message: string } | null>(null);

  const refresh = () => qc.invalidateQueries({ queryKey: ["routers"] });
  const clusters = Array.isArray(clustersQ.data) ? clustersQ.data : [];
  const clusterName = (id?: string | null) => clusters.find((c) => c.id === id)?.name || "—";

  const create = useMutation({
    mutationFn: () =>
      api("/api/routers", {
        method: "POST",
        body: JSON.stringify({
          name: form.name,
          address: form.address,
          port: form.port,
          username: form.username,
          password: form.password,
          provisioner: form.provisioner,
          use_tls: form.use_tls,
          cluster_id: form.cluster_id || undefined,
        }),
      }),
    onSuccess: () => {
      setForm(emptyForm);
      setFormErr("");
      setCreateOpen(false);
      refresh();
      void toastSuccess("Router ditambahkan");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const update = useMutation({
    mutationFn: () =>
      api(`/api/routers/${editId}`, {
        method: "PUT",
        body: JSON.stringify({
          name: form.name,
          address: form.address,
          port: form.port,
          username: form.username,
          password: form.password,
          provisioner: form.provisioner,
          use_tls: form.use_tls,
          is_active: form.is_active,
          cluster_id: form.cluster_id || null,
        }),
      }),
    onSuccess: () => {
      setEditId(null);
      setForm(emptyForm);
      setFormErr("");
      setCreateOpen(false);
      refresh();
      void toastSuccess("Router diperbarui");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/routers/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      if (editId === remove.variables) {
        setEditId(null);
        setForm(emptyForm);
      }
      refresh();
      void toastSuccess("Router dihapus");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const testMut = useMutation({
    mutationFn: (id: string) =>
      api<{ success: boolean; message: string }>(`/api/routers/${id}/test`, { method: "POST" }),
    onMutate: (id) => {
      setTestingId(id);
      setTestMsg((m) => {
        const next = { ...m };
        delete next[id];
        return next;
      });
    },
    onSuccess: (res, id) => {
      const router = (Array.isArray(q.data) ? q.data : []).find((r) => r.id === id);
      const name = router?.name || `Router #${id}`;
      setTestMsg((m) => ({ ...m, [id]: { ok: res.success, text: res.message } }));
      setPopup({
        ok: res.success,
        title: res.success ? `${name} online` : `${name} offline`,
        message: res.message || (res.success ? "Koneksi RouterOS berhasil." : "Koneksi RouterOS gagal."),
      });
      refresh();
    },
    onError: (err, id) => {
      const router = (Array.isArray(q.data) ? q.data : []).find((r) => r.id === id);
      const name = router?.name || `Router #${id}`;
      const message = (err as Error).message;
      setTestMsg((m) => ({ ...m, [id]: { ok: false, text: message } }));
      setPopup({
        ok: false,
        title: `${name} offline`,
        message,
      });
    },
    onSettled: () => setTestingId(null),
  });

  function routerOnline(r: RouterRow) {
    const result = testMsg[r.id];
    if (result) return result.ok;
    if (r.last_error) return false;
    if (r.last_seen_at) return true;
    return false;
  }

  function startEdit(r: RouterRow) {
    setCreateOpen(false);
    setEditId(r.id);
    setFormErr("");
    setForm({
      name: r.name,
      address: r.address,
      port: r.port,
      username: r.username || "admin",
      password: "",
      provisioner: r.provisioner || "routeros",
      use_tls: r.use_tls,
      is_active: r.is_active,
      cluster_id: r.cluster_id || "",
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
  const [statusFilter, setStatusFilter] = useState("");
  const [clusterFilter, setClusterFilter] = useState("");
  const filtered = useMemo(
    () =>
      list.filter((r) => {
        if (statusFilter === "true" && !r.is_active) return false;
        if (statusFilter === "false" && r.is_active) return false;
        if (clusterFilter === "__none__" && r.cluster_id) return false;
        if (clusterFilter && clusterFilter !== "__none__" && r.cluster_id !== clusterFilter) return false;
        return matchesQuery(search, r.name, r.address, r.username, r.provisioner);
      }),
    [list, search, statusFilter, clusterFilter],
  );
  const saving = create.isPending || update.isPending;
  const dialogOpen = createOpen || Boolean(editId);

  return (
    <Section
      title="Router MikroTik"
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
      <StatusDialog
        open={Boolean(popup)}
        ok={popup?.ok ?? false}
        title={popup?.title ?? ""}
        message={popup?.message ?? ""}
        onClose={() => setPopup(null)}
      />
      <p className="mb-4 text-sm text-[var(--muted)]">
        Alamat bisa IP atau domain. Port default API 8728, TLS 8729. Kosongkan password saat edit jika tidak ingin diubah.
      </p>

      <ListToolbar
        search={search}
        onSearchChange={setSearch}
        searchPlaceholder="Nama, IP, username…"
        filters={[
          {
            key: "cluster",
            label: "Cluster",
            value: clusterFilter,
            onChange: setClusterFilter,
            options: [
              { value: "__none__", label: "Tanpa cluster" },
              ...clusters.map((c) => ({ value: c.id, label: `${c.name} (${c.code})` })),
            ],
            className: "min-w-[200px]",
          },
          {
            key: "status",
            label: "Status",
            value: statusFilter,
            onChange: setStatusFilter,
            options: [
              { value: "true", label: "Aktif" },
              { value: "false", label: "Nonaktif" },
            ],
          },
        ]}
        total={filtered.length}
      />
      <Table
        columns={["Nama", "Cluster", "Alamat", "Port", "User", "TLS", "Status", "Last seen", "Aksi"]}
        rows={filtered.map((r) => {
          const online = routerOnline(r);
          const detail = testMsg[r.id]?.text || r.last_error || undefined;
          return [
            r.name,
            clusterName(r.cluster_id),
            r.address,
            r.port,
            r.username,
            r.use_tls ? "ya" : "tidak",
            <OnlineBadge key="st" online={online} title={detail} />,
            r.last_seen_at ? new Date(r.last_seen_at).toLocaleString("id-ID") : "—",
            <span key="act" className="flex flex-wrap items-center gap-1.5">
              <IconButton label="Test koneksi" disabled={testingId === r.id} onClick={() => testMut.mutate(r.id)}>
                <IconZap />
              </IconButton>
              <IconButton label="Edit router" onClick={() => startEdit(r)}>
                <IconPencil />
              </IconButton>
              <IconButton
                label="Hapus router"
                danger
                disabled={remove.isPending}
                onClick={async () => {
                  const ok = await confirm({
                    title: `Hapus router "${r.name}"?`,
                    description:
                      `Router "${r.name}" akan dihapus permanen.\n\n` +
                      `Router tidak bisa dihapus jika masih dipakai oleh langganan, IP pool, batch voucher, ` +
                      `atau sebagai router isolir — pindahkan/hapus data tersebut dulu.\n\n` +
                      `Metrik, log perintah, dan backup router ikut terhapus.`,
                    confirmLabel: "Hapus",
                    danger: true,
                  });
                  if (!ok) return;
                  remove.mutate(r.id);
                }}
              >
                <IconTrash />
              </IconButton>
            </span>,
          ];
        })}
      />
      {filtered.length > 0 && (
        <p className="mt-2 text-xs text-[var(--muted)]">
          Status ONLINE/OFFLINE dari hasil test koneksi (atau last seen / last error). Klik ikon petir untuk menguji.
        </p>
      )}

      <FormDialog open={dialogOpen} wide title={editId ? "Edit router" : "Tambah router"} onClose={closeForm}>
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
                {c.name} ({c.code})
              </option>
            ))}
          </select>
          <input
            className="input"
            placeholder="Nama"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            required
          />
          <input
            className="input"
            placeholder="host / domain"
            value={form.address}
            onChange={(e) => setForm({ ...form, address: e.target.value })}
            required
          />
          <input
            className="input"
            type="number"
            placeholder="Port"
            value={form.port}
            onChange={(e) => setForm({ ...form, port: Number(e.target.value) })}
            required
          />
          <input
            className="input"
            placeholder="Username"
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
            required
          />
          <SecretInput
            className="sm:col-span-2"
            placeholder={editId ? "Password baru (opsional)" : "Password"}
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            required={!editId}
            autoComplete="new-password"
          />
          <label className="flex items-center gap-2 text-sm text-[var(--text-body)]">
            <input type="checkbox" checked={form.use_tls} onChange={(e) => setForm({ ...form, use_tls: e.target.checked })} />
            Gunakan TLS
          </label>
          {editId ? (
            <label className="flex items-center gap-2 text-sm text-[var(--text-body)]">
              <input type="checkbox" checked={form.is_active} onChange={(e) => setForm({ ...form, is_active: e.target.checked })} />
              Router aktif
            </label>
          ) : (
            <span />
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

