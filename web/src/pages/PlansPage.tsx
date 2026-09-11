import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { ListToolbar, matchesQuery } from "../ListToolbar";
import { IconPencil, IconTrash } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import {
  FormDialog,
  formatRp,
  IconButton,
  Input,
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
import type { AdminPage } from "../admin/pages";

export function PlansPage({ onNavigate }: { onNavigate?: (page: AdminPage) => void }) {
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
    due_day?: number | null;
    quota_gb?: number | null;
    limit_uptime?: string | null;
    shared_users?: number | null;
    is_active?: boolean;
    portal_visible?: boolean;
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
    due_day: string;
    quota_gb: string;
    limit_uptime: string;
    shared_users: string;
    is_active: boolean;
    portal_visible: boolean;
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
    due_day: "",
    quota_gb: "",
    limit_uptime: "",
    shared_users: "",
    is_active: true,
    portal_visible: false,
  };

  const plansQ = useQuery({
    queryKey: ["plans"],
    queryFn: () => api<PlanRow[]>("/api/plans"),
  });
  const [form, setForm] = useState<PlanForm>(emptyPlanForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [planOpen, setPlanOpen] = useState(false);
  const [planErr, setPlanErr] = useState("");

  const refreshPlans = () => {
    qc.invalidateQueries({ queryKey: ["plans"] });
  };

  function openCreatePlan() {
    setEditId(null);
    setPlanErr("");
    setForm(emptyPlanForm);
    setPlanOpen(true);
  }

  function openEditPlan(p: PlanRow) {
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
      due_day: p.due_day != null ? String(p.due_day) : "",
      quota_gb: p.quota_gb != null && p.quota_gb > 0 ? String(p.quota_gb) : "",
      limit_uptime: p.limit_uptime || "",
      shared_users: p.shared_users != null && p.shared_users > 0 ? String(p.shared_users) : "",
      is_active: p.is_active !== false,
      portal_visible: p.portal_visible === true,
    });
    setPlanOpen(true);
  }

  function closePlanForm() {
    setPlanOpen(false);
    setEditId(null);
    setPlanErr("");
    setForm(emptyPlanForm);
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
        due_day:
          form.due_day.trim() === "" ? null : Math.max(1, Math.min(28, Math.floor(Number(form.due_day)))),
        tax_percent: 0,
        is_active: form.is_active,
        portal_visible: form.portal_visible,
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

  const plans = Array.isArray(plansQ.data) ? plansQ.data : [];
  const [planSearch, setPlanSearch] = useState("");
  const [planStatus, setPlanStatus] = useState("");
  const [planType, setPlanType] = useState("");
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
  return (
    <Section
      title="Paket"
      actions={
        <button type="button" className="btn" onClick={openCreatePlan}>
          + Tambah
        </button>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Katalog paket dasar yang berlaku untuk semua pelanggan. Centang tampil di portal agar pelanggan bisa
        upgrade/downgrade. Harga jual per cluster diatur di{" "}
        {onNavigate ? (
          <button
            type="button"
            className="font-medium text-[var(--accent)] underline-offset-2 hover:underline"
            onClick={() => onNavigate("offers")}
          >
            menu Paket per Cluster →
          </button>
        ) : (
          "menu Paket per Cluster"
        )}
        .
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
        columns={["Nama", "Kode", "Harga dasar", "DL / UL", "Kuota / limit", "Profile", "Jatuh tempo", "Tipe", "Portal", "Status", "Aksi"]}
        rows={filteredPlans.map((p) => [
          p.name,
          p.code,
          p.price === 0 ? "Gratis" : formatRp(p.price),
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
          p.due_day != null ? `tgl ${p.due_day}` : "— default",
          p.service_type,
          p.portal_visible ? "tampil" : "—",
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
          <div className="grid gap-1">
            <Input type="number" placeholder="Harga dasar" value={form.price} onChange={(e) => setForm({ ...form, price: Number(e.target.value) })} />
            <p className="text-xs text-[var(--muted)]">
              Isi 0 untuk paket gratis (mis. fasilitas umum): tidak ditagih & tidak diisolir otomatis.
            </p>
          </div>
          <Input placeholder="Profile RouterOS (kosong = kode)" value={form.profile_name} onChange={(e) => setForm({ ...form, profile_name: e.target.value })} />
          <Input
            placeholder="Profil isolir RouterOS"
            value={form.isolir_profile}
            onChange={(e) => setForm({ ...form, isolir_profile: e.target.value })}
          />
          <div className="grid gap-1.5">
            <Label htmlFor="plan-due">Tanggal jatuh tempo invoice (1–28)</Label>
            <Input
              id="plan-due"
              type="number"
              min={1}
              max={28}
              placeholder="Kosong = ikut pengaturan umum"
              value={form.due_day}
              onChange={(e) => setForm({ ...form, due_day: e.target.value })}
            />
            <p className="text-xs text-[var(--muted)]">
              Tanggal kalender jatuh tempo invoice paket ini. Kosongkan untuk memakai tanggal dari Pengaturan → Umum.
              Masa tenggang isolir diatur terpisah.
            </p>
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
          <div className="grid gap-1 sm:col-span-2">
            <div className="flex items-center gap-2">
              <Checkbox
                id="plan-portal"
                checked={form.portal_visible}
                onCheckedChange={(v) => setForm({ ...form, portal_visible: v === true })}
              />
              <Label htmlFor="plan-portal">Tampil di portal pelanggan</Label>
            </div>
            <p className="text-xs text-[var(--muted)]">
              Pelanggan bisa upgrade/downgrade ke paket ini. Di cluster, tetap butuh offer aktif.
            </p>
          </div>
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
    </Section>
  );
}

