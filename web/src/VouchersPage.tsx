import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiDownload } from "./api";
import { useAppDialog } from "./confirm";
import { Badge } from "./components/ui/badge";
import { IconDownload, IconEye, IconRefresh, IconTrash } from "./icons";
import { ListToolbar, matchesQuery } from "./ListToolbar";
import { toastError, toastSuccess } from "./swal";
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
  formatRp,
} from "./ui";

type Batch = {
  id: string;
  name: string;
  plan_id?: string | null;
  plan_name?: string;
  router_id?: string | null;
  router_name?: string;
  quantity: number;
  price: number;
  available: number;
  used: number;
  expires_at?: string | null;
  sync_status?: string;
  sync_error?: string;
  synced_count?: number;
  synced_at?: string | null;
  created_at?: string;
};

type VoucherRow = {
  id: string;
  code: string;
  status: string;
  used_by_name?: string;
  used_at?: string | null;
  expires_at?: string | null;
  created_at?: string;
};

type PlanOpt = { id: string; name: string; code: string; price: number; service_type?: string };
type RouterOpt = { id: string; name: string; is_active: boolean; provisioner?: string };

function formatWhen(iso?: string | null) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("id-ID");
  } catch {
    return iso;
  }
}

function syncBadge(status?: string) {
  switch (status) {
    case "synced":
      return <Badge variant="success">Synced</Badge>;
    case "partial":
      return <Badge variant="outline">Partial</Badge>;
    case "failed":
      return <Badge variant="danger">Gagal</Badge>;
    case "pending":
      return <Badge variant="outline">Pending</Badge>;
    default:
      return <Badge variant="outline">—</Badge>;
  }
}

