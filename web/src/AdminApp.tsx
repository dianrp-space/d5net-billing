import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import ReactECharts from "echarts-for-react";
import { api, clearToken, getToken } from "./api";
import {
  IconBell,
  IconBox,
  IconChart,
  IconDownload,
  IconEye,
  IconHome,
  IconImage,
  IconLogout,
  IconMapPin,
  IconPencil,
  IconPlug,
  IconReceipt,
  IconRouter,
  IconSettings,
  IconShield,
  IconTicket,
  IconTrash,
  IconUpload,
  IconUsers,
} from "./icons";
import { useAppDialog } from "./confirm";
import { toastError, toastSuccess } from "./swal";
import { Card, formatRp, FormDialog, IconButton, Input, OnlineBadge, Section, SecretInput, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, StatusDialog, Table, Button } from "./ui";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ThemeToggle } from "./ThemeToggle";
import { AccountingPage, AlertsPanel, IPAMPage, InvoiceActions, LeadsPage, ResellersPage, TechPage } from "./AdminExtra";
import { MapODP } from "./FtthMap";
import { HeaderSearch } from "./HeaderSearch";
import { getLastOdpCluster, setLastOdpCluster } from "./navPersist";
import { BrandingSettingsPage, RolesSettingsPage, UsersSettingsPage } from "./SettingsPages";
import { MessagingGWPage, PaymentGWPage, WebhooksIntegrationPage } from "./IntegrationPages";
import { BackupRestorePage } from "./BackupRestorePage";
import { applyBrandingMeta } from "./branding";

type Page =
  | "dashboard"
  | "customers"
  | "clusters"
  | "plans"
  | "subscriptions"
  | "invoices"
  | "routers"
  | "ipam"
  | "tickets"
  | "odp"
  | "vouchers"
  | "leads"
  | "accounting"
  | "resellers"
  | "tech"
  | "branding"
  | "roles"
  | "users"
  | "webhooks"
  | "payment-gw"
  | "messaging-gw"
  | "backup";

export type AdminPage = Page;

const ADMIN_PAGES: Page[] = [
  "dashboard",
  "customers",
  "clusters",
  "plans",
  "subscriptions",
  "invoices",
  "routers",
  "ipam",
  "tickets",
  "odp",
  "vouchers",
  "leads",
  "accounting",
  "resellers",
  "tech",
  "branding",
  "roles",
  "users",
  "webhooks",
  "payment-gw",
  "messaging-gw",
  "backup",
];

export function isAdminPage(value: string): value is Page {
  return (ADMIN_PAGES as string[]).includes(value);
}
type NavItem = { id: Page; label: string; icon: ReactNode };
type NavGroup = { label: string; items: NavItem[] };

const navGroups: NavGroup[] = [
  {
    label: "Main Menu",
    items: [
      { id: "dashboard", label: "Dashboard", icon: <IconHome /> },
      { id: "plans", label: "Paket", icon: <IconBox /> },
      { id: "subscriptions", label: "Langganan", icon: <IconReceipt /> },
      { id: "invoices", label: "Tagihan", icon: <IconChart /> },
      { id: "accounting", label: "Akunting", icon: <IconChart /> },
    ],
  },
  {
    label: "Customers",
    items: [
      { id: "customers", label: "Pelanggan", icon: <IconUsers /> },
      { id: "leads", label: "Lead", icon: <IconUsers /> },
      { id: "resellers", label: "Reseller", icon: <IconUsers /> },
    ],
  },
  {
    label: "Network",
    items: [
      { id: "clusters", label: "Cluster / POP", icon: <IconMapPin /> },
      { id: "routers", label: "Router", icon: <IconRouter /> },
      { id: "ipam", label: "IP Pool", icon: <IconRouter /> },
      { id: "odp", label: "ODP / FTTH", icon: <IconMapPin /> },
      { id: "vouchers", label: "Voucher", icon: <IconTicket /> },
    ],
  },
  {
    label: "Ops",
    items: [
      { id: "tickets", label: "Tiket", icon: <IconTicket /> },
      { id: "tech", label: "Teknisi", icon: <IconMapPin /> },
    ],
  },
  {
    label: "Integrasi",
    items: [
      { id: "webhooks", label: "Webhook", icon: <IconPlug /> },
      { id: "payment-gw", label: "Payment Gateway", icon: <IconReceipt /> },
      { id: "messaging-gw", label: "Messaging Gateway", icon: <IconBell /> },
    ],
  },
  {
    label: "Settings",
    items: [
      { id: "branding", label: "Branding", icon: <IconImage /> },
      { id: "roles", label: "Roles", icon: <IconShield /> },
      { id: "users", label: "Users", icon: <IconSettings /> },
      { id: "backup", label: "Backup / Restore", icon: <IconDownload /> },
    ],
  },
];

const pageTitles: Record<Page, string> = {
  dashboard: "Overview",
  customers: "Pelanggan",
  clusters: "Cluster / POP",
  plans: "Paket",
  subscriptions: "Langganan",
  invoices: "Tagihan",
  routers: "Router",
  ipam: "IP Pool",
  tickets: "Tiket",
  odp: "ODP / FTTH",
  vouchers: "Voucher",
  leads: "Lead",
  accounting: "Akunting",
  resellers: "Reseller",
  tech: "Teknisi",
  branding: "Branding",
  roles: "Roles",
  users: "Users",
  webhooks: "Webhook",
  "payment-gw": "Payment Gateway",
  "messaging-gw": "Messaging Gateway",
  backup: "Backup / Restore",
};

