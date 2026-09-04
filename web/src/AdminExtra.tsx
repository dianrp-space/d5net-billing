import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, getToken } from "./api";
import { useAppDialog } from "./confirm";
import { IconEye, IconPencil, IconTrash } from "./icons";
import { toastError, toastSuccess } from "./swal";
import {
  Button,
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
            <IconButton label="Lihat assignment" onClick={() => setAssignPool(p)}>
              <IconEye />
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

export function LeadsPage() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["leads"],
    queryFn: () => api<{ name: string; phone?: string; status?: string }[]>("/api/leads"),
  });
  const [form, setForm] = useState({ name: "", phone: "", address: "" });
  const [open, setOpen] = useState(false);
  const mut = useMutation({
    mutationFn: () => api("/api/leads", { method: "POST", body: JSON.stringify(form) }),
    onSuccess: () => {
      setForm({ name: "", phone: "", address: "" });
      setOpen(false);
      qc.invalidateQueries({ queryKey: ["leads"] });
    },
  });
  const raw = q.data;
  const list = Array.isArray(raw) ? raw : [];
  return (
    <Section
      title="Lead / pipeline"
      actions={
        <button type="button" className="btn" onClick={() => setOpen(true)}>
          + Tambah
        </button>
      }
    >
      <Table columns={["Nama", "Telepon", "Status"]} rows={(list as { name: string; phone?: string; status?: string }[]).map((l) => [l.name, l.phone ?? "—", l.status ?? "new"])} />
      <FormDialog open={open} title="Tambah lead" onClose={() => setOpen(false)}>
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            mut.mutate();
          }}
        >
          <input className="input" placeholder="Nama" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          <input className="input" placeholder="Telepon" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} />
          <input className="input" placeholder="Alamat" value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} />
          <div className="flex flex-wrap gap-2">
            <button className="btn" disabled={mut.isPending}>
              {mut.isPending ? "Menyimpan…" : "Simpan"}
            </button>
            <button type="button" className="btn-ghost" onClick={() => setOpen(false)}>
              Batal
            </button>
          </div>
        </form>
      </FormDialog>
    </Section>
  );
}

export function AccountingPage() {
  const pnl = useQuery({ queryKey: ["pnl"], queryFn: () => api<{ revenue: number; expense: number; profit: number }>("/api/accounting/pnl") });
  const cf = useQuery({ queryKey: ["cashflow"], queryFn: () => api<{ month: string; inflow: number; outflow: number; net: number }[]>("/api/accounting/cashflow?months=6") });
  const churn = useQuery({ queryKey: ["churn"], queryFn: () => api<Record<string, number>>("/api/reports/churn") });
  const [exp, setExp] = useState({ amount: 0, category: "ops", description: "" });
  const mut = useMutation({
    mutationFn: () => api("/api/accounting/expenses", { method: "POST", body: JSON.stringify(exp) }),
    onSuccess: () => {
      pnl.refetch();
      cf.refetch();
    },
  });
  const flow = Array.isArray(cf.data) ? cf.data : [];
  return (
    <Section title="Akunting & laporan">
      <p className="mb-3 text-sm text-[var(--muted)]">
        Laba bulan ini {formatRp(pnl.data?.profit ?? 0)} · pendapatan {formatRp(pnl.data?.revenue ?? 0)} · beban {formatRp(pnl.data?.expense ?? 0)}
      </p>
      <form
        className="mb-4 flex flex-wrap gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          mut.mutate();
        }}
      >
        <input className="input" type="number" placeholder="Beban (Rp)" value={exp.amount || ""} onChange={(e) => setExp({ ...exp, amount: Number(e.target.value) })} />
        <input className="input" placeholder="Kategori" value={exp.category} onChange={(e) => setExp({ ...exp, category: e.target.value })} />
        <input className="input" placeholder="Keterangan" value={exp.description} onChange={(e) => setExp({ ...exp, description: e.target.value })} />
        <button className="btn">Catat beban</button>
      </form>
      <div className="mb-3 flex gap-2">
        <a className="btn-ghost" href="/api/reports/invoices.csv">CSV</a>
        <a className="btn-ghost" href="/api/reports/invoices.xlsx">XLSX</a>
      </div>
      <Table columns={["Bulan", "Masuk", "Keluar", "Net"]} rows={flow.map((r) => [r.month, formatRp(r.inflow), formatRp(r.outflow), formatRp(r.net)])} />
      {churn.data && <p className="mt-3 text-sm text-[var(--muted)]">Churn: {JSON.stringify(churn.data)}</p>}
    </Section>
  );
}

export function ResellersPage() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["resellers"],
    queryFn: () => api<{ name: string; phone?: string; commission_percent?: number }[]>("/api/resellers"),
  });
  const [form, setForm] = useState({ name: "", phone: "", commission_percent: 10 });
  const [open, setOpen] = useState(false);
  const mut = useMutation({
    mutationFn: () => api("/api/resellers", { method: "POST", body: JSON.stringify(form) }),
    onSuccess: () => {
      setForm({ name: "", phone: "", commission_percent: 10 });
      setOpen(false);
      qc.invalidateQueries({ queryKey: ["resellers"] });
    },
  });
  const list = Array.isArray(q.data) ? q.data : [];
  return (
    <Section
      title="Reseller / agen"
      actions={
        <button type="button" className="btn" onClick={() => setOpen(true)}>
          + Tambah
        </button>
      }
    >
      <Table columns={["Nama", "Telepon", "Komisi %"]} rows={(list as { name: string; phone?: string; commission_percent?: number }[]).map((r) => [r.name, r.phone ?? "—", r.commission_percent ?? 0])} />
      <FormDialog open={open} title="Tambah reseller" onClose={() => setOpen(false)}>
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            mut.mutate();
          }}
        >
          <input className="input" placeholder="Nama" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          <input className="input" placeholder="Telepon" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} />
          <input className="input" type="number" placeholder="Komisi %" value={form.commission_percent} onChange={(e) => setForm({ ...form, commission_percent: Number(e.target.value) })} />
          <div className="flex flex-wrap gap-2">
            <button className="btn" disabled={mut.isPending}>
              {mut.isPending ? "Menyimpan…" : "Simpan"}
            </button>
            <button type="button" className="btn-ghost" onClick={() => setOpen(false)}>
              Batal
            </button>
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

export function InvoiceActions({ id, onDone }: { id: string; onDone?: () => void }) {
  const pay = useMutation({
    mutationFn: () => api(`/api/invoices/${id}/pay`, { method: "POST", body: JSON.stringify({ method: "manual" }) }),
    onSuccess: () => onDone?.(),
  });
  return (
    <div className="flex gap-1">
      <button type="button" className="btn-ghost" onClick={() => pay.mutate()} disabled={pay.isPending}>
        Bayar
      </button>
      <a className="btn-ghost" href={`/api/invoices/${id}/pdf`}>
        PDF
      </a>
    </div>
  );
}
