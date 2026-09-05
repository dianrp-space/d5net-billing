import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import ReactECharts from "echarts-for-react";
import { api, apiDownload, getToken } from "./api";
import { useAppDialog } from "./confirm";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { IconBanknote, IconBan, IconCheck, IconDownload, IconPencil, IconTrash, IconWrench } from "./icons";
import { toastError, toastSuccess } from "./swal";
import {
  Button,
  Card,
  FormDialog,
  IconButton,
  Input,
  Label,
  formatRp,
  Section,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Table,
} from "./ui";

export type ResellerOpt = { id: string; name: string; phone?: string | null; balance?: number; commission_percent?: number; is_active?: boolean };
export type StaffOpt = { user_id: string; full_name: string; email: string; is_active: boolean };

/** Pilih Reseller ATAU Staff/Tim (saling eksklusif). */
export function AttributionSelects({
  resellerId,
  salesUserId,
  onChange,
  resellers,
  users,
}: {
  resellerId: string;
  salesUserId: string;
  onChange: (next: { reseller_id: string; sales_user_id: string }) => void;
  resellers: ResellerOpt[];
  users: StaffOpt[];
}) {
  const activeUsers = users.filter((u) => u.is_active);
  return (
    <div className="grid gap-3 sm:grid-cols-2 sm:col-span-2">
      <div>
        <Label className="mb-1.5 block">Reseller (opsional)</Label>
        <Select
          value={resellerId || "__none__"}
          onValueChange={(v) => {
            const id = v === "__none__" ? "" : v;
            onChange({ reseller_id: id, sales_user_id: id ? "" : salesUserId });
          }}
        >
          <SelectTrigger>
            <SelectValue placeholder="Reseller" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__none__">— Tidak ada —</SelectItem>
            {resellers.map((r) => (
              <SelectItem key={r.id} value={r.id}>
                {r.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div>
        <Label className="mb-1.5 block">Sales (opsional)</Label>
        <Select
          value={salesUserId || "__none__"}
          onValueChange={(v) => {
            const id = v === "__none__" ? "" : v;
            onChange({ reseller_id: id ? "" : resellerId, sales_user_id: id });
          }}
          disabled={Boolean(resellerId)}
        >
          <SelectTrigger>
            <SelectValue placeholder="Sales" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__none__">— Tidak ada —</SelectItem>
            {activeUsers.map((u) => (
              <SelectItem key={u.user_id} value={u.user_id}>
                {u.full_name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {resellerId ? (
          <p className="mt-1 text-xs text-[var(--muted)]">Kosongkan reseller untuk memilih sales.</p>
        ) : (
          <p className="mt-1 text-xs text-[var(--muted)]">Salah satu: reseller atau sales (semua user aktif).</p>
        )}
      </div>
    </div>
  );
}

export const COMMISSION_BASIS_OPTIONS = [
  { value: "new_customer_flat", label: "Pelanggan baru" },
  { value: "acquisition_flat", label: "Akuisisi" },
] as const;

export function commissionBasisLabel(basis: string) {
  return COMMISSION_BASIS_OPTIONS.find((o) => o.value === basis)?.label || basis || "—";
}

/** Jenis komisi flat (pelanggan baru vs akuisisi). */
export function CommissionBasisSelect({
  value,
  onChange,
  className,
}: {
  value: string;
  onChange: (v: string) => void;
  className?: string;
}) {
  return (
    <div className={className ?? "sm:col-span-2"}>
      <Label className="mb-1.5 block">Jenis komisi</Label>
      <Select value={value || "new_customer_flat"} onValueChange={onChange}>
        <SelectTrigger>
          <SelectValue placeholder="Jenis komisi" />
        </SelectTrigger>
        <SelectContent>
          {COMMISSION_BASIS_OPTIONS.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="mt-1 text-xs text-[var(--muted)]">
        Nominal diambil dari setting Reseller &amp; Komisi sesuai jenis ini.
      </p>
    </div>
  );
}

export function AlertsPanel() {
  const q = useQuery({
    queryKey: ["alerts"],
    queryFn: () => api<{ severity: string; title: string; message: string }[]>("/api/alerts"),
    refetchInterval: 15000,
  });
  const rows = Array.isArray(q.data) ? q.data : [];
  return (
    <div className="mb-2">
      <h2 className="eyebrow mb-3">Alert realtime</h2>
      <Table
        columns={["Severity", "Judul", "Pesan"]}
        rows={rows.slice(0, 8).map((a) => [a.severity, a.title, a.message])}
      />
    </div>
  );
}

export function useDashboardSSE() {
  const [live, setLive] = useState<Record<string, unknown> | null>(null);
  useEffect(() => {
    const token = getToken();
    let tid = "0";
    if (token) {
      try {
        const payload = JSON.parse(atob(token.split(".")[1] || "")) as { tid?: string };
        if (payload.tid) tid = String(payload.tid);
      } catch {
        /* ignore */
      }
    }
    const es = new EventSource(`/events/stream?tenant_id=${tid}${token ? `&access_token=${encodeURIComponent(token)}` : ""}`);
    es.onmessage = (ev) => {
      try {
        setLive(JSON.parse(ev.data) as Record<string, unknown>);
      } catch {
        /* ignore */
      }
    };
    return () => es.close();
  }, []);
  return live;
}

export function IPAMPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  type IpPoolRow = {
    id: string;
    name: string;
    network: string;
    gateway?: string | null;
    router_id?: string | null;
    router_name?: string | null;
    dns_servers?: string[] | null;
    used_count?: number;
  };
  type RouterOpt = { id: string; name: string; is_active: boolean };
  type CustomerOpt = { id: string; full_name: string; customer_code: string };
  type AssignmentRow = {
    id: string;
    ip_address: string;
    mac_address?: string | null;
    status: string;
    customer_id?: string | null;
    customer_name?: string | null;
  };
  type PoolForm = { name: string; network: string; gateway: string; router_id: string; dns: string };
  const emptyForm: PoolForm = {
    name: "",
    network: "10.10.0.0/24",
    gateway: "10.10.0.1",
    router_id: "",
    dns: "8.8.8.8, 1.1.1.1",
  };

  const [form, setForm] = useState<PoolForm>(emptyForm);
  const [open, setOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [formErr, setFormErr] = useState("");
  const [assignPool, setAssignPool] = useState<IpPoolRow | null>(null);
  const [assignForm, setAssignForm] = useState({ ip_address: "", customer_id: "", mac_address: "" });
  const [assignErr, setAssignErr] = useState("");

  const q = useQuery({
    queryKey: ["ip-pools"],
    queryFn: () => api<IpPoolRow[]>("/api/ip-pools"),
  });
  const routersQ = useQuery({
    queryKey: ["routers"],
    queryFn: () => api<RouterOpt[]>("/api/routers"),
  });
  const customersQ = useQuery({
    queryKey: ["customers"],
    queryFn: () => api<{ data: CustomerOpt[] }>("/api/customers?limit=200"),
    enabled: Boolean(assignPool),
  });
  const assignmentsQ = useQuery({
    queryKey: ["ip-assignments", assignPool?.id],
    queryFn: () => api<AssignmentRow[]>(`/api/ip-pools/${assignPool!.id}/assignments`),
    enabled: Boolean(assignPool),
  });

  const list = Array.isArray(q.data) ? q.data : [];
  const routers = (Array.isArray(routersQ.data) ? routersQ.data : []).filter((r) => r.is_active);
  const customers = Array.isArray(customersQ.data?.data) ? customersQ.data!.data : [];
  const assignments = Array.isArray(assignmentsQ.data) ? assignmentsQ.data : [];

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
      closeForm();
      refresh();
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
    setEditId(null);
    setForm(emptyForm);
    setFormErr("");
    setOpen(true);
  }

  function openEdit(p: IpPoolRow) {
    setEditId(p.id);
    setForm({
      name: p.name,
      network: p.network,
      gateway: p.gateway || "",
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

  return (
    <Section
      title="IP Pool"
      actions={
        <Button type="button" onClick={openCreate}>
          + Tambah
        </Button>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Pool CIDR untuk alamat IP pelanggan. Pool terhubung ke <strong>router</strong> (MikroTik), bukan ke paket. Paket hanya mengatur bandwidth/profile.
      </p>

      <Table
        columns={["Nama", "Network", "Gateway", "Router", "Terpakai", "Aksi"]}
        rows={list.map((p) => [
          p.name,
          p.network,
          p.gateway || "—",
          p.router_name || "—",
          String(p.used_count ?? 0),
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton label="Kelola assignment" onClick={() => setAssignPool(p)}>
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
              placeholder="mis. PPPoE-Pool-A"
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
          <div className="grid gap-1.5 sm:col-span-2">
            <Label>Router terkait</Label>
            <Select
              value={form.router_id || "__none__"}
              onValueChange={(v) => setForm({ ...form, router_id: v === "__none__" ? "" : v })}
            >
              <SelectTrigger>
                <SelectValue placeholder="Pilih router" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__none__">— Tanpa router —</SelectItem>
                {routers.map((r) => (
                  <SelectItem key={r.id} value={r.id}>
                    {r.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
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
          {assignPool?.router_name ? ` · Router ${assignPool.router_name}` : ""}
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
          <Select
            value={assignForm.customer_id || "__none__"}
            onValueChange={(v) => setAssignForm({ ...assignForm, customer_id: v === "__none__" ? "" : v })}
          >
            <SelectTrigger>
              <SelectValue placeholder="Pelanggan (opsional)" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__none__">— Tanpa pelanggan —</SelectItem>
              {customers.map((c) => (
                <SelectItem key={c.id} value={c.id}>
                  {c.full_name} ({c.customer_code})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input
            placeholder="MAC (opsional)"
            value={assignForm.mac_address}
            onChange={(e) => setAssignForm({ ...assignForm, mac_address: e.target.value })}
          />
          <div className="sm:col-span-3">
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



export function AccountingPage() {
  const qc = useQueryClient();
  const [tab, setTab] = useState("ringkasan");
  const [expOpen, setExpOpen] = useState(false);
  const [exp, setExp] = useState({
    amount: "",
    category: "ops",
    description: "",
    expense_date: new Date().toISOString().slice(0, 10),
  });
  const [expErr, setExpErr] = useState("");

  const pnl = useQuery({
    queryKey: ["pnl"],
    queryFn: () =>
      api<{ revenue: number; expense: number; profit: number; period?: string }>("/api/accounting/pnl"),
  });
  const cf = useQuery({
    queryKey: ["cashflow"],
    queryFn: () => api<{ month: string; inflow: number; outflow: number; net: number }[]>("/api/accounting/cashflow?months=6"),
  });
  const business = useQuery({
    queryKey: ["business-report"],
    queryFn: () =>
      api<{
        mrr?: number;
        arpu?: number;
        unpaid_invoices?: number;
        active_customers?: number;
        aging?: { current?: number; d30?: number; d60?: number; d90?: number };
      }>("/api/reports/business"),
  });
  const churn = useQuery({
    queryKey: ["churn"],
    queryFn: () =>
      api<{ canceled_30d?: number; active?: number; churn_ratio?: number; window_days?: number }>("/api/reports/churn"),
  });
  const expensesQ = useQuery({
    queryKey: ["expenses"],
    queryFn: () =>
      api<{ data: { id: string; amount: number; category: string; description?: string | null; expense_date: string }[]; total: number }>(
        "/api/accounting/expenses?limit=50",
      ),
  });
  const accountsQ = useQuery({
    queryKey: ["accounts"],
    queryFn: () => api<{ id: string; code: string; name: string; type: string }[]>("/api/accounting/accounts"),
  });

  const createExpense = useMutation({
    mutationFn: () => {
      const amount = Math.floor(Number(exp.amount) || 0);
      if (amount <= 0) throw new Error("Nominal beban harus lebih dari 0");
      return api("/api/accounting/expenses", {
        method: "POST",
        body: JSON.stringify({
          amount,
          category: exp.category,
          description: exp.description.trim() || undefined,
          expense_date: exp.expense_date || undefined,
        }),
      });
    },
    onSuccess: () => {
      setExpOpen(false);
      setExp({
        amount: "",
        category: "ops",
        description: "",
        expense_date: new Date().toISOString().slice(0, 10),
      });
      setExpErr("");
      qc.invalidateQueries({ queryKey: ["pnl"] });
      qc.invalidateQueries({ queryKey: ["cashflow"] });
      qc.invalidateQueries({ queryKey: ["expenses"] });
      void toastSuccess("Beban dicatat");
    },
    onError: (e: Error) => {
      setExpErr(e.message);
      void toastError(e.message);
    },
  });

  const flow = Array.isArray(cf.data) ? cf.data : [];
  const accounts = Array.isArray(accountsQ.data) ? accountsQ.data : [];
  const expenses = expensesQ.data?.data ?? [];
  const aging = business.data?.aging ?? {};
  const typeLabel: Record<string, string> = {
    asset: "Aset",
    liability: "Liabilitas",
    equity: "Ekuitas",
    revenue: "Pendapatan",
    expense: "Beban",
  };
  const catLabel: Record<string, string> = {
    ops: "Operasional",
    gaji: "Gaji",
    sewa: "Sewa",
    marketing: "Marketing",
    lainnya: "Lainnya",
  };

  return (
    <Section
      title="Akunting & laporan"
      actions={
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() =>
              void apiDownload("/api/reports/invoices.csv", "invoices.csv").catch((e: Error) =>
                toastError(e.message || "Export gagal"),
              )
            }
          >
            <IconDownload /> CSV
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() =>
              void apiDownload("/api/reports/invoices.xlsx", "invoices.xlsx").catch((e: Error) =>
                toastError(e.message || "Export gagal"),
              )
            }
          >
            <IconDownload /> XLSX
          </Button>
          <Button type="button" onClick={() => setExpOpen(true)}>
            + Catat beban
          </Button>
        </div>
      }
    >
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList aria-label="Akunting" className="mb-5">
          <TabsTrigger value="ringkasan">Ringkasan</TabsTrigger>
          <TabsTrigger value="beban">Beban</TabsTrigger>
          <TabsTrigger value="coa">Bagan akun</TabsTrigger>
        </TabsList>

        <TabsContent value="ringkasan" className="space-y-6">
          <p className="text-sm text-[var(--muted)]">
            Periode P&amp;L: <span className="font-medium text-[var(--text)]">{pnl.data?.period || "—"}</span>
          </p>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Card title="Pendapatan bulan ini" value={formatRp(pnl.data?.revenue ?? 0)} />
            <Card title="Beban bulan ini" value={formatRp(pnl.data?.expense ?? 0)} />
            <Card title="Laba / rugi" value={formatRp(pnl.data?.profit ?? 0)} />
            <Card title="MRR" value={formatRp(Math.round(business.data?.mrr ?? 0))} />
          </div>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Card title="ARPU" value={formatRp(Math.round(business.data?.arpu ?? 0))} />
            <Card title="Tagihan belum lunas" value={business.data?.unpaid_invoices ?? 0} />
            <Card title="Pelanggan aktif" value={business.data?.active_customers ?? 0} />
            <Card
              title={`Churn ${churn.data?.window_days ?? 30}h`}
              value={`${(((churn.data?.churn_ratio ?? 0) as number) * 100).toFixed(1)}%`}
            />
          </div>

          <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div className="panel-card panel-card-pad lg:col-span-2">
              <div className="mb-4 flex items-center justify-between gap-2">
                <h3 className="eyebrow">Arus kas</h3>
                <span className="text-xs text-[var(--muted)]">6 bulan terakhir</span>
              </div>
              <ReactECharts
                style={{ height: 280 }}
                option={{
                  backgroundColor: "transparent",
                  textStyle: { color: "var(--chart-axis)", fontFamily: "Plus Jakarta Sans" },
                  legend: { data: ["Masuk", "Keluar"], textStyle: { color: "var(--muted)" } },
                  grid: { left: 48, right: 12, top: 36, bottom: 32 },
                  xAxis: {
                    type: "category",
                    data: flow.map((x) => x.month),
                    axisLine: { lineStyle: { color: "var(--border)" } },
                  },
                  yAxis: {
                    type: "value",
                    splitLine: { lineStyle: { color: "var(--chart-grid)" } },
                  },
                  series: [
                    {
                      name: "Masuk",
                      type: "bar",
                      data: flow.map((x) => x.inflow),
                      itemStyle: { color: "var(--chart-bar)", borderRadius: [4, 4, 0, 0] },
                      barMaxWidth: 18,
                    },
                    {
                      name: "Keluar",
                      type: "bar",
                      data: flow.map((x) => x.outflow),
                      itemStyle: { color: "var(--warn)", borderRadius: [4, 4, 0, 0] },
                      barMaxWidth: 18,
                    },
                  ],
                  tooltip: { trigger: "axis" },
                }}
              />
              <Table
                columns={["Bulan", "Masuk", "Keluar", "Net"]}
                rows={flow.map((r) => [r.month, formatRp(r.inflow), formatRp(r.outflow), formatRp(r.net)])}
              />
            </div>

            <div className="panel-card panel-card-pad space-y-4">
              <h3 className="eyebrow">Aging piutang</h3>
              <Table
                columns={["Bucket", "Nominal"]}
                rows={[
                  ["Current", formatRp(aging.current ?? 0)],
                  ["1–30 hari", formatRp(aging.d30 ?? 0)],
                  ["31–60 hari", formatRp(aging.d60 ?? 0)],
                  ["> 60 hari", formatRp(aging.d90 ?? 0)],
                ]}
              />
              <div className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/50 p-3 text-sm">
                <p className="font-medium text-[var(--text)]">Churn</p>
                <p className="mt-1 text-[var(--muted)]">
                  {churn.data?.canceled_30d ?? 0} batal / {churn.data?.active ?? 0} aktif (
                  {(((churn.data?.churn_ratio ?? 0) as number) * 100).toFixed(1)}%) dalam{" "}
                  {churn.data?.window_days ?? 30} hari.
                </p>
              </div>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="beban" className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-[var(--muted)]">{expensesQ.data?.total ?? 0} catatan beban</p>
            <Button type="button" onClick={() => setExpOpen(true)}>
              + Catat beban
            </Button>
          </div>
          <Table
            columns={["Tanggal", "Kategori", "Keterangan", "Nominal"]}
            rows={expenses.map((e) => [
              e.expense_date,
              catLabel[e.category] || e.category,
              e.description || "—",
              formatRp(e.amount),
            ])}
          />
        </TabsContent>

        <TabsContent value="coa" className="space-y-4">
          <p className="text-sm text-[var(--muted)]">
            Chart of accounts tenant. Digunakan untuk jurnal pembayaran (kas &amp; pendapatan).
          </p>
          <Table
            columns={["Kode", "Nama", "Tipe"]}
            rows={accounts.map((a) => [a.code, a.name, typeLabel[a.type] || a.type])}
          />
        </TabsContent>
      </Tabs>

      <FormDialog
        open={expOpen}
        title="Catat beban"
        onClose={() => {
          setExpOpen(false);
          setExpErr("");
        }}
      >
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            createExpense.mutate();
          }}
        >
          <div>
            <Label className="mb-1.5 block">Nominal (Rp)</Label>
            <Input
              type="number"
              min={1}
              step={1000}
              value={exp.amount}
              onChange={(e) => setExp({ ...exp, amount: e.target.value })}
              required
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Kategori</Label>
            <Select value={exp.category} onValueChange={(v) => setExp({ ...exp, category: v })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(catLabel).map(([k, label]) => (
                  <SelectItem key={k} value={k}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <Label className="mb-1.5 block">Tanggal</Label>
            <Input
              type="date"
              value={exp.expense_date}
              onChange={(e) => setExp({ ...exp, expense_date: e.target.value })}
              required
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Keterangan</Label>
            <Input
              placeholder="Opsional"
              value={exp.description}
              onChange={(e) => setExp({ ...exp, description: e.target.value })}
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={createExpense.isPending}>
              {createExpense.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setExpOpen(false)}>
              Batal
            </Button>
          </div>
          {expErr ? <p className="text-sm text-[var(--danger)]">{expErr}</p> : null}
        </form>
      </FormDialog>
    </Section>
  );
}

export function ResellersPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();

  type CommissionRow = {
    id: string;
    customer_name?: string;
    customer_code?: string;
    reseller_name?: string;
    sales_user_name?: string;
    amount: number;
    basis?: string;
    status: string;
    created_at: string;
    paid_at?: string | null;
  };

  const [commStatus, setCommStatus] = useState("");
  const [newAmountInput, setNewAmountInput] = useState("");
  const [acqAmountInput, setAcqAmountInput] = useState("");
  const [form, setForm] = useState({ name: "", phone: "", commission_percent: 0, is_active: true });
  const [editId, setEditId] = useState<string | null>(null);
  const [open, setOpen] = useState(false);

  const q = useQuery({
    queryKey: ["resellers"],
    queryFn: () => api<ResellerOpt[]>("/api/resellers"),
  });
  const settingsQ = useQuery({
    queryKey: ["commission-settings"],
    queryFn: () => api<{ new_customer_amount: number; acquisition_amount: number }>("/api/settings/commission"),
  });
  const commissionsQ = useQuery({
    queryKey: ["commissions", commStatus],
    queryFn: () =>
      api<{ data: CommissionRow[]; total: number }>(
        `/api/commissions?limit=50${commStatus ? `&status=${encodeURIComponent(commStatus)}` : ""}`,
      ),
  });

  useEffect(() => {
    if (settingsQ.data) {
      setNewAmountInput(String(settingsQ.data.new_customer_amount ?? 0));
      setAcqAmountInput(String(settingsQ.data.acquisition_amount ?? 0));
    }
  }, [settingsQ.data]);

  const saveSettings = useMutation({
    mutationFn: () =>
      api("/api/settings/commission", {
        method: "PUT",
        body: JSON.stringify({
          new_customer_amount: Math.max(0, Math.floor(Number(newAmountInput) || 0)),
          acquisition_amount: Math.max(0, Math.floor(Number(acqAmountInput) || 0)),
        }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commission-settings"] });
      void toastSuccess("Setting komisi disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const saveReseller = useMutation({
    mutationFn: () => {
      const body = {
        name: form.name.trim(),
        phone: form.phone.trim(),
        commission_percent: form.commission_percent,
        is_active: form.is_active,
      };
      if (editId) return api(`/api/resellers/${editId}`, { method: "PUT", body: JSON.stringify(body) });
      return api("/api/resellers", { method: "POST", body: JSON.stringify(body) });
    },
    onSuccess: () => {
      const wasEdit = Boolean(editId);
      setOpen(false);
      setEditId(null);
      setForm({ name: "", phone: "", commission_percent: 0, is_active: true });
      qc.invalidateQueries({ queryKey: ["resellers"] });
      void toastSuccess(wasEdit ? "Reseller diperbarui" : "Reseller ditambahkan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const removeReseller = useMutation({
    mutationFn: (id: string) => api(`/api/resellers/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["resellers"] });
      void toastSuccess("Reseller dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const markPaid = useMutation({
    mutationFn: (id: string) => api(`/api/commissions/${id}/paid`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commissions"] });
      void toastSuccess("Komisi ditandai dibayar");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const voidComm = useMutation({
    mutationFn: (id: string) => api(`/api/commissions/${id}/void`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commissions"] });
      qc.invalidateQueries({ queryKey: ["resellers"] });
      void toastSuccess("Komisi dibatalkan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const list = Array.isArray(q.data) ? q.data : [];
  const commissions = commissionsQ.data?.data ?? [];
  const statusLabel: Record<string, string> = { pending: "Pending", paid: "Dibayar", void: "Void" };

  return (
    <Section
      title="Reseller & Komisi"
      actions={
        <Button
          type="button"
          onClick={() => {
            setEditId(null);
            setForm({ name: "", phone: "", commission_percent: 0, is_active: true });
            setOpen(true);
          }}
        >
          + Reseller
        </Button>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Komisi flat per jenis (pelanggan baru / akuisisi). Saldo reseller naik saat entry dibuat; staff dilacak di riwayat
        saja.
      </p>

      <div className="panel-card mb-6 p-4">
        <h3 className="mb-2 text-sm font-semibold">Setting komisi</h3>
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            saveSettings.mutate();
          }}
        >
          <div className="min-w-[200px]">
            <Label className="mb-1.5 block">Pelanggan baru (Rp)</Label>
            <Input
              type="number"
              min={0}
              step={1000}
              value={newAmountInput}
              onChange={(e) => setNewAmountInput(e.target.value)}
              placeholder="0 = nonaktif"
            />
          </div>
          <div className="min-w-[200px]">
            <Label className="mb-1.5 block">Akuisisi (Rp)</Label>
            <Input
              type="number"
              min={0}
              step={1000}
              value={acqAmountInput}
              onChange={(e) => setAcqAmountInput(e.target.value)}
              placeholder="0 = nonaktif"
            />
          </div>
          <Button type="submit" disabled={saveSettings.isPending}>
            {saveSettings.isPending ? "Menyimpan…" : "Simpan"}
          </Button>
        </form>
        <p className="mt-2 text-xs text-[var(--muted)]">
          Baru: {formatRp(settingsQ.data?.new_customer_amount ?? 0)} · Akuisisi:{" "}
          {formatRp(settingsQ.data?.acquisition_amount ?? 0)}. Nilai 0 = tidak buat entry untuk jenis itu.
        </p>
      </div>

      <h3 className="mb-2 text-sm font-semibold">Reseller</h3>
      <Table
        columns={["Nama", "Telepon", "Saldo", "Aktif", "Aksi"]}
        rows={list.map((r) => [
          r.name,
          r.phone ?? "—",
          formatRp(r.balance ?? 0),
          r.is_active === false ? "nonaktif" : "aktif",
          <span key={r.id} className="flex flex-wrap items-center gap-1.5">
            <IconButton
              label="Edit reseller"
              onClick={() => {
                setEditId(r.id);
                setForm({
                  name: r.name,
                  phone: r.phone || "",
                  commission_percent: r.commission_percent ?? 0,
                  is_active: r.is_active !== false,
                });
                setOpen(true);
              }}
            >
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus reseller"
              danger
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus reseller",
                  description: `Hapus reseller "${r.name}"?`,
                  confirmLabel: "Hapus",
                });
                if (!ok) return;
                removeReseller.mutate(r.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />

      <div className="mt-8 mb-3 flex flex-wrap items-end justify-between gap-3">
        <h3 className="text-sm font-semibold">Riwayat komisi</h3>
        <div className="min-w-[160px]">
          <Label className="mb-1.5 block">Status</Label>
          <Select value={commStatus || "__all__"} onValueChange={(v) => setCommStatus(v === "__all__" ? "" : v)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all__">Semua</SelectItem>
              <SelectItem value="pending">Pending</SelectItem>
              <SelectItem value="paid">Dibayar</SelectItem>
              <SelectItem value="void">Void</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      <Table
        columns={["Tanggal", "Pelanggan", "Jenis", "Penerima", "Nominal", "Status", "Aksi"]}
        rows={commissions.map((c) => [
          c.created_at ? new Date(c.created_at).toLocaleString("id-ID") : "—",
          c.customer_code ? `${c.customer_code} · ${c.customer_name || ""}` : c.customer_name || "—",
          commissionBasisLabel(c.basis || ""),
          c.reseller_name ? `Reseller: ${c.reseller_name}` : c.sales_user_name ? `Staff: ${c.sales_user_name}` : "—",
          formatRp(c.amount),
          statusLabel[c.status] || c.status,
          <span key={c.id} className="flex flex-wrap items-center gap-1.5">
            {c.status === "pending" ? (
              <IconButton label="Tandai dibayar" onClick={() => markPaid.mutate(c.id)}>
                <IconCheck />
              </IconButton>
            ) : null}
            {c.status === "pending" || c.status === "paid" ? (
              <IconButton
                label="Batalkan komisi"
                danger
                onClick={async () => {
                  const ok = await confirm({
                    title: "Void komisi",
                    description: "Batalkan entry komisi ini? Saldo reseller akan dikurangi jika ada.",
                    confirmLabel: "Void",
                  });
                  if (!ok) return;
                  voidComm.mutate(c.id);
                }}
              >
                <IconBan />
              </IconButton>
            ) : null}
          </span>,
        ])}
      />

      <FormDialog
        open={open}
        title={editId ? "Edit reseller" : "Tambah reseller"}
        onClose={() => {
          setOpen(false);
          setEditId(null);
        }}
      >
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            saveReseller.mutate();
          }}
        >
          <Input
            placeholder="Nama"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            required
          />
          <Input
            placeholder="Telepon"
            value={form.phone}
            onChange={(e) => setForm({ ...form, phone: e.target.value })}
          />
          {editId ? (
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={form.is_active}
                onChange={(e) => setForm({ ...form, is_active: e.target.checked })}
              />
              Aktif
            </label>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={saveReseller.isPending}>
              {saveReseller.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>
              Batal
            </Button>
          </div>
        </form>
      </FormDialog>
    </Section>
  );
}

export function TechPage() {
  const [subject, setSubject] = useState("Instalasi");
  const mut = useMutation({
    mutationFn: () => api("/api/work-orders", { method: "POST", body: JSON.stringify({ type: "installation", notes: subject }) }),
  });
  const checkin = useMutation({
    mutationFn: (id: string) =>
      api(`/api/work-orders/${id}/check-in`, {
        method: "POST",
        body: JSON.stringify({ lat: -6.2, lng: 106.8 }),
      }),
  });
  return (
    <Section title="Portal teknisi">
      <p className="mb-3 text-sm text-[var(--muted)]">Buat work order dan check-in GPS (koordinat contoh; browser geolocation bisa ditambahkan). Upload foto via endpoint terpisah.</p>
      <form
        className="mb-3 flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          mut.mutate();
        }}
      >
        <input className="input" value={subject} onChange={(e) => setSubject(e.target.value)} />
        <button className="btn">Buat WO</button>
      </form>
      <button
        className="btn-ghost"
        type="button"
        onClick={() => {
          const id = window.prompt("UUID work order untuk check-in");
          if (id) checkin.mutate(id.trim());
        }}
      >
        Check-in WO
      </button>
      {mut.isSuccess && <p className="mt-2 text-sm text-[var(--ok)]">Work order dibuat.</p>}
    </Section>
  );
}

export function InvoiceActions({
  id,
  invoiceNumber,
  status,
  onDone,
}: {
  id: string;
  invoiceNumber: string;
  status: string;
  onDone?: () => void;
}) {
  const { confirm } = useAppDialog();
  const unpaid = status !== "paid" && status !== "void" && status !== "cancelled";
  const pay = useMutation({
    mutationFn: () => api(`/api/invoices/${id}/pay`, { method: "POST", body: JSON.stringify({ method: "manual" }) }),
    onSuccess: () => {
      void toastSuccess("Pembayaran dicatat");
      onDone?.();
    },
    onError: (e: Error) => void toastError(e.message || "Gagal bayar"),
  });
  const [pdfBusy, setPdfBusy] = useState(false);

  return (
    <span className="flex flex-wrap items-center gap-1.5">
      {unpaid && (
        <IconButton
          label="Bayar manual"
          disabled={pay.isPending}
          onClick={() => {
            void confirm({
              title: "Catat pembayaran?",
              description: `Tandai lunas invoice ${invoiceNumber} (pembayaran manual)?`,
              confirmLabel: "Bayar",
            }).then((ok) => {
              if (ok) pay.mutate();
            });
          }}
        >
          <IconBanknote />
        </IconButton>
      )}
      <IconButton
        label="Unduh PDF"
        disabled={pdfBusy}
        onClick={() => {
          setPdfBusy(true);
          void apiDownload(`/api/invoices/${id}/pdf`, `${invoiceNumber || id}.pdf`)
            .catch((e: Error) => toastError(e.message || "PDF gagal"))
            .finally(() => setPdfBusy(false));
        }}
      >
        <IconDownload />
      </IconButton>
    </span>
  );
}