export function AdminApp({
  tenantSlug,
  page,
  onNavigate,
  onLogout,
}: {
  tenantSlug?: string;
  page: Page;
  onNavigate: (page: Page) => void;
  onLogout: () => void;
}) {
  const branding = useQuery({
    queryKey: ["public-tenant-branding", tenantSlug],
    queryFn: () =>
      api<{
        app_name?: string;
        logo_url?: string | null;
        favicon_url?: string | null;
        name?: string;
      }>(`/api/public/tenants/${encodeURIComponent(tenantSlug || "")}`),
    enabled: Boolean(tenantSlug),
    retry: false,
  });
  const appName = branding.data?.app_name || branding.data?.name || tenantSlug || "drp-billing";
  const logoUrl = branding.data?.logo_url;
  const faviconUrl = branding.data?.favicon_url;
  const initial = (appName.trim()[0] || "D").toUpperCase();

  useEffect(() => {
    applyBrandingMeta({
      appName,
      faviconUrl,
      titleSuffix: pageTitles[page],
    });
  }, [appName, faviconUrl, page]);

  return (
    <div className="app-shell">
      <aside className="app-sidebar">
        <div className="app-sidebar-brand">
          {logoUrl ? (
            <img src={logoUrl} alt="" className="app-sidebar-logo object-contain" />
          ) : (
            <div className="app-sidebar-logo">{initial}</div>
          )}
          <div>
            <p className="text-[10px] font-medium text-[var(--stone)]">ISP Billing</p>
            <h1 className="text-sm font-bold">{appName}</h1>
          </div>
        </div>

        <nav className="flex-1 px-3 py-2">
          {navGroups.map((g) => (
            <div key={g.label} className="app-nav-group">
              <p className="app-nav-label">{g.label}</p>
              <div className="space-y-0.5">
                {g.items.map((n) => (
                  <button
                    key={n.id}
                    type="button"
                    onClick={() => onNavigate(n.id)}
                    className={`app-nav-item ${page === n.id ? "is-active" : ""}`}
                  >
                    <span className="opacity-80">{n.icon}</span>
                    {n.label}
                  </button>
                ))}
              </div>
            </div>
          ))}
        </nav>

        <div className="app-sidebar-foot">
          <div className="app-user-chip">
            <div className="app-user-avatar">AD</div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-xs font-bold">Admin</p>
              <p className="truncate text-[10px] text-[var(--muted)]">Tenant operator</p>
            </div>
            <IconButton
              label="Keluar"
              onClick={() => {
                clearToken();
                onLogout();
              }}
            >
              <IconLogout />
            </IconButton>
          </div>
        </div>
      </aside>

      <div className="app-main">
        <header className="app-header">
          <div className="flex items-center gap-2 text-sm text-[var(--stone)]">
            <span>Dashboard</span>
            <span className="text-[10px]">›</span>
            <span className="font-semibold text-[var(--text)]">{pageTitles[page]}</span>
          </div>
          <div className="flex items-center gap-3">
            <HeaderSearch
              onNavigate={(next) => {
                if (isAdminPage(next)) onNavigate(next);
              }}
            />
            <IconButton label="Notifikasi">
              <IconBell />
            </IconButton>
            <ThemeToggle />
          </div>
        </header>

        <main className="app-content">
          {page === "dashboard" && <Dashboard tenantSlug={tenantSlug} />}
          {page === "customers" && <Customers />}
          {page === "clusters" && <Clusters />}
          {page === "plans" && <Plans />}
          {page === "subscriptions" && <Subscriptions />}
          {page === "invoices" && <Invoices />}
          {page === "routers" && <Routers />}
          {page === "ipam" && <IPAMPage />}
          {page === "tickets" && <Tickets />}
          {page === "odp" && <ODP tenantSlug={tenantSlug} />}
          {page === "vouchers" && <Vouchers />}
          {page === "leads" && <LeadsPage />}
          {page === "accounting" && <AccountingPage />}
          {page === "resellers" && <ResellersPage />}
          {page === "tech" && <TechPage />}
          {page === "branding" && <BrandingSettingsPage />}
          {page === "roles" && <RolesSettingsPage />}
          {page === "users" && <UsersSettingsPage />}
          {page === "webhooks" && <WebhooksIntegrationPage />}
          {page === "payment-gw" && <PaymentGWPage />}
          {page === "messaging-gw" && <MessagingGWPage />}
          {page === "backup" && <BackupRestorePage />}
        </main>
      </div>
    </div>
  );
}

function Dashboard({ tenantSlug }: { tenantSlug?: string }) {
  const stats = useQuery({
    queryKey: ["stats"],
    queryFn: () => api<Record<string, number>>("/api/dashboard/stats"),
  });
  const chart = useQuery({
    queryKey: ["revenue"],
    queryFn: () => api<{ month: string; revenue: number }[]>("/api/dashboard/revenue-chart?months=6"),
  });
  const invoices = useQuery({
    queryKey: ["invoices-recent"],
    queryFn: () =>
      api<{ data: { invoice_number: string; customer_name: string; total_amount: number; status: string }[] }>(
        "/api/invoices?limit=8",
      ),
  });
  const s = stats.data ?? {};
  const series = Array.isArray(chart.data) ? chart.data : [];
  const today = new Date().toLocaleDateString("id-ID", { day: "numeric", month: "short", year: "numeric" });

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-2xl font-bold tracking-tight">
          Selamat datang{tenantSlug ? `, ${tenantSlug}` : ""}
        </h2>
        <div className="flex flex-wrap items-center gap-2">
          <span className="btn-ghost text-sm">{today}</span>
          <a className="btn" href="/api/reports/invoices.csv">
            Export CSV
          </a>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card title="Pelanggan aktif" value={s.active_customers ?? 0} />
        <Card title="Langganan aktif" value={s.active_subscriptions ?? 0} />
        <Card title="Tagihan belum lunas" value={s.unpaid_invoices ?? 0} />
        <Card title="Pendapatan bulan ini" value={formatRp(s.monthly_revenue ?? 0)} />
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="panel-card panel-card-pad lg:col-span-2">
          <div className="mb-4 flex items-center justify-between gap-2">
            <h3 className="eyebrow">Tren pendapatan</h3>
            <span className="text-xs text-[var(--muted)]">6 bulan terakhir</span>
          </div>
          <p className="mb-4 text-xs text-[var(--muted)]">
            Total bulan ini: <span className="text-xl font-bold text-[var(--text)]">{formatRp(s.monthly_revenue ?? 0)}</span>
          </p>
          <ReactECharts
            style={{ height: 280 }}
            option={{
              backgroundColor: "transparent",
              textStyle: { color: "var(--chart-axis)", fontFamily: "Plus Jakarta Sans" },
              grid: { left: 48, right: 12, top: 16, bottom: 32 },
              xAxis: {
                type: "category",
                data: series.map((x) => x.month),
                axisLine: { lineStyle: { color: "var(--border)" } },
              },
              yAxis: {
                type: "value",
                splitLine: { lineStyle: { color: "var(--chart-grid)" } },
              },
              series: [
                {
                  type: "bar",
                  data: series.map((x) => x.revenue),
                  itemStyle: { color: "var(--chart-bar)", borderRadius: [4, 4, 0, 0] },
                  barMaxWidth: 28,
                },
              ],
              tooltip: { trigger: "axis" },
            }}
          />
        </div>

        <div className="panel-card panel-card-pad flex flex-col">
          <h3 className="eyebrow mb-4">Alert & monitoring</h3>
          <div className="flex-1 overflow-auto">
            <AlertsPanel />
          </div>
        </div>
      </div>

      <div className="panel-card overflow-hidden">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border)] px-6 py-4">
          <h3 className="eyebrow">Tagihan terbaru</h3>
        </div>
        <Table
          columns={["Nomor", "Pelanggan", "Total", "Status"]}
          rows={(invoices.data?.data ?? []).map((i) => [
            i.invoice_number,
            i.customer_name,
            formatRp(i.total_amount),
            <StatusPill key={i.invoice_number} status={i.status} />,
          ])}
        />
      </div>
    </div>
  );
}