export function VouchersPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [open, setOpen] = useState(false);
  const [detail, setDetail] = useState<Batch | null>(null);
  const [codeStatus, setCodeStatus] = useState("");
  const [form, setForm] = useState({
    name: "Voucher hotspot",
    quantity: "10",
    price: "10000",
    prefix: "VCH",
    plan_id: "",
    router_id: "",
    expires_at: "",
  });
  const [formErr, setFormErr] = useState("");

  const batchesQ = useQuery({
    queryKey: ["voucher-batches"],
    queryFn: () => api<Batch[]>("/api/vouchers/batches"),
  });
  const plansQ = useQuery({
    queryKey: ["plans"],
    queryFn: () => api<PlanOpt[]>("/api/plans"),
  });
  const routersQ = useQuery({
    queryKey: ["routers"],
    queryFn: () => api<RouterOpt[]>("/api/routers"),
  });
  const codesQ = useQuery({
    queryKey: ["voucher-codes", detail?.id, codeStatus],
    queryFn: () =>
      api<VoucherRow[]>(
        `/api/vouchers/batches/${detail!.id}/codes${codeStatus ? `?status=${encodeURIComponent(codeStatus)}` : ""}`,
      ),
    enabled: Boolean(detail?.id),
  });

  const batches = Array.isArray(batchesQ.data) ? batchesQ.data : [];
  const [batchSearch, setBatchSearch] = useState("");
  const filteredBatches = useMemo(
    () => batches.filter((b) => matchesQuery(batchSearch, b.name, b.plan_name, b.router_name)),
    [batches, batchSearch],
  );
  const plans = (Array.isArray(plansQ.data) ? plansQ.data : []).filter(
    (p) => !p.service_type || p.service_type === "hotspot",
  );
  const routers = (Array.isArray(routersQ.data) ? routersQ.data : []).filter((r) => r.is_active);
  const codes = Array.isArray(codesQ.data) ? codesQ.data : [];

  useEffect(() => {
    if (!detail) setCodeStatus("");
  }, [detail]);

  const create = useMutation({
    mutationFn: () => {
      const qty = Math.floor(Number(form.quantity) || 0);
      const price = Math.floor(Number(form.price) || 0);
      if (!form.name.trim()) throw new Error("Nama batch wajib");
      if (!form.router_id) throw new Error("Router wajib dipilih");
      if (qty < 1) throw new Error("Jumlah minimal 1");
      if (qty > 5000) throw new Error("Maksimal 5000 kode");
      if (price < 0) throw new Error("Harga tidak valid");
      const body: Record<string, unknown> = {
        name: form.name.trim(),
        quantity: qty,
        price,
        prefix: form.prefix.trim() || "VCH",
        router_id: form.router_id,
      };
      if (form.plan_id) body.plan_id = form.plan_id;
      if (form.expires_at) {
        const d = new Date(form.expires_at);
        if (!Number.isNaN(d.getTime())) body.expires_at = d.toISOString();
      }
      return api<Batch>("/api/vouchers/batches", { method: "POST", body: JSON.stringify(body) });
    },
    onSuccess: (b) => {
      setOpen(false);
      setFormErr("");
      setForm({
        name: "Voucher hotspot",
        quantity: "10",
        price: "10000",
        prefix: "VCH",
        plan_id: "",
        router_id: "",
        expires_at: "",
      });
      qc.invalidateQueries({ queryKey: ["voucher-batches"] });
      const syncNote =
        b.sync_status === "synced"
          ? ` · ${b.synced_count ?? b.quantity} user di router`
          : b.sync_status === "partial"
            ? ` · sync partial (${b.synced_count ?? 0})`
            : b.sync_status === "failed"
              ? " · sync gagal"
              : "";
      void toastSuccess(`Batch dibuat · ${b.quantity} kode${syncNote}`);
      setDetail(b);
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/vouchers/batches/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      setDetail(null);
      qc.invalidateQueries({ queryKey: ["voucher-batches"] });
      void toastSuccess("Batch dihapus (user hotspot dihapus dari router bila ada)");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const resync = useMutation({
    mutationFn: (id: string) => api<Batch>(`/api/vouchers/batches/${id}/sync`, { method: "POST" }),
    onSuccess: (b) => {
      qc.invalidateQueries({ queryKey: ["voucher-batches"] });
      setDetail(b);
      if (b.sync_status === "synced") {
        void toastSuccess(`Sync OK · ${b.synced_count ?? 0} user`);
      } else if (b.sync_status === "partial") {
        void toastSuccess(`Sync partial · ${b.synced_count ?? 0} berhasil`);
      } else {
        void toastError(b.sync_error || "Sync gagal");
      }
    },
    onError: (e: Error) => void toastError(e.message),
  });

  return (
    <Section
      title="Voucher Hotspot"
      actions={
        <Button type="button" onClick={() => { setFormErr(""); setOpen(true); }}>
          + Generate batch
        </Button>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Generate kode, push sebagai user hotspot ke MikroTik (username = password = kode), lalu unduh CSV untuk cetak.
        IP dialokasikan lewat <strong>IP Pool</strong> yang terhubung ke router yang sama (profil hotspot{" "}
        <code className="text-xs">address-pool</code>) — pastikan pool sudah dibuat di menu IPAM.
      </p>

      <ListToolbar
        search={batchSearch}
        onSearchChange={setBatchSearch}
        searchPlaceholder="Nama batch, router, paket…"
        total={filteredBatches.length}
      />
      <Table
        columns={["Batch", "Router", "Paket", "Harga", "Qty", "Sync", "Tersedia", "Terpakai", "Aksi"]}
        rows={filteredBatches.map((b) => [
          <div key={`${b.id}-n`}>
            <p className="font-medium">{b.name}</p>
            <p className="text-[10px] text-[var(--muted)]">{formatWhen(b.created_at)}</p>
          </div>,
          b.router_name || "—",
          b.plan_name || "—",
          formatRp(b.price),
          b.quantity,
          <span key={`${b.id}-s`} title={b.sync_error || undefined}>
            {syncBadge(b.sync_status)}
          </span>,
          b.available,
          b.used,
          <span key={`${b.id}-a`} className="flex flex-wrap items-center gap-1.5">
            <IconButton
              label="Detail kode"
              onClick={() => {
                setDetail(b);
                setCodeStatus("");
              }}
            >
              <IconEye />
            </IconButton>
            <IconButton
              label="Sync ulang ke router"
              disabled={!b.router_id || resync.isPending}
              onClick={() => resync.mutate(b.id)}
            >
              <IconRefresh />
            </IconButton>
            <IconButton
              label="Unduh CSV"
              onClick={() =>
                void apiDownload(`/api/vouchers/batches/${b.id}/export.csv`, `vouchers-${b.name.replace(/\s+/g, "-")}.csv`).catch(
                  (e: Error) => toastError(e.message || "Export gagal"),
                )
              }
            >
              <IconDownload />
            </IconButton>
            <IconButton
              label="Hapus batch"
              danger
              onClick={() => {
                void confirm({
                  title: "Hapus batch voucher?",
                  description: `Hapus "${b.name}" beserta semua kodenya dan user hotspot di router?`,
                  confirmLabel: "Hapus",
                  danger: true,
                }).then((ok) => {
                  if (ok) remove.mutate(b.id);
                });
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />
      {filteredBatches.length === 0 && !batchesQ.isLoading ? (
        <p className="mt-3 text-sm text-[var(--muted)]">Belum ada batch. Generate batch untuk mulai.</p>
      ) : null}

      <FormDialog open={open} title="Generate batch voucher" onClose={() => setOpen(false)} wide>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <div className="sm:col-span-2">
            <Label className="mb-1.5 block">Nama batch</Label>
            <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </div>
          <div className="sm:col-span-2">
            <Label className="mb-1.5 block">Router MikroTik</Label>
            <SearchableSelect
              required
              placeholder="— Pilih router —"
              searchPlaceholder="Cari router…"
              value={form.router_id}
              onValueChange={(v) => setForm({ ...form, router_id: v })}
              options={routers.map((r) => ({
                value: r.id,
                label: r.name,
                keywords: r.name,
              }))}
            />
            <p className="mt-1 text-xs text-[var(--muted)]">Kode akan dibuat sebagai /ip/hotspot/user di router ini.</p>
          </div>
          <div>
            <Label className="mb-1.5 block">Jumlah kode</Label>
            <Input
              type="number"
              min={1}
              max={5000}
              value={form.quantity}
              onChange={(e) => setForm({ ...form, quantity: e.target.value })}
              required
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Harga jual (Rp)</Label>
            <Input
              type="number"
              min={0}
              value={form.price}
              onChange={(e) => setForm({ ...form, price: e.target.value })}
              required
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Prefix kode</Label>
            <Input
              value={form.prefix}
              onChange={(e) => setForm({ ...form, prefix: e.target.value.toUpperCase() })}
              placeholder="VCH"
              maxLength={8}
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Kadaluarsa (opsional)</Label>
            <Input
              type="datetime-local"
              value={form.expires_at}
              onChange={(e) => setForm({ ...form, expires_at: e.target.value })}
            />
          </div>
          <div className="sm:col-span-2">
            <Label className="mb-1.5 block">Paket hotspot (profil + kuota + IP pool offer)</Label>
            <SearchableSelect
              allowClear
              clearLabel="— Profil default router —"
              placeholder="Paket"
              searchPlaceholder="Cari paket hotspot…"
              value={form.plan_id}
              onValueChange={(v) => setForm({ ...form, plan_id: v })}
              options={plans.map((p) => ({
                value: p.id,
                label: `${p.name} (${formatRp(p.price)})`,
                keywords: `${p.name} ${p.code}`,
              }))}
            />
            <p className="mt-1 text-xs text-[var(--muted)]">
              Agar IP pool dari harga per cluster terpakai, pilih paket hotspot yang punya offer + IP pool di cluster router ini.
            </p>
          </div>
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? "Generate + sync…" : "Generate & sync"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>
              Batal
            </Button>
          </div>
          {formErr ? <p className="text-sm text-[var(--danger)] sm:col-span-2">{formErr}</p> : null}
        </form>
      </FormDialog>

      <FormDialog
        open={Boolean(detail)}
        wide
        title={detail ? `Batch: ${detail.name}` : "Detail batch"}
        onClose={() => setDetail(null)}
      >
        {detail ? (
          <div className="grid gap-4">
            <div className="grid gap-2 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/40 p-3 text-sm sm:grid-cols-2">
              <p>
                <span className="text-[var(--muted)]">Router:</span> {detail.router_name || "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">Paket:</span> {detail.plan_name || "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">Harga:</span> {formatRp(detail.price)}
              </p>
              <p>
                <span className="text-[var(--muted)]">Qty:</span> {detail.quantity}
              </p>
              <p>
                <span className="text-[var(--muted)]">Tersedia / terpakai:</span> {detail.available} / {detail.used}
              </p>
              <p className="flex flex-wrap items-center gap-2">
                <span className="text-[var(--muted)]">Sync:</span> {syncBadge(detail.sync_status)}
                {detail.synced_count != null ? (
                  <span className="text-xs text-[var(--muted)]">({detail.synced_count} user)</span>
                ) : null}
              </p>
              {detail.sync_error ? (
                <p className="sm:col-span-2 text-xs text-[var(--danger)]">{detail.sync_error}</p>
              ) : null}
              <p>
                <span className="text-[var(--muted)]">Kadaluarsa:</span> {formatWhen(detail.expires_at)}
              </p>
              <p>
                <span className="text-[var(--muted)]">Dibuat:</span> {formatWhen(detail.created_at)}
              </p>
            </div>

            <div className="flex flex-wrap items-end gap-3">
              <div className="min-w-[160px]">
                <Label className="mb-1.5 block">Filter status</Label>
                <Select value={codeStatus || "__all__"} onValueChange={(v) => setCodeStatus(v === "__all__" ? "" : v)}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__all__">Semua</SelectItem>
                    <SelectItem value="available">Tersedia</SelectItem>
                    <SelectItem value="used">Terpakai</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Button
                type="button"
                variant="outline"
                disabled={!detail.router_id || resync.isPending}
                onClick={() => resync.mutate(detail.id)}
              >
                <IconRefresh /> {resync.isPending ? "Sync…" : "Sync ulang"}
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  void apiDownload(
                    `/api/vouchers/batches/${detail.id}/export.csv`,
                    `vouchers-${detail.name.replace(/\s+/g, "-")}.csv`,
                  ).catch((e: Error) => toastError(e.message || "Export gagal"))
                }
              >
                <IconDownload /> CSV
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  void apiDownload(`/api/vouchers/batches/${detail.id}/qr`, `voucher-qr-${detail.id.slice(0, 8)}.png`).catch(
                    (e: Error) => toastError(e.message || "QR gagal"),
                  )
                }
              >
                <IconDownload /> QR contoh
              </Button>
            </div>

            <Table
              columns={["Kode", "Status", "Dipakai oleh", "Dipakai pada", "Kadaluarsa"]}
              rows={codes.map((c) => [
                <code key={`${c.id}-c`} className="text-xs font-semibold">
                  {c.code}
                </code>,
                <Badge key={`${c.id}-s`} variant={c.status === "available" ? "success" : "outline"}>
                  {c.status === "available" ? "Tersedia" : "Terpakai"}
                </Badge>,
                c.used_by_name || "—",
                formatWhen(c.used_at),
                formatWhen(c.expires_at),
              ])}
            />
            {codesQ.isLoading ? <p className="text-sm text-[var(--muted)]">Memuat kode…</p> : null}
            {!codesQ.isLoading && codes.length === 0 ? (
              <p className="text-sm text-[var(--muted)]">Tidak ada kode{codeStatus ? " dengan filter ini" : ""}.</p>
            ) : null}
          </div>
        ) : null}
      </FormDialog>
    </Section>
  );
}
