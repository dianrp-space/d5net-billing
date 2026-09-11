import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiDownload, getToken } from "../api";
import { ListToolbar, matchesQuery, usePagination } from "../ListToolbar";
import { IconDownload, IconEye, IconPencil, IconTrash, IconUpload } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import { FormDialog, IconButton, Section, Table, Button } from "../ui";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { MapPin } from "lucide-react";
import { MapODP } from "../FtthMap";
import { getLastOdpCluster, setLastOdpCluster } from "../navPersist";
import type { AdminPage } from "../admin/pages";

export function OdpPage({ tenantSlug, onNavigate }: { tenantSlug?: string; onNavigate?: (page: AdminPage) => void }) {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  type ClusterTab = {
    id: string;
    name: string;
    code: string;
    is_active: boolean;
    latitude?: number | null;
    longitude?: number | null;
  };
  type OdpRow = {
    id: string;
    name: string;
    code: string;
    cluster_id?: string | null;
    port_count: number;
    used_ports: number;
    free_ports: number;
    latitude?: number | null;
    longitude?: number | null;
    coverage_radius_km?: number | null;
  };
  type OdpForm = {
    name: string;
    code: string;
    latitude: string;
    longitude: string;
    coverage_radius_km: string;
    port_count: number;
  };
  const emptyForm: OdpForm = { name: "", code: "", latitude: "", longitude: "", coverage_radius_km: "", port_count: 8 };

  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterTab[]>("/api/clusters"),
  });
  const q = useQuery({
    queryKey: ["odps"],
    queryFn: () => api<OdpRow[]>("/api/odps"),
  });

  const clusters = (Array.isArray(clustersQ.data) ? clustersQ.data : []).filter((c) => c.is_active);
  const list = Array.isArray(q.data) ? q.data : [];
  const unassignedCount = list.filter((o) => !o.cluster_id).length;

  const [tabId, setTabId] = useState<string>(() =>
    tenantSlug ? getLastOdpCluster(tenantSlug) || "" : "",
  );
  const [form, setForm] = useState<OdpForm>(emptyForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [formErr, setFormErr] = useState("");
  const [importOpen, setImportOpen] = useState(false);
  const [importMsg, setImportMsg] = useState("");
  const [importBusy, setImportBusy] = useState(false);
  const [portsOdpId, setPortsOdpId] = useState<string | null>(null);

  const portsQ = useQuery({
    queryKey: ["odp-ports", portsOdpId],
    queryFn: () =>
      api<{
        odp: { name: string; code: string; port_count: number; used_ports: number; free_ports: number };
        ports: {
          port_number: number;
          status: string;
          customer_name?: string;
          subscription_username?: string;
        }[];
      }>(`/api/odps/${portsOdpId}/ports`),
    enabled: Boolean(portsOdpId),
  });

  useEffect(() => {
    const saved = tenantSlug ? getLastOdpCluster(tenantSlug) : null;
    const savedOk =
      saved === "__none__"
        ? unassignedCount > 0
        : Boolean(saved && clusters.some((c) => c.id === saved));

    if (tabId) {
      const tabOk =
        tabId === "__none__"
          ? unassignedCount > 0
          : clusters.some((c) => c.id === tabId);
      if (tabOk) {
        if (tenantSlug) setLastOdpCluster(tenantSlug, tabId);
        return;
      }
    }

    if (savedOk && saved) {
      setTabId(saved);
      return;
    }
    if (clusters.length === 0) {
      if (unassignedCount > 0) setTabId("__none__");
      return;
    }
    const withCoords = clusters.find((c) => c.latitude != null && c.longitude != null);
    setTabId((withCoords || clusters[0]).id);
  }, [clusters, tabId, unassignedCount, tenantSlug]);

  function selectClusterTab(id: string) {
    setTabId(id);
    if (tenantSlug) setLastOdpCluster(tenantSlug, id);
  }

  const activeCluster = clusters.find((c) => c.id === tabId) || null;
  const clusterCenter =
    activeCluster?.latitude != null && activeCluster?.longitude != null
      ? { lat: activeCluster.latitude, lng: activeCluster.longitude }
      : null;

  const [odpSearch, setOdpSearch] = useState("");
  const filtered = list.filter((o) => {
    if (tabId === "__none__") {
      if (o.cluster_id) return false;
    } else if (tabId && o.cluster_id !== tabId) {
      return false;
    }
    return matchesQuery(odpSearch, o.name, o.code);
  });
  const { page: odpPage, setPage: setOdpPage, pageCount: odpPageCount, pageItems: odpPageItems } =
    usePagination(filtered, 25);

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["odps"] });
    qc.invalidateQueries({ queryKey: ["ftth-map"] });
  };

  async function exportCsv() {
    try {
      const qs =
        tabId && tabId !== "__none__" ? `?cluster_id=${encodeURIComponent(tabId)}` : "";
      const res = await fetch(`/api/odps/export.csv${qs}`, {
        credentials: "include",
        headers: {
          Authorization: getToken() ? `Bearer ${getToken()}` : "",
        },
      });
      if (!res.ok) throw new Error(await res.text());
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = activeCluster ? `odps-${activeCluster.code}.csv` : "odps.csv";
      a.click();
      URL.revokeObjectURL(url);
    } catch (e: unknown) {
      setFormErr(e instanceof Error ? e.message : "Export gagal");
    }
  }

  function downloadTemplate() {
    const sample = [
      "code,name,latitude,longitude,port_count,address,cluster_code",
      `ODP-01,ODP Contoh 1,${clusterCenter?.lat ?? -6.2},${clusterCenter?.lng ?? 106.8},8,Jl. Contoh,${activeCluster?.code ?? ""}`,
      `ODP-02,ODP Contoh 2,${clusterCenter ? clusterCenter.lat + 0.001 : -6.201},${clusterCenter ? clusterCenter.lng + 0.001 : 106.801},16,,${activeCluster?.code ?? ""}`,
    ].join("\n");
    const blob = new Blob([sample + "\n"], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "odps-template.csv";
    a.click();
    URL.revokeObjectURL(url);
  }

  async function onImportFile(file: File) {
    setImportBusy(true);
    setImportMsg("");
    try {
      const csv = await file.text();
      const body: Record<string, unknown> = { csv };
      if (tabId && tabId !== "__none__") body.cluster_id = tabId;
      const res = await api<{ created: number; updated: number; skipped: number; errors?: string[] }>(
        "/api/odps/import",
        { method: "POST", body: JSON.stringify(body) },
      );
      const errHint = res.errors?.length ? ` · ${res.errors.slice(0, 3).join("; ")}` : "";
      setImportMsg(`Import selesai: ${res.created} baru, ${res.updated} diupdate, ${res.skipped} dilewati${errHint}`);
      refresh();
    } catch (e: unknown) {
      setImportMsg(e instanceof Error ? e.message : "Import gagal");
    } finally {
      setImportBusy(false);
    }
  }

  const update = useMutation({
    mutationFn: () => {
      const body: Record<string, unknown> = {
        name: form.name.trim(),
        code: form.code.trim(),
      };
      if (tabId && tabId !== "__none__") body.cluster_id = tabId;
      else body.cluster_id = null;
      if (form.latitude.trim()) body.latitude = Number(form.latitude);
      else body.latitude = null;
      if (form.longitude.trim()) body.longitude = Number(form.longitude);
      else body.longitude = null;
      const km = form.coverage_radius_km.trim() ? Number(form.coverage_radius_km) : null;
      body.coverage_radius_km = km && km > 0 ? km : null;
      return api(`/api/odps/${editId}`, { method: "PUT", body: JSON.stringify(body) });
    },
    onSuccess: () => {
      setEditId(null);
      setForm(emptyForm);
      setFormErr("");
      refresh();
      void toastSuccess("ODP diperbarui");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/odps/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      if (editId === remove.variables) {
        setEditId(null);
        setForm(emptyForm);
      }
      refresh();
      void toastSuccess("ODP dihapus");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  function startEdit(o: OdpRow) {
    setFormErr("");
    setEditId(o.id);
    setForm({
      name: o.name,
      code: o.code,
      latitude: o.latitude != null ? String(o.latitude) : "",
      longitude: o.longitude != null ? String(o.longitude) : "",
      coverage_radius_km:
        o.coverage_radius_km != null && o.coverage_radius_km > 0 ? String(o.coverage_radius_km) : "",
      port_count: o.port_count,
    });
  }

  function closeForm() {
    setEditId(null);
    setForm(emptyForm);
    setFormErr("");
  }

  return (
    <Section title="MAP FTTH">
      {clusters.length === 0 && unassignedCount === 0 ? (
        <p className="mb-4 text-sm text-[var(--muted)]">
          Belum ada cluster.{" "}
          {onNavigate ? (
            <button
              type="button"
              className="font-medium text-[var(--accent)] underline-offset-2 hover:underline"
              onClick={() => onNavigate("clusters")}
            >
              Buat Cluster/POP dulu →
            </button>
          ) : (
            "Buat Cluster/POP dulu"
          )}{" "}
          (isi lat/long) agar tab &amp; peta bisa dipakai.
        </p>
      ) : (
        <div className="mb-4">
          <Tabs value={tabId} onValueChange={selectClusterTab}>
            <TabsList aria-label="Cluster / POP">
              {clusters.map((c) => (
                <TabsTrigger key={c.id} value={c.id} title={`${c.name} (${c.code})`}>
                  <MapPin />
                  {c.name}
                </TabsTrigger>
              ))}
              {unassignedCount > 0 && (
                <TabsTrigger value="__none__">
                  <MapPin />
                  Tanpa cluster
                </TabsTrigger>
              )}
            </TabsList>
          </Tabs>
        </div>
      )}

      {activeCluster && (
        <p className="mb-3 text-sm text-[var(--muted)]">
          Cluster <span className="font-semibold text-[var(--text)]">{activeCluster.name}</span>
          {clusterCenter
            ? ` · center ${clusterCenter.lat.toFixed(5)}, ${clusterCenter.lng.toFixed(5)}`
            : " · belum ada lat/long (isi di menu Cluster/POP agar peta ter-center)"}
        </p>
      )}
      {tabId === "__none__" && (
        <p className="mb-3 text-sm text-[var(--muted)]">ODP tanpa cluster. Edit lalu pindahkan ke tab cluster yang sesuai.</p>
      )}

      {tabId ? <MapODP odps={filtered} clusterId={tabId} clusterCenter={clusterCenter} /> : null}

      {formErr && !editId && <p className="mb-3 mt-8 text-sm text-[var(--danger)]">{formErr}</p>}
      {importMsg && !importOpen && <p className="mb-3 mt-8 text-sm text-[var(--muted)]">{importMsg}</p>}
      <div className="mt-8">
      <ListToolbar
        search={odpSearch}
        onSearchChange={setOdpSearch}
        searchPlaceholder="Nama atau kode ODP…"
        total={filtered.length}
        page={odpPage}
        pageCount={odpPageCount}
        onPageChange={setOdpPage}
      >
        <span className="ml-auto flex flex-wrap items-center gap-1.5">
          <IconButton label="Export CSV" onClick={() => void exportCsv()}>
            <IconDownload />
          </IconButton>
          <IconButton label="Import CSV" onClick={() => { setImportMsg(""); setImportOpen(true); }}>
            <IconUpload />
          </IconButton>
        </span>
      </ListToolbar>
      <Table
        rowNumberStart={odpPage * 25 + 1}
        columns={["Nama", "Kode", "Port", "Terpakai", "Sisa", "Koordinat", "Coverage", "Aksi"]}
        rows={odpPageItems.map((o) => [
          o.name,
          o.code,
          o.port_count,
          o.used_ports,
          o.free_ports ?? Math.max(0, o.port_count - o.used_ports),
          o.latitude != null && o.longitude != null ? `${o.latitude}, ${o.longitude}` : "—",
          o.coverage_radius_km ? `${o.coverage_radius_km} km` : "—",
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton label="Lihat slot port" onClick={() => setPortsOdpId(o.id)}>
              <IconEye />
            </IconButton>
            <IconButton label="Edit ODP" onClick={() => startEdit(o)}>
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus ODP"
              danger
              disabled={remove.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus ODP",
                  description: `Hapus ODP "${o.name}" (${o.code})?`,
                  confirmLabel: "Hapus",
                });
                if (!ok) return;
                remove.mutate(o.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />
      </div>

      <FormDialog
        open={Boolean(portsOdpId)}
        wide
        title={
          portsQ.data
            ? `Slot port · ${portsQ.data.odp.code}`
            : "Slot port ODP"
        }
        onClose={() => setPortsOdpId(null)}
      >
        <div className="grid gap-3">
          {portsQ.isLoading && <p className="text-sm text-[var(--muted)]">Memuat…</p>}
          {portsQ.data && (
            <p className="text-sm text-[var(--muted)]">
              {portsQ.data.odp.name}: terpakai {portsQ.data.odp.used_ports}, sisa {portsQ.data.odp.free_ports} dari{" "}
              {portsQ.data.odp.port_count} port.
            </p>
          )}
          {portsQ.data && (
            <Table
              columns={["Port", "Status", "Pelanggan", "Username"]}
              rows={portsQ.data.ports.map((p) => [
                p.port_number,
                p.status === "available" ? "kosong" : p.status === "used" ? "terpakai" : p.status,
                p.customer_name || "—",
                p.subscription_username || "—",
              ])}
            />
          )}
          <button type="button" className="btn-ghost w-fit" onClick={() => setPortsOdpId(null)}>
            Tutup
          </button>
        </div>
      </FormDialog>

      <FormDialog open={importOpen} title="Import ODP (CSV)" onClose={() => setImportOpen(false)}>
        <div className="grid gap-3">
          <p className="text-sm text-[var(--muted)]">
            Kolom: <code className="text-xs">code,name,latitude,longitude,port_count,address,cluster_code</code>.
            Upsert berdasarkan <code className="text-xs">code</code>.
            {activeCluster ? ` Cluster default: ${activeCluster.name} (${activeCluster.code}).` : ""}
          </p>
          <div className="flex flex-wrap gap-2">
            <button type="button" className="btn-ghost" onClick={downloadTemplate}>
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
          {importMsg && <p className="text-sm text-[var(--text-body)] whitespace-pre-wrap">{importMsg}</p>}
        </div>
      </FormDialog>

      <FormDialog open={Boolean(editId)} wide title="Edit ODP" onClose={closeForm}>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            update.mutate();
          }}
        >
          <input className="input" placeholder="Nama" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          <input className="input" placeholder="Kode" value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} required />
          <input
            className="input"
            type="number"
            step="any"
            placeholder="Latitude (peta)"
            value={form.latitude}
            onChange={(e) => setForm({ ...form, latitude: e.target.value })}
          />
          <input
            className="input"
            type="number"
            step="any"
            placeholder="Longitude (peta)"
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
          <p className="text-sm text-[var(--muted)] sm:col-span-2">
            Port: {form.port_count}. Cluster tab aktif: {activeCluster?.name || "tanpa cluster"}.
          </p>
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <button className="btn" disabled={update.isPending}>
              {update.isPending ? "Menyimpan…" : "Simpan"}
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