function StatusPill({ status }: { status: string }) {
  const tone =
    status === "paid" || status === "success"
      ? "bg-[rgba(43,154,102,0.1)] text-[var(--ok)]"
      : status === "overdue" || status === "canceled"
        ? "bg-[rgba(220,38,38,0.1)] text-[var(--danger)]"
        : "bg-[rgba(245,158,11,0.12)] text-[var(--warn)]";
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-1 text-[10px] font-bold ${tone}`}>
      {status}
    </span>
  );
}

function Customers() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  type CustomerRow = {
    id: string;
    customer_code: string;
    full_name: string;
    phone: string;
    email?: string | null;
    address?: string | null;
    latitude?: number | null;
    longitude?: number | null;
    is_active: boolean;
    portal_enabled?: boolean;
    cluster_id?: string | null;
    cluster_name?: string;
    cluster_code?: string;
  };
  type ClusterOpt = { id: string; name: string; code: string; customer_code_prefix: string };
  type CustForm = {
    full_name: string;
    phone: string;
    email: string;
    address: string;
    cluster_id: string;
    customer_code: string;
    latitude: string;
    longitude: string;
    is_active: boolean;
    portal_enabled: boolean;
  };
  const emptyForm: CustForm = {
    full_name: "",
    phone: "",
    email: "",
    address: "",
    cluster_id: "",
    customer_code: "",
    latitude: "",
    longitude: "",
    is_active: true,
    portal_enabled: true,
  };
  const q = useQuery({
    queryKey: ["customers"],
    queryFn: () => api<{ data: CustomerRow[] }>("/api/customers?limit=100"),
  });
  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterOpt[]>("/api/clusters"),
  });
  const [form, setForm] = useState<CustForm>(emptyForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [formErr, setFormErr] = useState("");

  const previewQ = useQuery({
    queryKey: ["cluster-preview", form.cluster_id],
    queryFn: () => api<{ preview: string }>(`/api/clusters/${form.cluster_id}/next-customer-code`),
    enabled: createOpen && !editId && Boolean(form.cluster_id) && !form.customer_code.trim(),
  });

  const buildBody = () => {
    const body: Record<string, unknown> = {
      full_name: form.full_name,
      phone: form.phone,
      is_active: form.is_active,
      portal_enabled: form.portal_enabled,
    };
    if (form.email.trim()) body.email = form.email.trim();
    else body.email = null;
    if (form.address.trim()) body.address = form.address.trim();
    else body.address = null;
    if (form.cluster_id) body.cluster_id = form.cluster_id;
    else body.cluster_id = null;
    if (form.customer_code.trim()) body.customer_code = form.customer_code.trim();
    if (form.latitude.trim()) body.latitude = Number(form.latitude);
    else body.latitude = null;
    if (form.longitude.trim()) body.longitude = Number(form.longitude);
    else body.longitude = null;
    return body;
  };

  const create = useMutation({
    mutationFn: () => api("/api/customers", { method: "POST", body: JSON.stringify(buildBody()) }),
    onSuccess: () => {
      setForm({ ...emptyForm, cluster_id: form.cluster_id });
      setCreateOpen(false);
      setFormErr("");
      qc.invalidateQueries({ queryKey: ["customers"] });
      qc.invalidateQueries({ queryKey: ["ftth-map"] });
      qc.invalidateQueries({ queryKey: ["cluster-preview"] });
      void toastSuccess("Pelanggan ditambahkan");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const update = useMutation({
    mutationFn: () => {
      const body = buildBody();
      if (!body.customer_code) throw new Error("kode pelanggan wajib saat edit");
      return api(`/api/customers/${editId}`, { method: "PUT", body: JSON.stringify(body) });
    },
    onSuccess: () => {
      setEditId(null);
      setForm(emptyForm);
      setFormErr("");
      qc.invalidateQueries({ queryKey: ["customers"] });
      qc.invalidateQueries({ queryKey: ["ftth-map"] });
      void toastSuccess("Pelanggan diperbarui");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/customers/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      if (editId === remove.variables) {
        setEditId(null);
        setForm(emptyForm);
      }
      qc.invalidateQueries({ queryKey: ["customers"] });
      qc.invalidateQueries({ queryKey: ["ftth-map"] });
      void toastSuccess("Pelanggan dihapus");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  function startEdit(c: CustomerRow) {
    setCreateOpen(false);
    setEditId(c.id);
    setFormErr("");
    setForm({
      full_name: c.full_name,
      phone: c.phone,
      email: c.email || "",
      address: c.address || "",
      cluster_id: c.cluster_id || "",
      customer_code: c.customer_code,
      latitude: c.latitude != null ? String(c.latitude) : "",
      longitude: c.longitude != null ? String(c.longitude) : "",
      is_active: c.is_active,
      portal_enabled: c.portal_enabled ?? true,
    });
  }

  function closeForm() {
    setEditId(null);
    setCreateOpen(false);
    setForm(emptyForm);
    setFormErr("");
  }

  const clusters = Array.isArray(clustersQ.data) ? clustersQ.data : [];
  const dialogOpen = createOpen || Boolean(editId);
  const saving = create.isPending || update.isPending;

  return (
    <Section
      title="Pelanggan"
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
      {formErr && !dialogOpen && <p className="mb-3 text-sm text-[var(--danger)]">{formErr}</p>}
      <Table
        columns={["Kode", "Cluster", "Nama", "Telepon", "Koordinat", "Status", "Aksi"]}
        rows={(q.data?.data ?? []).map((c) => [
          c.customer_code,
          c.cluster_name || c.cluster_code || "—",
          c.full_name,
          c.phone,
          c.latitude != null && c.longitude != null ? `${c.latitude.toFixed(5)}, ${c.longitude.toFixed(5)}` : "—",
          c.is_active ? "aktif" : "nonaktif",
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton label="Edit pelanggan" onClick={() => startEdit(c)}>
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus pelanggan"
              danger
              disabled={remove.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus pelanggan",
                  description: `Hapus pelanggan "${c.full_name}"?`,
                  confirmLabel: "Hapus",
                });
                if (!ok) return;
                remove.mutate(c.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />

      <FormDialog open={dialogOpen} wide title={editId ? "Edit pelanggan" : "Tambah pelanggan"} onClose={closeForm}>
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
                {c.name} ({c.customer_code_prefix || c.code})
              </option>
            ))}
          </select>
          <input className="input" placeholder="Nama" value={form.full_name} onChange={(e) => setForm({ ...form, full_name: e.target.value })} required />
          <input className="input" placeholder="Telepon" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} required />
          <input className="input" placeholder="Email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
          <input
            className="input"
            placeholder={previewQ.data?.preview ? `Kode (kosong = ${previewQ.data.preview})` : "Kode pelanggan"}
            value={form.customer_code}
            onChange={(e) => setForm({ ...form, customer_code: e.target.value })}
            required={Boolean(editId)}
          />
          <input className="input sm:col-span-2" placeholder="Alamat" value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} />
          <input className="input" type="number" step="any" placeholder="Latitude (peta)" value={form.latitude} onChange={(e) => setForm({ ...form, latitude: e.target.value })} />
          <input className="input" type="number" step="any" placeholder="Longitude (peta)" value={form.longitude} onChange={(e) => setForm({ ...form, longitude: e.target.value })} />
          {form.cluster_id && !form.customer_code.trim() && !editId && previewQ.data?.preview && (
            <p className="text-xs text-[var(--muted)] sm:col-span-2">
              Kode otomatis: <span className="font-semibold text-[var(--text)]">{previewQ.data.preview}</span>
            </p>
          )}
          {editId && (
            <>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={form.is_active} onChange={(e) => setForm({ ...form, is_active: e.target.checked })} />
                Aktif
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={form.portal_enabled} onChange={(e) => setForm({ ...form, portal_enabled: e.target.checked })} />
                Portal aktif
              </label>
              <p className="text-xs text-[var(--muted)] sm:col-span-2">Password portal default = nomor HP (ikut berubah jika HP diubah).</p>
            </>
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

function Clusters() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  type ClusterRow = {
    id: string;
    name: string;
    code: string;
    customer_code_prefix: string;
    customer_code_pattern: string;
    seq_width: number;
    address?: string | null;
    latitude?: number | null;
    longitude?: number | null;
    notes?: string | null;
    is_active: boolean;
  };
  type ClusterForm = {
    name: string;
    code: string;
    customer_code_prefix: string;
    customer_code_pattern: string;
    seq_width: number;
    address: string;
    latitude: string;
    longitude: string;
    notes: string;
    is_active: boolean;
  };
  const emptyForm: ClusterForm = {
    name: "",
    code: "",
    customer_code_prefix: "",
    customer_code_pattern: "{prefix}-{yyyymm}-{seq}",
    seq_width: 4,
    address: "",
    latitude: "",
    longitude: "",
    notes: "",
    is_active: true,
  };
  const q = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterRow[]>("/api/clusters"),
  });
  const [form, setForm] = useState<ClusterForm>(emptyForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [formErr, setFormErr] = useState("");

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["clusters"] });
    qc.invalidateQueries({ queryKey: ["ftth-map"] });
  };

  function coordsBody() {
    return {
      latitude: form.latitude.trim() ? Number(form.latitude) : null,
      longitude: form.longitude.trim() ? Number(form.longitude) : null,
    };
  }

  const create = useMutation({
    mutationFn: () =>
      api("/api/clusters", {
        method: "POST",
        body: JSON.stringify({
          name: form.name,
          code: form.code,
          customer_code_prefix: form.customer_code_prefix || undefined,
          customer_code_pattern: form.customer_code_pattern || undefined,
          seq_width: form.seq_width,
          address: form.address.trim() || undefined,
          notes: form.notes.trim() || undefined,
          ...coordsBody(),
        }),
      }),
    onSuccess: () => {
      setForm(emptyForm);
      setFormErr("");
      setCreateOpen(false);
      refresh();
      void toastSuccess("Cluster ditambahkan");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const update = useMutation({
    mutationFn: () =>
      api(`/api/clusters/${editId}`, {
        method: "PUT",
        body: JSON.stringify({
          name: form.name,
          code: form.code,
          customer_code_prefix: form.customer_code_prefix,
          customer_code_pattern: form.customer_code_pattern,
          seq_width: form.seq_width,
          address: form.address.trim() || null,
          notes: form.notes.trim() || null,
          is_active: form.is_active,
          ...coordsBody(),
        }),
      }),
    onSuccess: () => {
      setEditId(null);
      setForm(emptyForm);
      setFormErr("");
      setCreateOpen(false);
      refresh();
      void toastSuccess("Cluster diperbarui");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/clusters/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      if (editId === remove.variables) {
        setEditId(null);
        setForm(emptyForm);
      }
      refresh();
      void toastSuccess("Cluster dihapus");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  function startEdit(c: ClusterRow) {
    setCreateOpen(false);
    setEditId(c.id);
    setFormErr("");
    setForm({
      name: c.name,
      code: c.code,
      customer_code_prefix: c.customer_code_prefix,
      customer_code_pattern: c.customer_code_pattern || "{prefix}-{yyyymm}-{seq}",
      seq_width: c.seq_width || 4,
      address: c.address || "",
      latitude: c.latitude != null ? String(c.latitude) : "",
      longitude: c.longitude != null ? String(c.longitude) : "",
      notes: c.notes || "",
      is_active: c.is_active,
    });
  }

  function closeForm() {
    setEditId(null);
    setCreateOpen(false);
    setForm(emptyForm);
    setFormErr("");
  }

  const list = Array.isArray(q.data) ? q.data : [];
  const saving = create.isPending || update.isPending;
  const dialogOpen = createOpen || Boolean(editId);
  const examplePrefix = (form.customer_code_prefix || form.code || "DLMA").toUpperCase().replace(/[^A-Z0-9]/g, "") || "DLMA";
  const yyyymm = new Date().toISOString().slice(0, 7).replace("-", "");
  const exampleCode = (form.customer_code_pattern || "{prefix}-{yyyymm}-{seq}")
    .replace("{prefix}", examplePrefix)
    .replace("{yyyymm}", yyyymm)
    .replace("{yyyy}", yyyymm.slice(0, 4))
    .replace("{yy}", yyyymm.slice(2, 4))
    .replace("{mm}", yyyymm.slice(4, 6))
    .replace("{seq}", String(1).padStart(form.seq_width || 4, "0"));

  return (
    <Section
      title="Cluster / POP"
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
      <p className="mb-4 text-sm text-[var(--muted)]">
        Satu tenant bisa punya banyak POP/cluster. Isi lat/long agar muncul di peta FTTH (titik awal jalur kabel).
        Placeholder kode: <code className="text-xs">{"{prefix}"}</code>, <code className="text-xs">{"{yyyymm}"}</code>,{" "}
        <code className="text-xs">{"{seq}"}</code>.
      </p>

      <Table
        columns={["Nama", "Kode", "Prefix", "Koordinat", "Status", "Aksi"]}
        rows={list.map((c) => [
          c.name,
          c.code,
          c.customer_code_prefix,
          c.latitude != null && c.longitude != null ? `${c.latitude.toFixed(5)}, ${c.longitude.toFixed(5)}` : "—",
          c.is_active ? "aktif" : "nonaktif",
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton label="Edit cluster" onClick={() => startEdit(c)}>
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus cluster"
              danger
              disabled={remove.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus cluster",
                  description: `Hapus cluster "${c.name}"? Router/pelanggan tetap ada (cluster dikosongkan).`,
                  confirmLabel: "Hapus",
                });
                if (!ok) return;
                remove.mutate(c.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />

      <FormDialog open={dialogOpen} wide title={editId ? "Edit cluster / POP" : "Tambah cluster / POP"} onClose={closeForm}>
        <p className="mb-3 text-xs text-[var(--muted)]">
          Contoh kode: <span className="font-semibold text-[var(--text)]">{exampleCode}</span>
        </p>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (editId) update.mutate();
            else create.mutate();
          }}
        >
          <input
            className="input"
            placeholder="Nama (mis. Delima)"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            required
          />
          <input
            className="input"
            placeholder="Kode cluster (mis. DLMA)"
            value={form.code}
            onChange={(e) => {
              const code = e.target.value.toUpperCase();
              setForm({
                ...form,
                code,
                customer_code_prefix: form.customer_code_prefix || code.replace(/[^A-Z0-9]/g, ""),
              });
            }}
            required
          />
          <input
            className="input"
            placeholder="Prefix kode pelanggan"
            value={form.customer_code_prefix}
            onChange={(e) => setForm({ ...form, customer_code_prefix: e.target.value.toUpperCase() })}
          />
          <input
            className="input"
            type="number"
            min={1}
            max={8}
            placeholder="Lebar nomor urut"
            value={form.seq_width}
            onChange={(e) => setForm({ ...form, seq_width: Number(e.target.value) || 4 })}
          />
          <input
            className="input sm:col-span-2"
            placeholder="Pola kode ({prefix}-{yyyymm}-{seq})"
            value={form.customer_code_pattern}
            onChange={(e) => setForm({ ...form, customer_code_pattern: e.target.value })}
          />
          <input
            className="input sm:col-span-2"
            placeholder="Alamat (opsional)"
            value={form.address}
            onChange={(e) => setForm({ ...form, address: e.target.value })}
          />
          <input
            className="input"
            type="number"
            step="any"
            placeholder="Latitude POP (peta)"
            value={form.latitude}
            onChange={(e) => setForm({ ...form, latitude: e.target.value })}
          />
          <input
            className="input"
            type="number"
            step="any"
            placeholder="Longitude POP (peta)"
            value={form.longitude}
            onChange={(e) => setForm({ ...form, longitude: e.target.value })}
          />
          <textarea
            className="input sm:col-span-2 min-h-[72px]"
            placeholder="Catatan"
            value={form.notes}
            onChange={(e) => setForm({ ...form, notes: e.target.value })}
          />
          {editId && (
            <label className="flex items-center gap-2 text-sm text-[var(--text-body)] sm:col-span-2">
              <input type="checkbox" checked={form.is_active} onChange={(e) => setForm({ ...form, is_active: e.target.checked })} />
              Cluster aktif
            </label>
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

function Plans() {
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
  const [form, setForm] = useState<PlanForm>(emptyPlanForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [planOpen, setPlanOpen] = useState(false);
  const [planErr, setPlanErr] = useState("");
  const [offerForm, setOfferForm] = useState({ plan_id: "", cluster_id: "", price: 150000, sync_profiles: true });
  const [offerOpen, setOfferOpen] = useState(false);
  const [offerErr, setOfferErr] = useState("");
  const [syncMsg, setSyncMsg] = useState("");

  const refreshPlans = () => {
    qc.invalidateQueries({ queryKey: ["plans"] });
    qc.invalidateQueries({ queryKey: ["plan-offers"] });
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
        is_active: form.is_active,
      };
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
    mutationFn: () =>
      api("/api/plan-offers", {
        method: "POST",
        body: JSON.stringify({
          plan_id: offerForm.plan_id,
          cluster_id: offerForm.cluster_id,
          price: offerForm.price,
          sync_profiles: offerForm.sync_profiles,
        }),
      }),
    onSuccess: () => {
      setOfferErr("");
      setSyncMsg(offerForm.sync_profiles ? "Offer disimpan; sync profile dijalankan." : "Offer disimpan.");
      setOfferOpen(false);
      qc.invalidateQueries({ queryKey: ["plan-offers"] });
      void toastSuccess("Offer ditambahkan");
    },
    onError: (e: Error) => {
      setOfferErr(e.message);
      void toastError(e.message);
    },
  });

  const syncOffer = useMutation({
    mutationFn: (id: string) =>
      api<{ synced: number; failed: number }>(`/api/plan-offers/${id}/sync-profiles`, { method: "POST" }),
    onSuccess: (res) => setSyncMsg(`Sync selesai: ${res.synced} ok, ${res.failed} gagal`),
    onError: (e: Error) => setSyncMsg(e.message),
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

  return (
    <Section
      title="Paket"
      actions={
        <>
          <button type="button" className="btn-ghost" onClick={() => { setOfferErr(""); setOfferOpen(true); }}>
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

      <Table
        columns={["Nama", "Kode", "Harga dasar", "DL / UL (Mbps)", "Profile", "Tipe", "Status", "Aksi"]}
        rows={plans.map((p) => [
          p.name,
          p.code,
          formatRp(p.price),
          `DL ${p.download_mbps} / UL ${p.upload_mbps || p.download_mbps}`,
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
      {syncMsg && <p className="mb-2 text-sm text-[var(--muted)]">{syncMsg}</p>}
      <Table
        columns={["Cluster", "Paket", "Harga", "DL (Mbps)", "Status", "Aksi"]}
        rows={offers.map((o) => [
          `${o.cluster_name} (${o.cluster_code})`,
          `${o.plan_name} (${o.plan_code})`,
          formatRp(o.price),
          o.download_mbps,
          o.is_active ? "aktif" : "nonaktif",
          <span key="act" className="flex flex-wrap items-center gap-1.5">
            <IconButton label="Sync profile ke router" onClick={() => syncOffer.mutate(o.id)} disabled={syncOffer.isPending}>
              <IconPlug />
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

      <FormDialog open={offerOpen} wide title="Tambah offer cluster" onClose={() => { setOfferOpen(false); setOfferErr(""); }}>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            upsertOffer.mutate();
          }}
        >
          <select
            className="input"
            value={offerForm.plan_id}
            onChange={(e) => {
              const plan = plans.find((p) => p.id === e.target.value);
              setOfferForm({ ...offerForm, plan_id: e.target.value, price: plan?.price ?? offerForm.price });
            }}
            required
          >
            <option value="">— Pilih paket —</option>
            {plans.map((p) => (
              <option key={p.id} value={p.id}>{p.name} ({p.code})</option>
            ))}
          </select>
          <select className="input" value={offerForm.cluster_id} onChange={(e) => setOfferForm({ ...offerForm, cluster_id: e.target.value })} required>
            <option value="">— Pilih cluster —</option>
            {clusters.map((c) => (
              <option key={c.id} value={c.id}>{c.name} ({c.code})</option>
            ))}
          </select>
          <input className="input" type="number" placeholder="Harga di cluster" value={offerForm.price} onChange={(e) => setOfferForm({ ...offerForm, price: Number(e.target.value) })} required />
          <label className="flex items-center gap-2 text-sm text-[var(--text-body)]">
            <input type="checkbox" checked={offerForm.sync_profiles} onChange={(e) => setOfferForm({ ...offerForm, sync_profiles: e.target.checked })} />
            Sync profile ke router cluster
          </label>
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <button className="btn" disabled={upsertOffer.isPending}>{upsertOffer.isPending ? "Menyimpan…" : "Simpan"}</button>
            <button type="button" className="btn-ghost" onClick={() => setOfferOpen(false)}>Batal</button>
          </div>
          {offerErr && <p className="text-sm text-[var(--danger)] sm:col-span-2">{offerErr}</p>}
        </form>
      </FormDialog>
    </Section>
  );
}

function Subscriptions() {
  const qc = useQueryClient();
  type SubRow = {
    id: string;
    username: string;
    customer_name: string;
    plan_name: string;
    status: string;
    odp_id?: string | null;
    odp_code?: string;
    odp_name?: string;
    port_number?: number | null;
  };
  type CustomerOpt = {
    id: string;
    full_name: string;
    customer_code: string;
    cluster_id?: string | null;
    cluster_name?: string;
  };
  type OfferOpt = {
    plan_id: string;
    plan_name: string;
    plan_code: string;
    price: number;
    service_type: string;
    is_active: boolean;
  };
  type RouterOpt = { id: string; name: string; cluster_id?: string | null; is_active: boolean };
  type OdpOpt = {
    id: string;
    name: string;
    code: string;
    cluster_id?: string | null;
    port_count: number;
    used_ports: number;
    free_ports: number;
  };
  type OdpPortOpt = { port_number: number; status: string };

  const [form, setForm] = useState({
    customer_id: "",
    plan_id: "",
    router_id: "",
    username: "",
    password: "",
    odp_id: "",
    port_number: "",
  });
  const [open, setOpen] = useState(false);
  const [formErr, setFormErr] = useState("");

  const listQ = useQuery({
    queryKey: ["subs"],
    queryFn: () => api<{ data: SubRow[] }>("/api/subscriptions"),
  });
  const customersQ = useQuery({
    queryKey: ["customers"],
    queryFn: () => api<{ data: CustomerOpt[] }>("/api/customers?limit=100"),
  });
  const routersQ = useQuery({
    queryKey: ["routers"],
    queryFn: () => api<RouterOpt[]>("/api/routers"),
  });
  const odpsQ = useQuery({
    queryKey: ["odps"],
    queryFn: () => api<OdpOpt[]>("/api/odps"),
    enabled: open,
  });

  const customers = customersQ.data?.data ?? [];
  const selectedCustomer = customers.find((c) => c.id === form.customer_id);
  const clusterId = selectedCustomer?.cluster_id || "";

  const offersQ = useQuery({
    queryKey: ["plan-offers", clusterId],
    queryFn: () => api<OfferOpt[]>(`/api/plan-offers?cluster_id=${clusterId}`),
    enabled: open && Boolean(clusterId),
  });
  const plansQ = useQuery({
    queryKey: ["plans"],
    queryFn: () => api<{ id: string; name: string; code: string; price: number }[]>("/api/plans"),
    enabled: open && Boolean(form.customer_id) && !clusterId,
  });
  const odpPortsQ = useQuery({
    queryKey: ["odp-ports", form.odp_id],
    queryFn: () => api<{ ports: OdpPortOpt[] }>(`/api/odps/${form.odp_id}/ports`),
    enabled: open && Boolean(form.odp_id),
  });

  const offers = (Array.isArray(offersQ.data) ? offersQ.data : []).filter((o) => o.is_active);
  const basePlans = Array.isArray(plansQ.data) ? plansQ.data : [];
  const routers = (Array.isArray(routersQ.data) ? routersQ.data : []).filter(
    (r) => r.is_active && (!clusterId || r.cluster_id === clusterId),
  );
  const odps = (Array.isArray(odpsQ.data) ? odpsQ.data : []).filter(
    (o) => (!clusterId || o.cluster_id === clusterId) && o.free_ports > 0,
  );
  const freePorts = (odpPortsQ.data?.ports ?? []).filter((p) => p.status === "available");
  const selectedOdp = odps.find((o) => o.id === form.odp_id) || (Array.isArray(odpsQ.data) ? odpsQ.data : []).find((o) => o.id === form.odp_id);

  const create = useMutation({
    mutationFn: () => {
      const body: Record<string, unknown> = {
        customer_id: form.customer_id,
        plan_id: form.plan_id,
        router_id: form.router_id || undefined,
        username: form.username,
        password: form.password,
      };
      if (form.odp_id) {
        body.odp_id = form.odp_id;
        if (form.port_number) body.port_number = Number(form.port_number);
      }
      return api("/api/subscriptions", { method: "POST", body: JSON.stringify(body) });
    },
    onSuccess: () => {
      setFormErr("");
      setForm({ customer_id: form.customer_id, plan_id: "", router_id: "", username: "", password: "", odp_id: "", port_number: "" });
      setOpen(false);
      qc.invalidateQueries({ queryKey: ["subs"] });
      qc.invalidateQueries({ queryKey: ["odps"] });
      qc.invalidateQueries({ queryKey: ["ftth-map"] });
      void toastSuccess("Langganan ditambahkan");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const activate = useMutation({
    mutationFn: (id: string) => api(`/api/subscriptions/${id}/activate`, { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["subs", "invoices"] }),
    onError: (e: Error) => setFormErr(e.message),
  });

  return (
    <Section
      title="Langganan"
      actions={
        <button type="button" className="btn" onClick={() => { setFormErr(""); setOpen(true); }}>
          + Tambah
        </button>
      }
    >
      {formErr && !open && <p className="mb-3 text-sm text-[var(--danger)]">{formErr}</p>}
      <Table
        columns={["Username", "Pelanggan", "Paket", "ODP / Port", "Status", "Aksi"]}
        rows={(listQ.data?.data ?? []).map((s) => [
          s.username,
          s.customer_name,
          s.plan_name,
          s.odp_code
            ? `${s.odp_code}${s.port_number != null ? ` · P${s.port_number}` : ""}`
            : "—",
          s.status,
          s.status === "pending" ? (
            <IconButton key="act" label="Aktifkan" onClick={() => activate.mutate(s.id)} disabled={activate.isPending}>
              <IconPlug />
            </IconButton>
          ) : (
            "—"
          ),
        ])}
      />

      <FormDialog open={open} wide title="Buat langganan" onClose={() => { setOpen(false); setFormErr(""); }}>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <select
            className="input sm:col-span-2"
            value={form.customer_id}
            onChange={(e) =>
              setForm({
                customer_id: e.target.value,
                plan_id: "",
                router_id: "",
                username: form.username,
                password: form.password,
                odp_id: "",
                port_number: "",
              })
            }
            required
          >
            <option value="">— Pelanggan —</option>
            {customers.map((c) => (
              <option key={c.id} value={c.id}>
                {c.customer_code} — {c.full_name}
                {c.cluster_name ? ` [${c.cluster_name}]` : ""}
              </option>
            ))}
          </select>
          {selectedCustomer && !clusterId && (
            <p className="text-xs text-[var(--warn)] sm:col-span-2">
              Pelanggan tanpa cluster: set cluster di halaman Pelanggan agar harga offer dipakai.
            </p>
          )}
          <select
            className="input"
            value={form.plan_id}
            onChange={(e) => setForm({ ...form, plan_id: e.target.value })}
            required
            disabled={Boolean(clusterId) && offers.length === 0}
          >
            <option value="">— Paket {clusterId ? "(offer cluster)" : "(harga dasar)"} —</option>
            {clusterId
              ? offers.map((o) => (
                  <option key={o.plan_id} value={o.plan_id}>
                    {o.plan_name} — {formatRp(o.price)}
                  </option>
                ))
              : basePlans.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} — {formatRp(p.price)}
                  </option>
                ))}
          </select>
          <select className="input" value={form.router_id} onChange={(e) => setForm({ ...form, router_id: e.target.value })}>
            <option value="">— Router (opsional) —</option>
            {routers.map((r) => (
              <option key={r.id} value={r.id}>{r.name}</option>
            ))}
          </select>
          <select
            className="input"
            value={form.odp_id}
            onChange={(e) => setForm({ ...form, odp_id: e.target.value, port_number: "" })}
          >
            <option value="">— ODP (opsional, cek slot) —</option>
            {odps.map((o) => (
              <option key={o.id} value={o.id}>
                {o.code} — {o.name} · sisa {o.free_ports}/{o.port_count}
              </option>
            ))}
          </select>
          <select
            className="input"
            value={form.port_number}
            onChange={(e) => setForm({ ...form, port_number: e.target.value })}
            disabled={!form.odp_id}
          >
            <option value="">— Port otomatis (port kosong terendah) —</option>
            {freePorts.map((p) => (
              <option key={p.port_number} value={String(p.port_number)}>
                Port {p.port_number}
              </option>
            ))}
          </select>
          {form.odp_id && selectedOdp && (
            <p className="text-xs text-[var(--muted)] sm:col-span-2">
              ODP {selectedOdp.code}: terpakai {selectedOdp.used_ports}, sisa {selectedOdp.free_ports} dari {selectedOdp.port_count} port.
            </p>
          )}
          <input
            className="input"
            placeholder="Username PPPoE"
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
            required
            autoComplete="off"
          />
          <SecretInput
            placeholder="Password PPPoE"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            required
            autoComplete="new-password"
          />
          <p className="text-xs text-[var(--muted)] sm:col-span-2">
            Password dipakai saat aktivasi ke RouterOS. Komentar secret: kode + nama pelanggan (bukan UUID).
          </p>
          {clusterId && offers.length === 0 && (
            <p className="text-sm text-[var(--danger)] sm:col-span-2">Belum ada offer paket untuk cluster ini.</p>
          )}
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <button className="btn" disabled={create.isPending || (Boolean(clusterId) && !form.plan_id)}>
              {create.isPending ? "Menyimpan…" : "Simpan"}
            </button>
            <button type="button" className="btn-ghost" onClick={() => setOpen(false)}>Batal</button>
          </div>
          {formErr && <p className="text-sm text-[var(--danger)] sm:col-span-2">{formErr}</p>}
        </form>
      </FormDialog>
    </Section>
  );
}

function Invoices() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["invoices"],
    queryFn: () => api<{ data: { id: string; invoice_number: string; customer_name: string; total_amount: number; status: string }[] }>("/api/invoices"),
  });
  return (
    <Section title="Tagihan">
      <Table
        columns={["Nomor", "Pelanggan", "Total", "Status", "Aksi"]}
        rows={(q.data?.data ?? []).map((i) => [
          i.invoice_number,
          i.customer_name,
          formatRp(i.total_amount),
          i.status,
          <InvoiceActions key={i.id} id={i.id} onDone={() => qc.invalidateQueries({ queryKey: ["invoices"] })} />,
        ])}
      />
    </Section>
  );
}

function Routers() {
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

      <Table
        columns={["Nama", "Cluster", "Alamat", "Port", "User", "TLS", "Status", "Last seen", "Aksi"]}
        rows={list.map((r) => {
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
                <IconPlug />
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
                    title: "Hapus router",
                    description: `Hapus router "${r.name}"?`,
                    confirmLabel: "Hapus",
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
      {list.length > 0 && (
        <p className="mt-2 text-xs text-[var(--muted)]">
          Status ONLINE/OFFLINE dari hasil test koneksi (atau last seen / last error). Klik ikon plug untuk menguji.
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

function Tickets() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["tickets"],
    queryFn: () => api<{ data: { subject: string; category: string; priority: string; status: string }[] }>("/api/tickets"),
  });
  const [form, setForm] = useState({ subject: "", description: "", category: "general", priority: "normal" });
  const [open, setOpen] = useState(false);
  const mut = useMutation({
    mutationFn: () => api("/api/tickets", { method: "POST", body: JSON.stringify(form) }),
    onSuccess: () => {
      setForm({ subject: "", description: "", category: "general", priority: "normal" });
      setOpen(false);
      qc.invalidateQueries({ queryKey: ["tickets"] });
    },
  });
  return (
    <Section
      title="Tiket"
      actions={
        <button type="button" className="btn" onClick={() => setOpen(true)}>
          + Tambah
        </button>
      }
    >
      <Table columns={["Subjek", "Kategori", "Prioritas", "Status"]} rows={(q.data?.data ?? []).map((t) => [t.subject, t.category, t.priority, t.status])} />
      <FormDialog open={open} title="Buat tiket" onClose={() => setOpen(false)}>
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            mut.mutate();
          }}
        >
          <input className="input" placeholder="Subjek" value={form.subject} onChange={(e) => setForm({ ...form, subject: e.target.value })} required />
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

function ODP({ tenantSlug }: { tenantSlug?: string }) {
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
  };
  type OdpForm = { name: string; code: string; latitude: string; longitude: string; port_count: number };
  const emptyForm: OdpForm = { name: "", code: "", latitude: "", longitude: "", port_count: 8 };

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

  const filtered = list.filter((o) => {
    if (tabId === "__none__") return !o.cluster_id;
    if (!tabId) return true;
    return o.cluster_id === tabId;
  });

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
      port_count: o.port_count,
    });
  }

  function closeForm() {
    setEditId(null);
    setForm(emptyForm);
    setFormErr("");
  }

  function countForCluster(id: string) {
    if (id === "__none__") return unassignedCount;
    return list.filter((o) => o.cluster_id === id).length;
  }

  return (
    <Section
      title="ODP / FTTH"
      actions={
        <span className="flex flex-wrap items-center gap-1.5">
          <IconButton label="Export CSV" onClick={() => void exportCsv()}>
            <IconDownload />
          </IconButton>
          <IconButton label="Import CSV" onClick={() => { setImportMsg(""); setImportOpen(true); }}>
            <IconUpload />
          </IconButton>
        </span>
      }
    >
      {clusters.length === 0 && unassignedCount === 0 ? (
        <p className="mb-4 text-sm text-[var(--muted)]">
          Belum ada cluster. Buat Cluster/POP dulu (isi lat/long) agar tab &amp; peta bisa dipakai.
        </p>
      ) : (
        <div className="cluster-tabs" role="tablist" aria-label="Cluster / POP">
          {clusters.map((c) => (
            <button
              key={c.id}
              type="button"
              role="tab"
              aria-selected={tabId === c.id}
              className={`cluster-tab${tabId === c.id ? " is-active" : ""}`}
              onClick={() => selectClusterTab(c.id)}
            >
              {c.name}
              <span className="cluster-tab-meta">{c.code} · {countForCluster(c.id)}</span>
            </button>
          ))}
          {unassignedCount > 0 && (
            <button
              type="button"
              role="tab"
              aria-selected={tabId === "__none__"}
              className={`cluster-tab${tabId === "__none__" ? " is-active" : ""}`}
              onClick={() => selectClusterTab("__none__")}
            >
              Tanpa cluster
              <span className="cluster-tab-meta">{unassignedCount}</span>
            </button>
          )}
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

      {formErr && !editId && <p className="mb-3 text-sm text-[var(--danger)]">{formErr}</p>}
      {importMsg && !importOpen && <p className="mb-3 text-sm text-[var(--muted)]">{importMsg}</p>}
      <Table
        columns={["Nama", "Kode", "Port", "Terpakai", "Sisa", "Koordinat", "Aksi"]}
        rows={filtered.map((o) => [
          o.name,
          o.code,
          o.port_count,
          o.used_ports,
          o.free_ports ?? Math.max(0, o.port_count - o.used_ports),
          o.latitude != null && o.longitude != null ? `${o.latitude}, ${o.longitude}` : "—",
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

      {tabId ? <MapODP odps={filtered} clusterId={tabId} clusterCenter={clusterCenter} /> : null}
    </Section>
  );
}

function Vouchers() {
  const [name, setName] = useState("Voucher 1 hari");
  const [open, setOpen] = useState(false);
  const mut = useMutation({
    mutationFn: () =>
      api("/api/vouchers/batches", {
        method: "POST",
        body: JSON.stringify({ name, price: 10000, codes: Array.from({ length: 10 }, (_, i) => `VCH${Date.now()}${i}`) }),
      }),
    onSuccess: () => {
      setOpen(false);
      void toastSuccess("Batch voucher ditambahkan");
    },
    onError: (e: Error) => void toastError(e.message),
  });
  return (
    <Section
      title="Voucher Hotspot"
      actions={
        <button type="button" className="btn" onClick={() => setOpen(true)}>
          + Tambah
        </button>
      }
    >
      {mut.isSuccess && (
        <p className="mb-3 text-sm text-[var(--ok)]">
          Batch dibuat.{" "}
          <a className="underline" href="/api/vouchers/batches/1/qr">
            Unduh QR batch #1
          </a>
        </p>
      )}
      <p className="text-sm text-[var(--muted)]">Generate batch voucher hotspot dari tombol + Tambah.</p>
      <FormDialog open={open} title="Generate voucher" onClose={() => setOpen(false)}>
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            mut.mutate();
          }}
        >
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} required />
          <div className="flex flex-wrap gap-2">
            <button className="btn" disabled={mut.isPending}>
              {mut.isPending ? "Membuat…" : "Generate 10 kode"}
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
