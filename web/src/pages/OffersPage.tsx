import { useEffect, useMemo, useState } from "react";
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
  Table,
  Button,
} from "../ui";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { AdminPage } from "../admin/pages";

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
  due_day?: number | null;
};

type SyncProfileResult = {
  synced: number;
  failed: number;
  results?: { router?: string; status?: string; message?: string; profile?: string; error?: string; info?: string }[];
};

export function OffersPage({ onNavigate }: { onNavigate?: (page: AdminPage) => void }) {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  type PlanOpt = { id: string; name: string; code: string; price: number };
  type ClusterOpt = { id: string; name: string; code: string };
  type PoolOpt = { id: string; name: string; network: string; router_id?: string | null; router_name?: string | null; cluster_id?: string | null };
  type RouterOpt = { id: string; name: string; cluster_id?: string | null; is_active: boolean };

  const plansQ = useQuery({
    queryKey: ["plans"],
    queryFn: () => api<PlanOpt[]>("/api/plans"),
  });
  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterOpt[]>("/api/clusters"),
  });
  const offersQ = useQuery({
    queryKey: ["plan-offers"],
    queryFn: () => api<OfferRow[]>("/api/plan-offers"),
  });
  const poolsQ = useQuery({
    queryKey: ["ip-pools"],
    queryFn: () => api<PoolOpt[]>("/api/ip-pools"),
  });
  const routersQ = useQuery({
    queryKey: ["routers"],
    queryFn: () => api<RouterOpt[]>("/api/routers"),
  });

  const [offerForm, setOfferForm] = useState({
    plan_id: "",
    cluster_id: "",
    price: 150000,
    is_active: true,
    sync_profiles: true,
    ip_pool_id: "",
    due_day: "",
  });
  const [offerEditId, setOfferEditId] = useState<string | null>(null);
  const [offerOpen, setOfferOpen] = useState(false);
  const [offerErr, setOfferErr] = useState("");
  const [syncMsg, setSyncMsg] = useState("");
  const [offerSearch, setOfferSearch] = useState("");
  const [offerClusterTab, setOfferClusterTab] = useState("");

  const plans = Array.isArray(plansQ.data) ? plansQ.data : [];
  const clusters = Array.isArray(clustersQ.data) ? clustersQ.data : [];
  const offers = Array.isArray(offersQ.data) ? offersQ.data : [];
  const pools = Array.isArray(poolsQ.data) ? poolsQ.data : [];
  const routers = Array.isArray(routersQ.data) ? routersQ.data : [];

  function openCreateOffer() {
    setOfferEditId(null);
    setOfferErr("");
    setOfferForm({ plan_id: "", cluster_id: offerClusterTab, price: 150000, is_active: true, sync_profiles: true, ip_pool_id: "", due_day: "" });
    setOfferOpen(true);
  }

  function openEditOffer(o: OfferRow) {
    setOfferEditId(o.id);
    setOfferErr("");
    setOfferForm({
      plan_id: o.plan_id,
      cluster_id: o.cluster_id,
      price: o.price,
      is_active: o.is_active,
      sync_profiles: false,
      ip_pool_id: o.ip_pool_id || "",
      due_day: o.due_day != null ? String(o.due_day) : "",
    });
    setOfferOpen(true);
  }

  function closeOfferForm() {
    setOfferOpen(false);
    setOfferEditId(null);
    setOfferErr("");
    setOfferForm({ plan_id: "", cluster_id: "", price: 150000, is_active: true, sync_profiles: true, ip_pool_id: "", due_day: "" });
  }

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

  const upsertOffer = useMutation({
    mutationFn: async () => {
      const wantSync = offerForm.sync_profiles;
      const dueDay =
        offerForm.due_day.trim() === "" ? null : Math.max(1, Math.min(28, Math.floor(Number(offerForm.due_day))));
      type OfferSaved = OfferRow & { id: string };
      let saved: OfferSaved;
      if (offerEditId) {
        saved = await api<OfferSaved>(`/api/plan-offers/${offerEditId}`, {
          method: "PUT",
          body: JSON.stringify({
            price: offerForm.price,
            is_active: offerForm.is_active,
            ip_pool_id: offerForm.ip_pool_id || null,
            due_day: dueDay,
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
            due_day: dueDay,
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

  const filteredOffers = useMemo(
    () =>
      offers.filter((o) => {
        if (offerClusterTab && o.cluster_id !== offerClusterTab) return false;
        return matchesQuery(offerSearch, o.plan_name, o.plan_code, o.ip_pool_name);
      }),
    [offers, offerSearch, offerClusterTab],
  );

  useEffect(() => {
    if (clusters.length === 0) {
      if (offerClusterTab) setOfferClusterTab("");
      return;
    }
    if (offerClusterTab && clusters.some((c) => c.id === offerClusterTab)) return;
    setOfferClusterTab(clusters[0].id);
  }, [clusters, offerClusterTab]);

  function selectOfferClusterTab(id: string) {
    setOfferClusterTab(id);
    setOfferSearch("");
  }

  const activeOfferCluster = clusters.find((c) => c.id === offerClusterTab) || null;
  const clusterRouterIds = new Set(
    routers.filter((r) => r.cluster_id && r.cluster_id === offerForm.cluster_id).map((r) => r.id),
  );
  const poolsForCluster = pools.filter((p) => {
    if (!offerForm.cluster_id) return false;
    if (p.cluster_id && p.cluster_id === offerForm.cluster_id) return true;
    return Boolean(p.router_id && clusterRouterIds.has(p.router_id));
  });

  return (
    <Section
      title="Paket per Cluster"
      actions={
        <button type="button" className="btn" onClick={openCreateOffer}>
          + Tambah offer
        </button>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Harga jual paket dasar per cluster / POP. Pelanggan hanya bisa dipasang paket yang punya offer aktif di cluster-nya.
      </p>
      {syncMsg && (
        <p className="mb-2 rounded-md border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm text-[var(--text)]">
          {syncMsg}
        </p>
      )}
      {clusters.length === 0 ? (
        <p className="text-sm text-[var(--muted)]">
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
          agar harga per cluster bisa ditampilkan per tab.
        </p>
      ) : (
        <>
          <Tabs value={offerClusterTab} onValueChange={selectOfferClusterTab}>
            <TabsList aria-label="Cluster / POP">
              {clusters.map((c) => {
                const count = offers.filter((o) => o.cluster_id === c.id).length;
                return (
                  <TabsTrigger key={c.id} value={c.id} title={`${c.name} (${c.code})`}>
                    {c.name}
                    <span className="rounded-full border border-[var(--border)] px-1.5 text-xs text-[var(--muted)]">
                      {count}
                    </span>
                  </TabsTrigger>
                );
              })}
            </TabsList>
          </Tabs>
          <p className="mb-4 mt-3 text-sm text-[var(--muted)]">
            {activeOfferCluster ? (
              <>
                Harga jual paket di cluster <strong>{activeOfferCluster.name}</strong> ({activeOfferCluster.code}).
              </>
            ) : (
              "Harga jual paket per cluster."
            )}
          </p>
          <ListToolbar
            search={offerSearch}
            onSearchChange={setOfferSearch}
            searchPlaceholder="Paket, pool…"
            total={filteredOffers.length}
          />
          <Table
            columns={["Paket", "Harga", "Jatuh tempo", "IP Pool", "DL (Mbps)", "Status", "Aksi"]}
            rows={filteredOffers.map((o) => [
              `${o.plan_name} (${o.plan_code})`,
              formatRp(o.price),
              o.due_day != null ? `tgl ${o.due_day}` : "— paket",
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
        </>
      )}

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

          <div className="flex min-w-0 flex-col gap-1.5">
            <Label htmlFor="offer-due">Tanggal jatuh tempo di cluster (1–28)</Label>
            <Input
              id="offer-due"
              type="number"
              min={1}
              max={28}
              placeholder="Kosong = ikut paket"
              value={offerForm.due_day}
              onChange={(e) => setOfferForm({ ...offerForm, due_day: e.target.value })}
            />
            <p className="text-xs text-[var(--muted)]">
              Tanggal kalender jatuh tempo di cluster ini. Kosongkan untuk memakai jatuh tempo paket.
            </p>
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
                Belum ada IP pool di cluster ini.{" "}
                {onNavigate ? (
                  <button
                    type="button"
                    className="font-medium text-[var(--accent)] underline-offset-2 hover:underline"
                    onClick={() => onNavigate("ip-pool")}
                  >
                    Buat di menu IP Pool →
                  </button>
                ) : (
                  "Buat di menu IP Pool"
                )}{" "}
                (tab cluster ini), atau biarkan Auto.
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
