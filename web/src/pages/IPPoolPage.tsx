import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MapPin } from "lucide-react";
import { api } from "../api";
import { useAppDialog } from "../confirm";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { IconPencil, IconTrash, IconWrench } from "../icons";
import { ListToolbar, matchesQuery } from "../ListToolbar";
import { getLastIpPoolCluster, setLastIpPoolCluster } from "../navPersist";
import { toastError, toastSuccess } from "../swal";
import {
  Button,
  FormDialog,
  IconButton,
  Input,
  Label,
  SearchableSelect,
  Section,
  Table,
} from "../ui";

export function IPPoolPage({ tenantSlug }: { tenantSlug?: string }) {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  type IpPoolRow = {
    id: string;
    name: string;
    network: string;
    gateway?: string | null;
    cluster_id?: string | null;
    cluster_name?: string | null;
    router_id?: string | null;
    router_name?: string | null;
    dns_servers?: string[] | null;
    used_count?: number;
  };
  type ClusterTab = { id: string; name: string; code: string; is_active: boolean };
  type RouterOpt = { id: string; name: string; is_active: boolean; cluster_id?: string | null };
  type CustomerOpt = { id: string; full_name: string; customer_code: string };
  type AssignmentRow = {
    id: string;
    ip_address: string;
    mac_address?: string | null;
    status: string;
    customer_id?: string | null;
    customer_name?: string | null;
  };
  type PoolForm = { name: string; network: string; gateway: string; cluster_id: string; router_id: string; dns: string };
  const emptyForm: PoolForm = {
    name: "",
    network: "10.10.0.0/24",
    gateway: "10.10.0.1",
    cluster_id: "",
    router_id: "",
    dns: "8.8.8.8, 1.1.1.1",
  };

  const [tabId, setTabId] = useState<string>(() =>
    tenantSlug ? getLastIpPoolCluster(tenantSlug) || "" : "",
  );
  const [form, setForm] = useState<PoolForm>(emptyForm);
  const [open, setOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [formErr, setFormErr] = useState("");
  const [assignPool, setAssignPool] = useState<IpPoolRow | null>(null);
  const [assignForm, setAssignForm] = useState({ ip_address: "", customer_id: "", mac_address: "" });
  const [assignErr, setAssignErr] = useState("");
  const [search, setSearch] = useState("");
  const [routerFilter, setRouterFilter] = useState("");

  const q = useQuery({
    queryKey: ["ip-pools"],
    queryFn: () => api<IpPoolRow[]>("/api/ip-pools"),
  });
  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterTab[]>("/api/clusters"),
  });
  const routersQ = useQuery({
    queryKey: ["routers"],
    queryFn: () => api<RouterOpt[]>("/api/routers"),
  });
  const assignClusterId = assignPool?.cluster_id || "";
  const customersQ = useQuery({
    queryKey: ["customers", "ip-pool", assignClusterId],
    queryFn: () => {
      const qs = new URLSearchParams({ limit: "500" });
      if (assignClusterId) qs.set("cluster_id", assignClusterId);
      return api<{ data: CustomerOpt[] }>(`/api/customers?${qs.toString()}`);
    },
    enabled: Boolean(assignPool),
  });
  const assignmentsQ = useQuery({
    queryKey: ["ip-assignments", assignPool?.id],
    queryFn: () => api<AssignmentRow[]>(`/api/ip-pools/${assignPool!.id}/assignments`),
    enabled: Boolean(assignPool),
  });

  const list = Array.isArray(q.data) ? q.data : [];
  const clusters = (Array.isArray(clustersQ.data) ? clustersQ.data : []).filter((c) => c.is_active);
  const routersAll = Array.isArray(routersQ.data) ? routersQ.data : [];
  const customers = Array.isArray(customersQ.data?.data) ? customersQ.data!.data : [];
  const assignments = Array.isArray(assignmentsQ.data) ? assignmentsQ.data : [];
  const unassignedCount = list.filter((p) => !p.cluster_id).length;

  useEffect(() => {
    const saved = tenantSlug ? getLastIpPoolCluster(tenantSlug) : null;
    const savedOk =
      saved === "__none__"
        ? unassignedCount > 0
        : Boolean(saved && clusters.some((c) => c.id === saved));

    if (tabId) {
      const tabOk =
        tabId === "__none__" ? unassignedCount > 0 : clusters.some((c) => c.id === tabId);
      if (tabOk) {
        if (tenantSlug) setLastIpPoolCluster(tenantSlug, tabId);
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
    setTabId(clusters[0].id);
  }, [clusters, tabId, unassignedCount, tenantSlug]);

  function selectClusterTab(id: string) {
    setTabId(id);
    setRouterFilter("");
    setSearch("");
    if (tenantSlug) setLastIpPoolCluster(tenantSlug, id);
  }

  const filtered = useMemo(
    () =>
      list.filter((p) => {
        if (tabId === "__none__") {
          if (p.cluster_id) return false;
        } else if (tabId && p.cluster_id !== tabId) {
          return false;
        }
        if (routerFilter === "__none__" && p.router_id) return false;
        if (routerFilter && routerFilter !== "__none__" && p.router_id !== routerFilter) return false;
        return matchesQuery(search, p.name, p.network, p.gateway, p.router_name);
      }),
    [list, search, tabId, routerFilter],
  );

  const tabRouters = routersAll.filter((r) => {
    if (tabId === "__none__") return !r.cluster_id;
    if (tabId) return r.cluster_id === tabId;
    return true;
  });
  const formRouters = routersAll.filter((r) => {
    const clusterId = form.cluster_id;
    return (
      (r.is_active || r.id === form.router_id) &&
      (!clusterId || r.cluster_id === clusterId || r.id === form.router_id)
    );
  });

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["ip-pools"] });
    if (assignPool) qc.invalidateQueries({ queryKey: ["ip-assignments", assignPool.id] });
  };

  function parseDNS(s: string) {
    return s
      .split(/[,;\s]+/)
      .map((x) => x.trim())
      .filter(Boolean);
  }

  function buildBody() {
    const body: Record<string, unknown> = {
      name: form.name.trim(),
      network: form.network.trim(),
      dns_servers: parseDNS(form.dns),
    };
    body.gateway = form.gateway.trim() || null;
    body.cluster_id = form.cluster_id || null;
    body.router_id = form.router_id || null;
    return body;
  }

  const save = useMutation({
    mutationFn: () => {
      const body = buildBody();
      if (editId) return api(`/api/ip-pools/${editId}`, { method: "PUT", body: JSON.stringify(body) });
      return api("/api/ip-pools", { method: "POST", body: JSON.stringify(body) });
    },
    onSuccess: () => {
      const wasEdit = Boolean(editId);
      const movedCluster = form.cluster_id;
      closeForm();
      refresh();
      if (movedCluster && movedCluster !== tabId) {
        selectClusterTab(movedCluster);
      }
      void toastSuccess(wasEdit ? "IP Pool diperbarui" : "IP Pool ditambahkan");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/ip-pools/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      if (editId === remove.variables) closeForm();
      if (assignPool?.id === remove.variables) setAssignPool(null);
      refresh();
      void toastSuccess("IP Pool dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const assign = useMutation({
    mutationFn: () =>
      api(`/api/ip-pools/${assignPool!.id}/assignments`, {
        method: "POST",
        body: JSON.stringify({
          ip_address: assignForm.ip_address.trim(),
          customer_id: assignForm.customer_id || undefined,
          mac_address: assignForm.mac_address.trim() || undefined,
          status: "assigned",
        }),
      }),
    onSuccess: () => {
      setAssignForm({ ip_address: "", customer_id: "", mac_address: "" });
      setAssignErr("");
      refresh();
      void toastSuccess("IP di-assign");
    },
    onError: (e: Error) => {
      setAssignErr(e.message);
      void toastError(e.message);
    },
  });

  const release = useMutation({
    mutationFn: (assignmentId: string) =>
      api(`/api/ip-pools/${assignPool!.id}/assignments/${assignmentId}`, { method: "DELETE" }),
    onSuccess: () => {
      refresh();
      void toastSuccess("IP dilepas");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  function openCreate() {
    if (!tabId || tabId === "__none__") return;
    setEditId(null);
    setForm({ ...emptyForm, cluster_id: tabId });
    setFormErr("");
    setOpen(true);
  }

  function openEdit(p: IpPoolRow) {
    setEditId(p.id);
    setForm({
      name: p.name,
      network: p.network,
      gateway: p.gateway || "",
      cluster_id: p.cluster_id || "",
      router_id: p.router_id || "",
      dns: (p.dns_servers ?? []).join(", "),
    });
    setFormErr("");
    setOpen(true);
  }

  function closeForm() {
    setOpen(false);
    setEditId(null);
    setForm(emptyForm);
    setFormErr("");
  }

  const activeCluster = clusters.find((c) => c.id === tabId) || null;
  const canCreate = Boolean(tabId && tabId !== "__none__");
  const showClusterPicker = tabId === "__none__";

  return (
    <Section
      title="IP Pool"
      actions={
        canCreate ? (
          <Button type="button" onClick={openCreate}>
            + Tambah
          </Button>
        ) : undefined
      }
    >
      {clusters.length === 0 && unassignedCount === 0 ? (
        <p className="mb-4 text-sm text-[var(--muted)]">
          Belum ada cluster. Buat Cluster/POP dulu agar pool bisa dikelompokkan per tab.
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

      <p className="mb-4 text-sm text-[var(--muted)]">
        Pool CIDR untuk alamat IP pelanggan / hotspot
        {activeCluster ? (
          <>
            {" "}
            di cluster <strong>{activeCluster.name}</strong>
          </>
        ) : tabId === "__none__" ? (
          <> yang belum terikat cluster</>
        ) : null}
        . Nama pool boleh sama di cluster lain. Router MikroTik opsional (untuk sync RouterOS).
      </p>

      <ListToolbar
        search={search}
        onSearchChange={setSearch}
        searchPlaceholder="Nama, network, router…"
        filters={[
          {
            key: "router",
            label: "Router",
            value: routerFilter,
            onChange: setRouterFilter,
            options: [
              { value: "__none__", label: "Tanpa router" },
              ...tabRouters.map((r) => ({ value: r.id, label: r.name })),
            ],
            className: "min-w-[200px]",
          },
        ]}
        total={filtered.length}
      />

      <Table
        columns={["Nama", "Network", "Gateway", "Router", "Terpakai", "Aksi"]}
        rows={filtered.map((p) => [
          p.name,
          p.network,
          p.gateway || "—",
          p.router_name || "—",
          String(p.used_count ?? 0),
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton
              label="Kelola assignment"
              onClick={() => {
                setAssignForm({ ip_address: "", customer_id: "", mac_address: "" });
                setAssignErr("");
                setAssignPool(p);
              }}
            >
              <IconWrench />
            </IconButton>
            <IconButton label="Edit IP Pool" onClick={() => openEdit(p)}>
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus IP Pool"
              danger
              disabled={remove.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus IP Pool",
                  description: `Hapus pool "${p.name}" (${p.network})? Assignment di dalamnya ikut terhapus.`,
                  confirmLabel: "Hapus",
                });
                if (!ok) return;
                remove.mutate(p.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />

      <FormDialog open={open} title={editId ? "Edit IP Pool" : "Tambah IP Pool"} onClose={closeForm} wide>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <div className="grid gap-1.5 sm:col-span-2">
            <Label htmlFor="pool-name">Nama pool</Label>
            <Input
              id="pool-name"
              placeholder="mis. Hotspot-Pool / PPPoE-Pool-A"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              required
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="pool-net">Network (CIDR)</Label>
            <Input
              id="pool-net"
              placeholder="10.10.0.0/24"
              value={form.network}
              onChange={(e) => setForm({ ...form, network: e.target.value })}
              required
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="pool-gw">Gateway</Label>
            <Input
              id="pool-gw"
              placeholder="10.10.0.1"
              value={form.gateway}
              onChange={(e) => setForm({ ...form, gateway: e.target.value })}
            />
          </div>
          {showClusterPicker ? (
            <div className="grid gap-1.5">
              <Label>Pindah ke cluster</Label>
              <SearchableSelect
                allowClear
                clearLabel="— Tetap tanpa cluster —"
                placeholder="Pilih cluster"
                searchPlaceholder="Cari cluster…"
                value={form.cluster_id}
                onValueChange={(v) => {
                  const r = routersAll.find((x) => x.id === form.router_id);
                  const keepRouter = Boolean(r && (!v || r.cluster_id === v));
                  setForm({ ...form, cluster_id: v, router_id: keepRouter ? form.router_id : "" });
                }}
                options={clusters.map((c) => ({
                  value: c.id,
                  label: `${c.name} (${c.code})`,
                  keywords: `${c.name} ${c.code}`,
                }))}
              />
            </div>
          ) : (
            <div className="grid gap-1.5">
              <Label>Cluster</Label>
              <Input value={activeCluster ? `${activeCluster.name} (${activeCluster.code})` : "—"} disabled />
            </div>
          )}
          <div className="grid gap-1.5">
            <Label>Router terkait</Label>
            <SearchableSelect
              allowClear
              clearLabel="— Tanpa router —"
              placeholder="Pilih router"
              searchPlaceholder="Cari router…"
              value={form.router_id}
              onValueChange={(v) => {
                const r = routersAll.find((x) => x.id === v);
                setForm({
                  ...form,
                  router_id: v,
                  cluster_id: showClusterPicker ? r?.cluster_id || form.cluster_id : form.cluster_id,
                });
              }}
              options={formRouters.map((r) => ({
                value: r.id,
                label: r.name,
                keywords: r.name,
              }))}
            />
          </div>
          <div className="grid gap-1.5 sm:col-span-2">
            <Label htmlFor="pool-dns">DNS (pisahkan koma)</Label>
            <Input
              id="pool-dns"
              placeholder="8.8.8.8, 1.1.1.1"
              value={form.dns}
              onChange={(e) => setForm({ ...form, dns: e.target.value })}
            />
          </div>
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
        open={Boolean(assignPool)}
        wide
        title={assignPool ? `Assignment · ${assignPool.name}` : "Assignment"}
        onClose={() => {
          setAssignPool(null);
          setAssignErr("");
          setAssignForm({ ip_address: "", customer_id: "", mac_address: "" });
        }}
      >
        <p className="mb-3 text-sm text-[var(--muted)]">
          Network {assignPool?.network}
          {assignPool?.cluster_name ? ` · Cluster ${assignPool.cluster_name}` : ""}
          {assignPool?.router_name ? ` · Router ${assignPool.router_name}` : ""}
          {assignPool?.cluster_id
            ? " · daftar pelanggan difilter menurut cluster pool."
            : " · pool tanpa cluster menampilkan semua pelanggan."}
        </p>
        <form
          className="mb-4 grid gap-3 sm:grid-cols-3"
          onSubmit={(e) => {
            e.preventDefault();
            assign.mutate();
          }}
        >
          <Input
            placeholder="IP address"
            value={assignForm.ip_address}
            onChange={(e) => setAssignForm({ ...assignForm, ip_address: e.target.value })}
            required
          />
          <SearchableSelect
            allowClear
            clearLabel="— Tanpa pelanggan —"
            placeholder={assignPool?.cluster_id ? "Pelanggan cluster ini" : "Pelanggan (opsional)"}
            searchPlaceholder="Cari nama / kode…"
            value={assignForm.customer_id}
            onValueChange={(v) => setAssignForm({ ...assignForm, customer_id: v })}
            options={customers.map((c) => ({
              value: c.id,
              label: `${c.full_name} (${c.customer_code})`,
              keywords: `${c.full_name} ${c.customer_code}`,
            }))}
          />
          <Input
            placeholder="MAC (opsional)"
            value={assignForm.mac_address}
            onChange={(e) => setAssignForm({ ...assignForm, mac_address: e.target.value })}
          />
          <div className="sm:col-span-3">
            {assignPool?.cluster_id && customers.length === 0 ? (
              <p className="mb-2 text-xs text-[var(--muted)]">Tidak ada pelanggan di cluster pool ini.</p>
            ) : null}
            <Button type="submit" disabled={assign.isPending}>
              {assign.isPending ? "Menyimpan…" : "Assign IP"}
            </Button>
            {assignErr && <p className="mt-2 text-sm text-[var(--danger)]">{assignErr}</p>}
          </div>
        </form>
        <Table
          columns={["IP", "Pelanggan", "MAC", "Status", "Aksi"]}
          rows={assignments.map((a) => [
            a.ip_address,
            a.customer_name || "—",
            a.mac_address || "—",
            a.status,
            <IconButton
              key="del"
              label="Lepas IP"
              danger
              disabled={release.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: "Lepas IP",
                  description: `Lepas assignment ${a.ip_address}?`,
                  confirmLabel: "Lepas",
                });
                if (!ok) return;
                release.mutate(a.id);
              }}
            >
              <IconTrash />
            </IconButton>,
          ])}
        />
      </FormDialog>
    </Section>
  );
}
