import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { ListToolbar, matchesQuery } from "../ListToolbar";
import { IconPencil, IconRefresh, IconTrash } from "../icons";
import { useAppDialog } from "../confirm";
import { swalAlert, toastError, toastSuccess } from "../swal";
import {
  FormDialog,
  formatRp,
  IconButton,
  Input,
  SearchableSelect,
  Section,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Table,
  Button,
} from "../ui";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";

export function PlansPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  type PlanRow = {
    id: string;
    name: string;
    code: string;
    price: number;
    download_mbps: number;
    upload_mbps: number;
    service_type: string;
    billing_cycle?: string;
    profile_name?: string | null;
    isolir_profile?: string | null;
    grace_days?: number;
    quota_gb?: number | null;
    limit_uptime?: string | null;
    shared_users?: number | null;
    is_active?: boolean;
  };
  type ClusterOpt = { id: string; name: string; code: string };
  type OfferRow = {
    id: string;
    plan_id: string;
    cluster_id: string;
    price: number;
    is_active: boolean;
    plan_name: string;
    plan_code: string;
    cluster_name: string;
    cluster_code: string;
    download_mbps: number;
    ip_pool_id?: string | null;
    ip_pool_name?: string;
  };
  type PlanForm = {
    name: string;
    code: string;
    price: number;
    download_mbps: number;
    upload_mbps: number;
    service_type: string;
    billing_cycle: string;
    profile_name: string;
    isolir_profile: string;
    grace_days: number;
    quota_gb: string;
    limit_uptime: string;
    shared_users: string;
    is_active: boolean;
  };
  const emptyPlanForm: PlanForm = {
    name: "",
    code: "",
    price: 150000,
    download_mbps: 20,
    upload_mbps: 20,
    service_type: "pppoe",
    billing_cycle: "monthly",
    profile_name: "",
    isolir_profile: "isolir",
    grace_days: 3,
    quota_gb: "",
    limit_uptime: "",
    shared_users: "",
    is_active: true,
  };

  const plansQ = useQuery({
    queryKey: ["plans"],
    queryFn: () => api<PlanRow[]>("/api/plans"),
  });
  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterOpt[]>("/api/clusters"),
  });
  const offersQ = useQuery({
    queryKey: ["plan-offers"],
    queryFn: () => api<OfferRow[]>("/api/plan-offers"),
  });
  type PoolOpt = { id: string; name: string; network: string; router_id?: string | null; router_name?: string | null };
  type RouterOpt = { id: string; name: string; cluster_id?: string | null; is_active: boolean };
  const poolsQ = useQuery({
    queryKey: ["ip-pools"],
    queryFn: () => api<PoolOpt[]>("/api/ip-pools"),
  });
  const routersQ = useQuery({
    queryKey: ["routers"],
    queryFn: () => api<RouterOpt[]>("/api/routers"),
  });
  const [form, setForm] = useState<PlanForm>(emptyPlanForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [planOpen, setPlanOpen] = useState(false);
  const [planErr, setPlanErr] = useState("");
  const [offerForm, setOfferForm] = useState({
    plan_id: "",
    cluster_id: "",
    price: 150000,
    is_active: true,
    sync_profiles: true,
    ip_pool_id: "",
  });
  const [offerEditId, setOfferEditId] = useState<string | null>(null);
  const [offerOpen, setOfferOpen] = useState(false);
  const [offerErr, setOfferErr] = useState("");
  const [syncMsg, setSyncMsg] = useState("");

  const refreshPlans = () => {
    qc.invalidateQueries({ queryKey: ["plans"] });
    qc.invalidateQueries({ queryKey: ["plan-offers"] });
  };

  function openCreatePlan() {
    setOfferOpen(false);
    setEditId(null);
    setPlanErr("");
    setForm(emptyPlanForm);
    setPlanOpen(true);
  }

  function openEditPlan(p: PlanRow) {
    setOfferOpen(false);
    setEditId(p.id);
    setPlanErr("");
    setForm({
      name: p.name,
      code: p.code,
      price: p.price,
      download_mbps: p.download_mbps,
      upload_mbps: p.upload_mbps || p.download_mbps,
      service_type: p.service_type || "pppoe",
      billing_cycle: p.billing_cycle || "monthly",
      profile_name: p.profile_name || "",
      isolir_profile: p.isolir_profile || "isolir",
      grace_days: p.grace_days ?? 3,
      quota_gb: p.quota_gb != null && p.quota_gb > 0 ? String(p.quota_gb) : "",
      limit_uptime: p.limit_uptime || "",
      shared_users: p.shared_users != null && p.shared_users > 0 ? String(p.shared_users) : "",
      is_active: p.is_active !== false,
    });
    setPlanOpen(true);
  }

  function closePlanForm() {
    setPlanOpen(false);
    setEditId(null);
    setPlanErr("");
    setForm(emptyPlanForm);
  }

  function openCreateOffer() {
    setPlanOpen(false);
    setOfferEditId(null);
    setOfferErr("");
    setOfferForm({ plan_id: "", cluster_id: "", price: 150000, is_active: true, sync_profiles: true, ip_pool_id: "" });
    setOfferOpen(true);
  }

  function openEditOffer(o: OfferRow) {
    setPlanOpen(false);
    setOfferEditId(o.id);
    setOfferErr("");
    setOfferForm({
      plan_id: o.plan_id,
      cluster_id: o.cluster_id,
      price: o.price,
      is_active: o.is_active,
      sync_profiles: false,
      ip_pool_id: o.ip_pool_id || "",
    });
    setOfferOpen(true);
  }

  function closeOfferForm() {
    setOfferOpen(false);
    setOfferEditId(null);
    setOfferErr("");
    setOfferForm({ plan_id: "", cluster_id: "", price: 150000, is_active: true, sync_profiles: true, ip_pool_id: "" });
  }

  const savePlan = useMutation({
    mutationFn: () => {
      const body: Record<string, unknown> = {
        name: form.name.trim(),
        service_type: form.service_type,
        price: form.price,
        billing_cycle: form.billing_cycle,
        download_mbps: form.download_mbps,
        upload_mbps: form.upload_mbps,
        profile_name: form.profile_name.trim() || form.code,
        isolir_profile: form.isolir_profile.trim() || "isolir",
        grace_days: Math.max(0, Math.floor(Number(form.grace_days) || 0)),
        tax_percent: 0,
        is_active: form.is_active,
      };
      if (form.service_type === "hotspot") {
        const q = Math.floor(Number(form.quota_gb));
        body.quota_gb = Number.isFinite(q) && q > 0 ? q : null;
        const up = form.limit_uptime.trim();
        body.limit_uptime = up || null;
        const su = Math.floor(Number(form.shared_users));
        body.shared_users = Number.isFinite(su) && su > 0 ? su : null;
      } else {
        body.quota_gb = null;
        body.limit_uptime = null;
        body.shared_users = null;
      }
      if (editId) {
        return api(`/api/plans/${editId}`, { method: "PUT", body: JSON.stringify(body) });
      }
      return api("/api/plans", {
        method: "POST",
        body: JSON.stringify({ ...body, code: form.code.trim() }),
      });
    },
    onSuccess: () => {
      const wasEdit = Boolean(editId);
      closePlanForm();
      refreshPlans();
      void toastSuccess(wasEdit ? "Paket diperbarui" : "Paket ditambahkan");
    },
    onError: (e: Error) => {
      setPlanErr(e.message);
      void toastError(e.message);
    },
  });

  const removePlan = useMutation({
    mutationFn: (id: string) => api(`/api/plans/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      refreshPlans();
      void toastSuccess("Paket dihapus");
    },
    onError: (e: Error) => {
      setPlanErr(e.message);
      void toastError(e.message);
    },
  });

  const upsertOffer = useMutation({
    mutationFn: async () => {
      const wantSync = offerForm.sync_profiles;
      type OfferSaved = OfferRow & { id: string };
      let saved: OfferSaved;
      if (offerEditId) {
        saved = await api<OfferSaved>(`/api/plan-offers/${offerEditId}`, {
          method: "PUT",
          body: JSON.stringify({
            price: offerForm.price,
            is_active: offerForm.is_active,
            ip_pool_id: offerForm.ip_pool_id || null,
            sync_profiles: false,
          }),
        });
      } else {
        saved = await api<OfferSaved>("/api/plan-offers", {
          method: "POST",
          body: JSON.stringify({
            plan_id: offerForm.plan_id,
            cluster_id: offerForm.cluster_id,
            price: offerForm.price,
            is_active: offerForm.is_active,
            ip_pool_id: offerForm.ip_pool_id || null,
            sync_profiles: false,
          }),
        });
      }
      let sync: SyncProfileResult | null = null;
      if (wantSync) {
        sync = await api<SyncProfileResult>(`/api/plan-offers/${saved.id}/sync-profiles`, { method: "POST" });
      }
      return { saved, sync, wasEdit: Boolean(offerEditId), wantSync };
    },
    onSuccess: ({ saved, sync, wasEdit, wantSync }) => {
      closeOfferForm();
      qc.invalidateQueries({ queryKey: ["plan-offers"] });
      if (wantSync && sync) {
        void notifyOfferSync(saved, sync, wasEdit ? "disimpan & disync" : "dibuat & disync");
      } else {
        setSyncMsg(wasEdit ? "Harga per cluster diperbarui (tanpa sync RouterOS)." : "Harga per cluster ditambahkan (tanpa sync RouterOS).");
        void toastSuccess(wasEdit ? "Harga per cluster diperbarui" : "Harga per cluster ditambahkan");
      }
    },
    onError: (e: Error) => {
      setOfferErr(e.message);
      void toastError(e.message);
    },
  });

  const syncOffer = useMutation({
    mutationFn: async (o: OfferRow) => {
      const sync = await api<SyncProfileResult>(`/api/plan-offers/${o.id}/sync-profiles`, { method: "POST" });
      return { o, sync };
    },
    onSuccess: ({ o, sync }) => {
      void notifyOfferSync(o, sync, "disync");
    },
    onError: (e: Error) => {
      setSyncMsg(e.message);
      void toastError(e.message);
      void swalAlert({
        title: "Sync profile gagal",
        description: e.message || "Tidak bisa mendorong profile ke router cluster.",
      });
    },
  });

  const removeOffer = useMutation({
    mutationFn: (id: string) => api(`/api/plan-offers/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["plan-offers"] });
      void toastSuccess("Offer dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const plans = Array.isArray(plansQ.data) ? plansQ.data : [];
  const clusters = Array.isArray(clustersQ.data) ? clustersQ.data : [];
  const offers = Array.isArray(offersQ.data) ? offersQ.data : [];
  const [planSearch, setPlanSearch] = useState("");
  const [planStatus, setPlanStatus] = useState("");
  const [planType, setPlanType] = useState("");
  const [offerSearch, setOfferSearch] = useState("");
  const filteredPlans = useMemo(
    () =>
      plans.filter((p) => {
        if (planStatus === "true" && p.is_active === false) return false;
        if (planStatus === "false" && p.is_active !== false) return false;
        if (planType && p.service_type !== planType) return false;
        return matchesQuery(planSearch, p.name, p.code, p.profile_name, p.service_type);
      }),
    [plans, planSearch, planStatus, planType],
  );
  const filteredOffers = useMemo(
    () =>
      offers.filter((o) =>
        matchesQuery(offerSearch, o.cluster_name, o.cluster_code, o.plan_name, o.plan_code, o.ip_pool_name),
      ),
    [offers, offerSearch],
  );
  const pools = Array.isArray(poolsQ.data) ? poolsQ.data : [];
  const routers = Array.isArray(routersQ.data) ? routersQ.data : [];
  const clusterRouterIds = new Set(
    routers.filter((r) => r.cluster_id && r.cluster_id === offerForm.cluster_id).map((r) => r.id),
  );
  const poolsForCluster = pools.filter((p) => p.router_id && clusterRouterIds.has(p.router_id));

  type SyncProfileResult = {
    synced: number;
    failed: number;
    results?: { router?: string; status?: string; message?: string; profile?: string; error?: string; info?: string }[];
  };

  function notifyOfferSync(
    o: { plan_name?: string; plan_code?: string; cluster_name?: string; cluster_code?: string; price: number },
    sync: SyncProfileResult,
    action: string,
  ) {
    const planLabel = [o.plan_name, o.plan_code ? `(${o.plan_code})` : ""].filter(Boolean).join(" ");
    const clusterLabel = [o.cluster_name, o.cluster_code ? `(${o.cluster_code})` : ""].filter(Boolean).join(" ");
    const detailLines = (sync.results ?? [])
      .map((r) => {
        if (r.error) return `• ${r.error}`;
        if (r.info) return `• ${r.info}`;
        const msg = r.message ? ` — ${r.message}` : "";
        return `• ${r.router || "router"}: ${r.status || "?"}${msg}`;
      })
      .join("\n");
    const summary =
      `Paket: ${planLabel || "—"}\n` +
      `Cluster: ${clusterLabel || "—"}\n` +
      `Harga: ${formatRp(o.price)}\n` +
      `Router sukses: ${sync.synced} · gagal: ${sync.failed}` +
      (detailLines ? `\n\nDetail:\n${detailLines}` : "");
    setSyncMsg(
      sync.failed > 0
        ? `Sync ${planLabel}: ${sync.synced} ok, ${sync.failed} gagal`
        : `Sync ${planLabel} ke ${clusterLabel}: ${sync.synced} router OK`,
    );
    if (sync.failed > 0) {
      void toastError(`Sync selesai: ${sync.failed} router gagal`);
    } else {
      void toastSuccess(`Sync profile OK (${sync.synced} router)`);
    }
    void swalAlert({
      title: sync.failed > 0 ? `Profile ${action} — ada yang gagal` : `Profile ${action}`,
      description: summary,
      icon: sync.failed > 0 ? "warning" : "success",
    });
  }
  return (
    <Section
      title="Paket"
      actions={
        <>
          <button type="button" className="btn-ghost" onClick={openCreateOffer}>
            + Offer cluster
          </button>
          <button type="button" className="btn" onClick={openCreatePlan}>
            + Tambah
          </button>
        </>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Katalog paket tenant-wide. Harga jual per cluster lewat offer. Sync profile mendorong PPP/hotspot ke router di cluster.
      </p>
      {planErr && !planOpen && <p className="mb-3 text-sm text-[var(--danger)]">{planErr}</p>}

      <ListToolbar
        search={planSearch}
        onSearchChange={setPlanSearch}
        searchPlaceholder="Nama, kode, profile…"
        filters={[
          {
            key: "type",
            label: "Tipe",
            value: planType,
            onChange: setPlanType,
            options: [
              { value: "pppoe", label: "PPPoE" },
              { value: "hotspot", label: "Hotspot" },
            ],
          },
          {
            key: "status",
            label: "Status",
            value: planStatus,
            onChange: setPlanStatus,
            options: [
              { value: "true", label: "Aktif" },
              { value: "false", label: "Nonaktif" },
            ],
          },
        ]}
        total={filteredPlans.length}
      />
      <Table
        columns={["Nama", "Kode", "Harga dasar", "DL / UL", "Kuota / limit", "Profile", "Tipe", "Status", "Aksi"]}
        rows={filteredPlans.map((p) => [
          p.name,
          p.code,
          formatRp(p.price),
          `DL ${p.download_mbps} / UL ${p.upload_mbps || p.download_mbps}`,
          p.service_type === "hotspot"
            ? [
                p.quota_gb ? `${p.quota_gb} GB` : null,
                p.limit_uptime || null,
                p.shared_users ? `${p.shared_users} user` : null,
              ]
                .filter(Boolean)
                .join(" · ") || "—"
            : "—",
          p.profile_name || p.code,
          p.service_type,
          p.is_active === false ? "nonaktif" : "aktif",
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton label="Edit paket" onClick={() => openEditPlan(p)}>
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus paket"
              danger
              disabled={removePlan.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus paket",
                  description: `Hapus paket "${p.name}" (${p.code})?`,
                  confirmLabel: "Hapus",
                });
                if (!ok) return;
                removePlan.mutate(p.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />

      <h2 className="mb-3 mt-8 text-sm font-semibold">Harga per cluster (offer)</h2>
      {syncMsg && (
        <p className="mb-2 rounded-md border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm text-[var(--text)]">
          {syncMsg}
        </p>
      )}
      <ListToolbar
        search={offerSearch}
        onSearchChange={setOfferSearch}
        searchPlaceholder="Cluster, paket, pool…"
        total={filteredOffers.length}
      />
      <Table
        columns={["Cluster", "Paket", "Harga", "IP Pool", "DL (Mbps)", "Status", "Aksi"]}
        rows={filteredOffers.map((o) => [
          `${o.cluster_name} (${o.cluster_code})`,
          `${o.plan_name} (${o.plan_code})`,
          formatRp(o.price),
          o.ip_pool_name || "— auto",
          o.download_mbps,
          o.is_active ? "aktif" : "nonaktif",
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton label="Edit harga offer" onClick={() => openEditOffer(o)}>
              <IconPencil />
            </IconButton>
            <IconButton
              label="Sync profile & harga ke router"
              onClick={() => syncOffer.mutate(o)}
              disabled={syncOffer.isPending}
            >
              <IconRefresh />
            </IconButton>
            <IconButton
              label="Hapus offer"
              danger
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus offer",
                  description: `Hapus offer ${o.plan_name} di ${o.cluster_name}?`,
                  confirmLabel: "Hapus",
                });
                if (!ok) return;
                removeOffer.mutate(o.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />

      <FormDialog open={planOpen} wide title={editId ? "Edit paket" : "Tambah paket"} onClose={closePlanForm}>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            savePlan.mutate();
          }}
        >
          <Input placeholder="Nama" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          <Input
            placeholder="Kode"
            value={form.code}
            onChange={(e) => setForm({ ...form, code: e.target.value })}
            required
            disabled={Boolean(editId)}
            title={editId ? "Kode tidak bisa diubah" : undefined}
          />
          <Input type="number" placeholder="Harga dasar" value={form.price} onChange={(e) => setForm({ ...form, price: Number(e.target.value) })} />
          <Input placeholder="Profile RouterOS (kosong = kode)" value={form.profile_name} onChange={(e) => setForm({ ...form, profile_name: e.target.value })} />
          <Input
            placeholder="Profil isolir RouterOS"
            value={form.isolir_profile}
            onChange={(e) => setForm({ ...form, isolir_profile: e.target.value })}
          />
          <div className="grid gap-1.5">
            <Label htmlFor="plan-grace">Grace days (hari setelah jatuh tempo)</Label>
            <Input
              id="plan-grace"
              type="number"
              min={0}
              value={form.grace_days}
              onChange={(e) => setForm({ ...form, grace_days: Number(e.target.value) })}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="plan-dl">Download / DL (Mbps)</Label>
            <Input
              id="plan-dl"
              type="number"
              min={1}
              placeholder="mis. 50"
              value={form.download_mbps}
              onChange={(e) => setForm({ ...form, download_mbps: Number(e.target.value) })}
              required
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="plan-ul">Upload / UL (Mbps)</Label>
            <Input
              id="plan-ul"
              type="number"
              min={1}
              placeholder="mis. 20"
              value={form.upload_mbps}
              onChange={(e) => setForm({ ...form, upload_mbps: Number(e.target.value) })}
              required
            />
          </div>
          <Select value={form.service_type} onValueChange={(v) => setForm({ ...form, service_type: v })}>
            <SelectTrigger>
              <SelectValue placeholder="Tipe layanan" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="pppoe">PPPoE</SelectItem>
              <SelectItem value="hotspot">Hotspot</SelectItem>
              <SelectItem value="static">Static</SelectItem>
            </SelectContent>
          </Select>
          <Select value={form.billing_cycle} onValueChange={(v) => setForm({ ...form, billing_cycle: v })}>
            <SelectTrigger>
              <SelectValue placeholder="Siklus billing" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="monthly">Bulanan</SelectItem>
              <SelectItem value="yearly">Tahunan</SelectItem>
            </SelectContent>
          </Select>
          {form.service_type === "hotspot" ? (
            <>
              <div className="grid gap-1.5">
                <Label htmlFor="plan-quota">Kuota data (GB)</Label>
                <Input
                  id="plan-quota"
                  type="number"
                  min={0}
                  placeholder="Kosong = unlimited"
                  value={form.quota_gb}
                  onChange={(e) => setForm({ ...form, quota_gb: e.target.value })}
                />
                <p className="text-xs text-[var(--muted)]">Di-push ke MikroTik sebagai limit-bytes-total.</p>
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="plan-uptime">Limit waktu (uptime)</Label>
                <Input
                  id="plan-uptime"
                  placeholder="mis. 1d, 12h, 30m"
                  value={form.limit_uptime}
                  onChange={(e) => setForm({ ...form, limit_uptime: e.target.value })}
                />
                <p className="text-xs text-[var(--muted)]">Format RouterOS: 1d / 12h / 30m. Kosong = unlimited.</p>
              </div>
              <div className="grid gap-1.5 sm:col-span-2">
                <Label htmlFor="plan-shared">Shared users (login bersamaan)</Label>
                <Input
                  id="plan-shared"
                  type="number"
                  min={1}
                  placeholder="Kosong = default router (biasanya 1)"
                  value={form.shared_users}
                  onChange={(e) => setForm({ ...form, shared_users: e.target.value })}
                />
              </div>
            </>
          ) : null}
          {editId && (
            <div className="flex items-center gap-2 sm:col-span-2">
              <Checkbox
                id="plan-active"
                checked={form.is_active}
                onCheckedChange={(v) => setForm({ ...form, is_active: v === true })}
              />
              <Label htmlFor="plan-active">Paket aktif</Label>
            </div>
          )}
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <Button type="submit" disabled={savePlan.isPending}>
              {savePlan.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button type="button" variant="secondary" onClick={closePlanForm}>
              Batal
            </Button>
          </div>
          {planErr && <p className="text-sm text-[var(--danger)] sm:col-span-2">{planErr}</p>}
        </form>
      </FormDialog>

      <FormDialog
        open={offerOpen}
        wide
        title={offerEditId ? "Edit harga per cluster" : "Tambah offer cluster"}
        onClose={closeOfferForm}
      >
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            upsertOffer.mutate();
          }}
        >
          <div className="flex min-w-0 flex-col gap-1.5">
            <Label htmlFor="offer-plan">Paket</Label>
            <SearchableSelect
              id="offer-plan"
              required
              disabled={Boolean(offerEditId)}
              placeholder="— Pilih paket —"
              searchPlaceholder="Cari paket…"
              value={offerForm.plan_id}
              onValueChange={(v) => {
                const plan = plans.find((p) => p.id === v);
                setOfferForm({ ...offerForm, plan_id: v, price: plan?.price ?? offerForm.price });
              }}
              options={plans.map((p) => ({
                value: p.id,
                label: `${p.name} (${p.code})`,
                keywords: `${p.name} ${p.code}`,
              }))}
            />
          </div>

          <div className="flex min-w-0 flex-col gap-1.5">
            <Label htmlFor="offer-cluster">Cluster</Label>
            <SearchableSelect
              id="offer-cluster"
              required
              disabled={Boolean(offerEditId)}
              placeholder="— Pilih cluster —"
              searchPlaceholder="Cari cluster…"
              value={offerForm.cluster_id}
              onValueChange={(v) => setOfferForm({ ...offerForm, cluster_id: v, ip_pool_id: "" })}
              options={clusters.map((c) => ({
                value: c.id,
                label: `${c.name} (${c.code})`,
                keywords: `${c.name} ${c.code}`,
              }))}
            />
          </div>

          <div className="flex min-w-0 flex-col gap-1.5">
            <Label htmlFor="offer-price">Harga di cluster (Rp)</Label>
            <Input
              id="offer-price"
              type="number"
              min={0}
              className="w-full"
              value={offerForm.price}
              onChange={(e) => setOfferForm({ ...offerForm, price: Number(e.target.value) })}
              required
            />
          </div>

          <div className="flex min-w-0 flex-col gap-1.5 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/40 p-3">
            <Label htmlFor="offer-pool">IP Pool (untuk profil router)</Label>
            <SearchableSelect
              id="offer-pool"
              allowClear
              clearLabel="— Auto (pool pertama di router) —"
              placeholder="— Auto (pool pertama di router) —"
              searchPlaceholder="Cari pool / network…"
              value={offerForm.ip_pool_id}
              onValueChange={(v) => setOfferForm({ ...offerForm, ip_pool_id: v })}
              disabled={!offerForm.cluster_id}
              options={poolsForCluster.map((p) => ({
                value: p.id,
                label: `${p.name} · ${p.network}${p.router_name ? ` · ${p.router_name}` : ""}`,
                keywords: `${p.name} ${p.network} ${p.router_name || ""}`,
              }))}
            />
            {!offerForm.cluster_id ? (
              <p className="text-xs text-[var(--muted)]">Pilih cluster dulu untuk melihat pool yang tersedia.</p>
            ) : poolsForCluster.length === 0 ? (
              <p className="text-xs text-[var(--muted)]">
                Belum ada IP pool di router cluster ini. Buat di menu IPAM, atau biarkan Auto.
              </p>
            ) : (
              <p className="text-xs text-[var(--muted)]">
                Dipakai sebagai address-pool (hotspot) / remote-address (PPPoE) saat sync profile.
              </p>
            )}
          </div>

          <div className="flex flex-col gap-2">
            <label className="flex items-center gap-2 text-sm text-[var(--text-body)]">
              <input
                type="checkbox"
                checked={offerForm.is_active}
                onChange={(e) => setOfferForm({ ...offerForm, is_active: e.target.checked })}
              />
              Offer aktif
            </label>
            <label className="flex items-center gap-2 text-sm text-[var(--text-body)]">
              <input
                type="checkbox"
                checked={offerForm.sync_profiles}
                onChange={(e) => setOfferForm({ ...offerForm, sync_profiles: e.target.checked })}
              />
              Sync profile ke router cluster
            </label>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={upsertOffer.isPending}>
              {upsertOffer.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button type="button" variant="secondary" onClick={closeOfferForm}>
              Batal
            </Button>
          </div>
          {offerErr && <p className="text-sm text-[var(--danger)]">{offerErr}</p>}
        </form>
      </FormDialog>
    </Section>
  );
}

