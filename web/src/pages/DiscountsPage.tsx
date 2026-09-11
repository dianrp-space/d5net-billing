import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { ListToolbar, matchesQuery, usePagination } from "../ListToolbar";
import { IconPencil, IconTrash } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import {
  FormDialog,
  formatRp,
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
} from "../ui";
import { Checkbox } from "@/components/ui/checkbox";

type DiscountRow = {
  id: string;
  name: string;
  plan_id?: string | null;
  plan_name?: string;
  kind: "percent" | "amount" | string;
  value: number;
  starts_on: string;
  ends_on: string;
  audience: "all" | "selected" | string;
  is_active: boolean;
  customer_ids?: string[];
  customer_count: number;
};

type PlanOpt = { id: string; name: string; code: string; price: number };
type CustomerOpt = { id: string; full_name: string; customer_code: string; phone?: string };

type DiscountForm = {
  name: string;
  plan_id: string;
  kind: "percent" | "amount";
  value: number;
  starts_on: string;
  ends_on: string;
  audience: "all" | "selected";
  is_active: boolean;
  customer_ids: string[];
};

function localISODate(d = new Date()) {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function todayISO() {
  return localISODate();
}

function plusDaysISO(days: number) {
  const d = new Date();
  d.setDate(d.getDate() + days);
  return localISODate(d);
}

function emptyForm(): DiscountForm {
  return {
    name: "",
    plan_id: "",
    kind: "percent",
    value: 10,
    starts_on: todayISO(),
    ends_on: plusDaysISO(30),
    audience: "all",
    is_active: true,
    customer_ids: [],
  };
}

function formatDateRange(start: string, end: string) {
  const fmt = (s: string) => {
    const [y, m, d] = s.split("-");
    if (!y || !m || !d) return s;
    return `${d}/${m}/${y}`;
  };
  return `${fmt(start)} – ${fmt(end)}`;
}

function discountStatus(row: DiscountRow) {
  if (!row.is_active) return "nonaktif";
  const today = todayISO();
  if (row.ends_on < today) return "berakhir";
  if (row.starts_on > today) return "mendatang";
  return "berlaku";
}

function valueLabel(row: DiscountRow) {
  if (row.kind === "amount") return formatRp(row.value);
  return `${row.value}%`;
}

export function DiscountsPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [open, setOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState<DiscountForm>(emptyForm);
  const [custSearch, setCustSearch] = useState("");

  const listQ = useQuery({
    queryKey: ["plan-discounts"],
    queryFn: () => api<DiscountRow[]>("/api/plan-discounts"),
  });
  const plansQ = useQuery({
    queryKey: ["plans"],
    queryFn: () => api<PlanOpt[]>("/api/plans"),
  });
  const customersQ = useQuery({
    queryKey: ["customers", "discounts"],
    queryFn: () => api<{ data: CustomerOpt[] }>("/api/customers?limit=1000"),
    enabled: open,
  });

  const rows = Array.isArray(listQ.data) ? listQ.data : [];
  const plans = Array.isArray(plansQ.data) ? plansQ.data : [];
  const customers = Array.isArray(customersQ.data?.data) ? customersQ.data!.data : [];

  const filtered = useMemo(() => {
    return rows.filter((r) => {
      if (statusFilter && discountStatus(r) !== statusFilter) return false;
      return matchesQuery(search, r.name, r.plan_name, r.audience, valueLabel(r));
    });
  }, [rows, search, statusFilter]);
  const { page: discPage, setPage: setDiscPage, pageCount: discPageCount, pageItems: discPageItems } =
    usePagination(filtered, 25);

  const customerHits = useMemo(() => {
    return customers.filter((c) =>
      matchesQuery(custSearch, c.full_name, c.customer_code, c.phone),
    );
  }, [customers, custSearch]);

  function closeForm() {
    setOpen(false);
    setEditId(null);
    setForm(emptyForm());
    setCustSearch("");
  }

  function openCreate() {
    setEditId(null);
    setForm(emptyForm());
    setCustSearch("");
    setOpen(true);
  }

  async function openEdit(row: DiscountRow) {
    setEditId(row.id);
    setCustSearch("");
    setOpen(true);
    try {
      const full = await api<DiscountRow>(`/api/plan-discounts/${row.id}`);
      setForm({
        name: full.name,
        plan_id: full.plan_id || "",
        kind: full.kind === "amount" ? "amount" : "percent",
        value: full.value,
        starts_on: full.starts_on,
        ends_on: full.ends_on,
        audience: full.audience === "selected" ? "selected" : "all",
        is_active: full.is_active,
        customer_ids: full.customer_ids ?? [],
      });
    } catch (e) {
      toastError(e instanceof Error ? e.message : "Gagal memuat diskon");
      closeForm();
    }
  }

  function buildBody() {
    return {
      name: form.name.trim(),
      plan_id: form.plan_id || null,
      kind: form.kind,
      value: Number(form.value) || 0,
      starts_on: form.starts_on,
      ends_on: form.ends_on,
      audience: form.audience,
      is_active: form.is_active,
      customer_ids: form.audience === "selected" ? form.customer_ids : [],
    };
  }

  const save = useMutation({
    mutationFn: () => {
      const body = JSON.stringify(buildBody());
      if (editId) {
        return api(`/api/plan-discounts/${editId}`, { method: "PUT", body });
      }
      return api("/api/plan-discounts", { method: "POST", body });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["plan-discounts"] });
      toastSuccess(editId ? "Diskon diperbarui" : "Diskon dibuat");
      closeForm();
    },
    onError: (e: Error) => toastError(e.message || "Gagal menyimpan diskon"),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/plan-discounts/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["plan-discounts"] });
      toastSuccess("Diskon dihapus");
    },
    onError: (e: Error) => toastError(e.message || "Gagal menghapus diskon"),
  });

  return (
    <Section
      title="Diskon paket"
      actions={
        <button type="button" className="btn" onClick={openCreate}>
          + Tambah
        </button>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Potongan harga paket untuk pelanggan dalam rentang tanggal. Bisa berlaku untuk semua
        pelanggan atau yang dipilih saja. Tagihan berikutnya memakai harga setelah diskon.
      </p>

      <ListToolbar
        search={search}
        onSearchChange={setSearch}
        searchPlaceholder="Nama, paket…"
        filters={[
          {
            key: "status",
            label: "Status",
            value: statusFilter,
            onChange: setStatusFilter,
            options: [
              { value: "berlaku", label: "Berlaku" },
              { value: "mendatang", label: "Mendatang" },
              { value: "berakhir", label: "Berakhir" },
              { value: "nonaktif", label: "Nonaktif" },
            ],
          },
        ]}
        total={filtered.length}
        page={discPage}
        pageCount={discPageCount}
        onPageChange={setDiscPage}
      />

      <Table
        rowNumberStart={discPage * 25 + 1}
        columns={["Nama", "Paket", "Diskon", "Periode", "Sasaran", "Status", "Aksi"]}
        rows={discPageItems.map((r) => [
          r.name,
          r.plan_name || "Semua paket",
          valueLabel(r),
          formatDateRange(r.starts_on, r.ends_on),
          r.audience === "selected" ? `${r.customer_count} pelanggan` : "Semua",
          discountStatus(r),
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton label="Edit diskon" onClick={() => void openEdit(r)}>
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus diskon"
              danger
              disabled={remove.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus diskon",
                  description: `Hapus diskon "${r.name}"?`,
                  confirmLabel: "Hapus",
                });
                if (!ok) return;
                remove.mutate(r.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />

      <FormDialog open={open} wide title={editId ? "Edit diskon" : "Tambah diskon"} onClose={closeForm}>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <div className="sm:col-span-2">
            <Label htmlFor="disc-name">Nama</Label>
            <Input
              id="disc-name"
              className="mt-1.5"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="Promo Lebaran"
              required
            />
          </div>
          <div>
            <Label>Paket</Label>
            <div className="mt-1.5">
              <SearchableSelect
                value={form.plan_id}
                onValueChange={(v) => setForm({ ...form, plan_id: v })}
                placeholder="Semua paket"
                allowClear
                clearLabel="Semua paket"
                options={plans.map((p) => ({
                  value: p.id,
                  label: `${p.name} (${p.code})`,
                  keywords: p.code,
                }))}
              />
            </div>
          </div>
          <div>
            <Label>Jenis</Label>
            <div className="mt-1.5">
              <Select
                value={form.kind}
                onValueChange={(v) => setForm({ ...form, kind: v === "amount" ? "amount" : "percent" })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="percent">Persen (%)</SelectItem>
                  <SelectItem value="amount">Nominal (Rp)</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div>
            <Label htmlFor="disc-value">{form.kind === "amount" ? "Nominal potongan" : "Persentase"}</Label>
            <Input
              id="disc-value"
              className="mt-1.5"
              type="number"
              min={1}
              max={form.kind === "percent" ? 100 : undefined}
              value={form.value}
              onChange={(e) => setForm({ ...form, value: Number(e.target.value) })}
              required
            />
          </div>
          <div>
            <Label>Sasaran</Label>
            <div className="mt-1.5">
              <Select
                value={form.audience}
                onValueChange={(v) => setForm({ ...form, audience: v === "selected" ? "selected" : "all" })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Semua pelanggan</SelectItem>
                  <SelectItem value="selected">Pelanggan tertentu</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div>
            <Label htmlFor="disc-start">Mulai</Label>
            <Input
              id="disc-start"
              className="mt-1.5"
              type="date"
              value={form.starts_on}
              onChange={(e) => setForm({ ...form, starts_on: e.target.value })}
              required
            />
          </div>
          <div>
            <Label htmlFor="disc-end">Sampai</Label>
            <Input
              id="disc-end"
              className="mt-1.5"
              type="date"
              value={form.ends_on}
              onChange={(e) => setForm({ ...form, ends_on: e.target.value })}
              required
            />
          </div>
          <label className="flex items-center gap-2 text-sm sm:col-span-2">
            <Checkbox
              checked={form.is_active}
              onCheckedChange={(v) => setForm({ ...form, is_active: v === true })}
            />
            Aktif
          </label>

          {form.audience === "selected" ? (
            <div className="sm:col-span-2 space-y-2">
              <Label htmlFor="disc-cust">Pelanggan ({form.customer_ids.length} dipilih)</Label>
              <Input
                id="disc-cust"
                value={custSearch}
                onChange={(e) => setCustSearch(e.target.value)}
                placeholder="Cari nama, kode, atau telepon…"
              />
              <div className="max-h-52 overflow-auto rounded-md border border-[var(--border)] bg-[var(--panel)] p-2">
                {customerHits.length === 0 ? (
                  <p className="px-1 py-2 text-sm text-[var(--muted)]">Tidak ada pelanggan.</p>
                ) : (
                  customerHits.slice(0, 80).map((c) => {
                    const checked = form.customer_ids.includes(c.id);
                    return (
                      <label key={c.id} className="flex cursor-pointer items-center gap-2 rounded px-1 py-1.5 text-sm hover:bg-[var(--bg)]">
                        <Checkbox
                          checked={checked}
                          onCheckedChange={(v) => {
                            setForm((prev) => ({
                              ...prev,
                              customer_ids:
                                v === true
                                  ? prev.customer_ids.includes(c.id)
                                    ? prev.customer_ids
                                    : [...prev.customer_ids, c.id]
                                  : prev.customer_ids.filter((x) => x !== c.id),
                            }));
                          }}
                        />
                        <span className="min-w-0 truncate">
                          {c.full_name}
                          <span className="text-[var(--muted)]"> · {c.customer_code}</span>
                        </span>
                      </label>
                    );
                  })
                )}
              </div>
            </div>
          ) : null}

          <div className="flex justify-end gap-2 sm:col-span-2">
            <button type="button" className="btn-ghost" onClick={closeForm}>
              Batal
            </button>
            <button type="submit" className="btn" disabled={save.isPending}>
              {editId ? "Simpan" : "Buat"}
            </button>
          </div>
        </form>
      </FormDialog>
    </Section>
  );
}
