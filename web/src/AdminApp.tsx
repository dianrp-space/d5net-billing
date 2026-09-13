import { ThemeToggle } from "./ThemeToggle";
import { AlertsBell } from "./AlertsBell";
import { HeaderSearch } from "./HeaderSearch";
import { UserMenu } from "./UserMenu";
import { getSidebarOpen, setSidebarOpen } from "./navPersist";
import { applyBrandingMeta, DEFAULT_BRAND_LOGO } from "./branding";
import { canAccessPage, canDispatchOps, firstAllowedPage, type MePermissions } from "./permissions";
import { isAdminPage, pageTitles, type AdminPage } from "./admin/pages";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { api } from "./api";
import {
  IconBanknote,
  IconBell,
  IconBox,
  IconBriefcase,
  IconChart,
  IconClock,
  IconDownload,
  IconEye,
  IconGauge,
  IconHeadset,
  IconHome,
  IconMap,
  IconMapPin,
  IconPercent,
  IconPlug,
  IconRadar,
  IconReceipt,
  IconRouter,
  IconSend,
  IconServer,
  IconSettings,
  IconShield,
  IconShieldCheck,
  IconTicket,
  IconUser,
  IconUserPlus,
  IconUsers,
} from "./icons";
import { IconButton } from "./ui";
import { PanelLeft, PanelLeftClose, Home } from "lucide-react";
import { Suspense, Component, useEffect, useState, type ReactNode } from "react";
import { lazyWithReload as lazy, isChunkLoadError } from "./lazyReload";
import { useQuery } from "@tanstack/react-query";

export type { AdminPage } from "./admin/pages";
export { isAdminPage, normalizeAdminPage } from "./admin/pages";

type Page = AdminPage;

const DashboardPage = lazy(() =>
  import("./pages/DashboardPage").then((m) => ({ default: m.DashboardPage })),
);
const CustomersPage = lazy(() =>
  import("./pages/CustomersPage").then((m) => ({ default: m.CustomersPage })),
);
const CustomerGalleryPage = lazy(() =>
  import("./pages/CustomerGalleryPage").then((m) => ({ default: m.CustomerGalleryPage })),
);
const ClustersPage = lazy(() =>
  import("./pages/ClustersPage").then((m) => ({ default: m.ClustersPage })),
);
const PlansPage = lazy(() => import("./pages/PlansPage").then((m) => ({ default: m.PlansPage })));
const OffersPage = lazy(() => import("./pages/OffersPage").then((m) => ({ default: m.OffersPage })));
const DiscountsPage = lazy(() =>
  import("./pages/DiscountsPage").then((m) => ({ default: m.DiscountsPage })),
);
const SubscriptionsPage = lazy(() =>
  import("./pages/SubscriptionsPage").then((m) => ({ default: m.SubscriptionsPage })),
);
const InvoicesPage = lazy(() =>
  import("./pages/InvoicesPage").then((m) => ({ default: m.InvoicesPage })),
);
const PaymentsPage = lazy(() =>
  import("./pages/PaymentsPage").then((m) => ({ default: m.PaymentsPage })),
);
const RoutersPage = lazy(() =>
  import("./pages/RoutersPage").then((m) => ({ default: m.RoutersPage })),
);
const OdpPage = lazy(() => import("./pages/OdpPage").then((m) => ({ default: m.OdpPage })));
const CoveragePage = lazy(() =>
  import("./pages/CoveragePage").then((m) => ({ default: m.CoveragePage })),
);
const IPPoolPage = lazy(() => import("./pages/IPPoolPage").then((m) => ({ default: m.IPPoolPage })));
const TicketsPage = lazy(() => import("./TicketsPage").then((m) => ({ default: m.TicketsPage })));
const SLAReportPage = lazy(() =>
  import("./SLAReportPage").then((m) => ({ default: m.SLAReportPage })),
);
const VouchersPage = lazy(() =>
  import("./VouchersPage").then((m) => ({ default: m.VouchersPage })),
);
const LeadsPage = lazy(() => import("./LeadsKanban").then((m) => ({ default: m.LeadsPage })));
const AccountingPage = lazy(() =>
  import("./AdminExtra").then((m) => ({ default: m.AccountingPage })),
);
const ResellersPage = lazy(() =>
  import("./AdminExtra").then((m) => ({ default: m.ResellersPage })),
);
const GeneralSettingsPage = lazy(() =>
  import("./SettingsPages").then((m) => ({ default: m.GeneralSettingsPage })),
);
const InvoiceSettingsPage = lazy(() =>
  import("./SettingsPages").then((m) => ({ default: m.InvoiceSettingsPage })),
);
const IsolirTemplatePage = lazy(() =>
  import("./IsolirTemplatePage").then((m) => ({ default: m.IsolirTemplatePage })),
);
const JobsSettingsPage = lazy(() =>
  import("./JobsSettingsPage").then((m) => ({ default: m.JobsSettingsPage })),
);
const NotificationsPage = lazy(() =>
  import("./NotificationsPage").then((m) => ({ default: m.NotificationsPage })),
);
const AuditLogPage = lazy(() =>
  import("./pages/AuditLogPage").then((m) => ({ default: m.AuditLogPage })),
);
const RolesSettingsPage = lazy(() =>
  import("./SettingsPages").then((m) => ({ default: m.RolesSettingsPage })),
);
const UsersSettingsPage = lazy(() =>
  import("./SettingsPages").then((m) => ({ default: m.UsersSettingsPage })),
);
const WebhooksIntegrationPage = lazy(() =>
  import("./IntegrationPages").then((m) => ({ default: m.WebhooksIntegrationPage })),
);
const PaymentGWPage = lazy(() =>
  import("./IntegrationPages").then((m) => ({ default: m.PaymentGWPage })),
);
const MessagingGWPage = lazy(() =>
  import("./IntegrationPages").then((m) => ({ default: m.MessagingGWPage })),
);
const BackupRestorePage = lazy(() =>
  import("./BackupRestorePage").then((m) => ({ default: m.BackupRestorePage })),
);

type NavItem = { id: Page; label: string; icon: ReactNode };
type NavGroup = { label: string; items: NavItem[] };

const navGroups: NavGroup[] = [
  {
    label: "Main Menu",
    items: [{ id: "dashboard", label: "Dashboard", icon: <IconHome /> }],
  },
  {
    label: "Finance",
    items: [
      { id: "plans", label: "Paket", icon: <IconBox /> },
      { id: "offers", label: "Paket per Cluster", icon: <IconMapPin /> },
      { id: "invoices", label: "Tagihan", icon: <IconChart /> },
      { id: "payments", label: "Pembayaran", icon: <IconBanknote /> },
      { id: "accounting", label: "Akunting", icon: <IconBanknote /> },
    ],
  },
  {
    label: "Customers",
    items: [
      { id: "customers", label: "Pelanggan", icon: <IconUsers /> },
      { id: "discounts", label: "Diskon", icon: <IconPercent /> },
      { id: "leads", label: "Lead", icon: <IconUserPlus /> },
      { id: "coverage", label: "Coverage", icon: <IconRadar /> },
      { id: "resellers", label: "Reseller & Komisi", icon: <IconBriefcase /> },
    ],
  },
  {
    label: "Network",
    items: [
      { id: "clusters", label: "Cluster / POP", icon: <IconMapPin /> },
      { id: "routers", label: "Router", icon: <IconRouter /> },
      { id: "ip-pool", label: "IP Pool", icon: <IconServer /> },
      { id: "odp", label: "MAP FTTH", icon: <IconMap /> },
      { id: "vouchers", label: "Voucher", icon: <IconTicket /> },
    ],
  },
  {
    label: "Ops",
    items: [
      { id: "tickets", label: "Tiket", icon: <IconHeadset /> },
      { id: "sla-report", label: "Laporan SLA", icon: <IconGauge /> },
    ],
  },
  {
    label: "Integrasi",
    items: [
      { id: "webhooks", label: "Webhook", icon: <IconPlug /> },
      { id: "payment-gw", label: "Payment Gateway", icon: <IconReceipt /> },
      { id: "messaging-gw", label: "Messaging Gateway", icon: <IconSend /> },
      { id: "notifications", label: "Notifikasi", icon: <IconBell /> },
    ],
  },
  {
    label: "Settings",
    items: [
      { id: "general", label: "Umum", icon: <IconSettings /> },
      { id: "audit-logs", label: "Audit Log", icon: <IconEye /> },
      { id: "invoice-format", label: "Format Invoice", icon: <IconReceipt /> },
      { id: "isolir-template", label: "Template Isolir", icon: <IconShield /> },
      { id: "jobs", label: "Cronjob", icon: <IconClock /> },
      { id: "roles", label: "Roles", icon: <IconShieldCheck /> },
      { id: "users", label: "Users", icon: <IconUser /> },
      { id: "backup", label: "Backup / Restore", icon: <IconDownload /> },
    ],
  },
];

function PageFallback() {
  return <p className="text-sm text-[var(--muted)]">Memuat halaman…</p>;
}

class PageErrorBoundary extends Component<{ children: ReactNode; resetKey: string }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidUpdate(prevProps: { resetKey: string }) {
    if (prevProps.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null });
    }
  }

  render() {
    if (this.state.error) {
      const chunk = isChunkLoadError(this.state.error);
      return (
        <div className="rounded-[var(--radius)] border border-[var(--danger)]/30 bg-[var(--panel)] p-4">
          <p className="text-sm font-semibold text-[var(--danger)]">Halaman gagal dimuat</p>
          <p className="mt-1 text-sm text-[var(--muted)]">
            {chunk
              ? "Aplikasi baru saja diperbarui. Muat ulang halaman untuk memakai versi terbaru."
              : this.state.error.message}
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <button type="button" className="btn" onClick={() => window.location.reload()}>
              Muat ulang halaman
            </button>
            <button type="button" className="btn-ghost" onClick={() => this.setState({ error: null })}>
              Coba lagi
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

export function AdminApp({
  tenantSlug,
  page,
  customerSecretsId,
  customerSecretsCreate = false,
  customerGalleryId,
  onNavigate,
  onLogout,
}: {
  tenantSlug?: string;
  page: Page;
  customerSecretsId?: string | null;
  customerSecretsCreate?: boolean;
  customerGalleryId?: string | null;
  onNavigate: (page: Page, rest?: string[]) => void;
  onLogout: () => void;
}) {
  const branding = useQuery({
    queryKey: ["public-branding"],
    queryFn: () =>
      api<{
        app_name?: string;
        logo_url?: string | null;
        favicon_url?: string | null;
        name?: string;
      }>("/api/public/branding"),
    retry: false,
  });
  const meQ = useQuery({
    queryKey: ["me"],
    queryFn: () =>
      api<
        MePermissions & {
          user_id: string;
          email: string;
          full_name: string;
          avatar_url?: string | null;
        }
      >("/api/me"),
  });
  const perms = meQ.data?.permissions;
  const visibleGroups = navGroups
    .map((g) => ({
      ...g,
      items: g.items.filter((n) => canAccessPage(perms, n.id)),
    }))
    .filter((g) => g.items.length > 0);

  useEffect(() => {
    if (!meQ.data) return;
    if (page === "tech") {
      onNavigate("tickets");
      return;
    }
    if (page === "subscriptions") {
      onNavigate("customers");
      return;
    }
    const permPage = page === "customers" ? "customers" : page;
    if (!canAccessPage(meQ.data.permissions, permPage)) {
      const next = firstAllowedPage(meQ.data.permissions, "dashboard");
      if (isAdminPage(next) && next !== page) onNavigate(next);
    }
  }, [meQ.data, page, onNavigate]);

  const appName = (branding.data?.name || branding.data?.app_name || "Delima Net").trim();
  const logoUrl = branding.data?.logo_url;
  const faviconUrl = branding.data?.favicon_url;
  const [sidebarOpen, setSidebarOpenState] = useState(() => getSidebarOpen());
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(
    () => typeof window !== "undefined" && window.matchMedia("(max-width: 767px)").matches,
  );

  useEffect(() => {
    const mq = window.matchMedia("(max-width: 767px)");
    const onChange = () => {
      setIsMobile(mq.matches);
      if (!mq.matches) setMobileNavOpen(false);
    };
    onChange();
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  useEffect(() => {
    setMobileNavOpen(false);
  }, [page]);

  function toggleSidebar() {
    if (isMobile) {
      setMobileNavOpen((prev) => !prev);
      return;
    }
    setSidebarOpenState((prev) => {
      const next = !prev;
      setSidebarOpen(next);
      return next;
    });
  }

  function handleNavigate(next: Page, rest?: string[]) {
    if (isMobile) setMobileNavOpen(false);
    onNavigate(next, rest);
  }

  const navShown = isMobile ? mobileNavOpen : sidebarOpen;

  useEffect(() => {
    applyBrandingMeta({
      appName,
      faviconUrl,
      titleSuffix: pageTitles[page],
    });
  }, [appName, faviconUrl, page]);

  return (
    <div className={`app-shell${sidebarOpen ? "" : " is-sidebar-collapsed"}${mobileNavOpen ? " is-mobile-open" : ""}`}>
      {mobileNavOpen && (
        <button
          type="button"
          className="app-sidebar-backdrop"
          aria-label="Tutup navigasi"
          onClick={() => setMobileNavOpen(false)}
        />
      )}
      <aside className="app-sidebar" aria-label="Navigasi utama">
        <div className="app-sidebar-brand">
          <img src={logoUrl || DEFAULT_BRAND_LOGO} alt="" className="app-sidebar-logo object-contain" />
          <div className="app-sidebar-brand-text">
            <p className="text-[10px] font-medium text-[var(--stone)]">ISP Billing</p>
            <h1 className="text-sm font-bold">{appName}</h1>
          </div>
        </div>

        <nav className="flex-1 px-3 py-2">
          {visibleGroups.map((g) => (
            <div key={g.label} className="app-nav-group">
              <p className="app-nav-label">{g.label}</p>
              <div className="space-y-0.5">
                {g.items.map((n) => (
                  <button
                    key={n.id}
                    type="button"
                    title={n.label}
                    aria-label={n.label}
                    onClick={() => handleNavigate(n.id)}
                    className={`app-nav-item ${page === n.id || (n.id === "customers" && (customerSecretsId || customerGalleryId)) ? "is-active" : ""}`}
                  >
                    <span className="opacity-80">{n.icon}</span>
                    <span className="app-nav-item-label">{n.label}</span>
                  </button>
                ))}
              </div>
            </div>
          ))}
        </nav>
      </aside>

      <div className="app-main">
        <header className="app-header">
          <div className="app-header-start">
            <IconButton
              className="app-header-sidebar-toggle"
              label={navShown ? "Sembunyikan sidebar" : "Tampilkan sidebar"}
              onClick={toggleSidebar}
            >
              {navShown ? <PanelLeftClose /> : <PanelLeft />}
            </IconButton>
            <Breadcrumb className="app-header-breadcrumb">
              <BreadcrumbList>
                <BreadcrumbItem>
                  <BreadcrumbLink asChild>
                    <button
                      type="button"
                      className="inline-flex items-center gap-1.5"
                      onClick={() => {
                        if (canAccessPage(perms, "dashboard")) handleNavigate("dashboard");
                      }}
                      title="Dashboard"
                      aria-label="Dashboard"
                    >
                      <Home className="size-4" />
                      <span className="sr-only">Dashboard</span>
                    </button>
                  </BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator />
                {customerSecretsId || customerGalleryId ? (
                  <>
                    <BreadcrumbItem>
                      <BreadcrumbLink asChild>
                        <button type="button" className="truncate" onClick={() => handleNavigate("customers")}>
                          {pageTitles.customers}
                        </button>
                      </BreadcrumbLink>
                    </BreadcrumbItem>
                    <BreadcrumbSeparator />
                    {customerGalleryId ? (
                      <BreadcrumbItem>
                        <BreadcrumbPage className="truncate">Galeri</BreadcrumbPage>
                      </BreadcrumbItem>
                    ) : customerSecretsCreate ? (
                      <>
                        <BreadcrumbItem>
                          <BreadcrumbLink asChild>
                            <button
                              type="button"
                              className="truncate"
                              onClick={() => onNavigate("customers", [customerSecretsId!, "secrets"])}
                            >
                              Secrets
                            </button>
                          </BreadcrumbLink>
                        </BreadcrumbItem>
                        <BreadcrumbSeparator />
                        <BreadcrumbItem>
                          <BreadcrumbPage className="truncate">Buat</BreadcrumbPage>
                        </BreadcrumbItem>
                      </>
                    ) : (
                      <BreadcrumbItem>
                        <BreadcrumbPage className="truncate">Secrets</BreadcrumbPage>
                      </BreadcrumbItem>
                    )}
                  </>
                ) : (
                  <BreadcrumbItem>
                    <BreadcrumbPage className="truncate">{pageTitles[page]}</BreadcrumbPage>
                  </BreadcrumbItem>
                )}
              </BreadcrumbList>
            </Breadcrumb>
          </div>
          <div className="flex items-center gap-3">
            <HeaderSearch
              allowedPages={perms}
              onNavigate={(next, rest) => {
                if (isAdminPage(next) && canAccessPage(perms, next)) handleNavigate(next, rest);
              }}
            />
            <AlertsBell onNavigatePage={(p) => onNavigate(p)} />
            <ThemeToggle />
            <UserMenu user={meQ.data} onLogout={onLogout} />
          </div>
        </header>

        <main className="app-content">
          {meQ.isLoading && <p className="text-sm text-[var(--muted)]">Memuat izin akses…</p>}
          {meQ.isError && (
            <p className="text-sm text-[var(--danger)]">Gagal memuat izin role. Coba refresh atau login ulang.</p>
          )}
          {meQ.data && canAccessPage(perms, page) && (
            <PageErrorBoundary resetKey={page}>
              <Suspense fallback={<PageFallback />}>
              {page === "dashboard" && (
                <DashboardPage
                  userName={meQ.data?.full_name || meQ.data?.email}
                  fieldOps={!canDispatchOps(perms)}
                  onNavigate={onNavigate}
                />
              )}
              {page === "customers" && !customerSecretsId && !customerGalleryId && (
                <CustomersPage
                  onOpenSecrets={(id) => onNavigate("customers", [id, "secrets"])}
                  onOpenGallery={(id) => onNavigate("customers", [id, "gallery"])}
                />
              )}
              {page === "customers" && customerGalleryId ? (
                <CustomerGalleryPage
                  customerId={customerGalleryId}
                  onBack={() => onNavigate("customers")}
                />
              ) : null}
              {page === "customers" && customerSecretsId ? (
                <SubscriptionsPage
                  customerId={customerSecretsId}
                  createMode={customerSecretsCreate}
                  onBack={() => onNavigate("customers")}
                  onOpenList={() => onNavigate("customers", [customerSecretsId, "secrets"])}
                  onOpenCreate={() => onNavigate("customers", [customerSecretsId, "secrets", "new"])}
                  onNavigatePage={(p) => onNavigate(p)}
                />
              ) : null}
              {page === "clusters" && <ClustersPage />}
              {page === "plans" && <PlansPage onNavigate={(p) => onNavigate(p)} />}
              {page === "offers" && <OffersPage onNavigate={(p) => onNavigate(p)} />}
              {page === "discounts" && <DiscountsPage />}
              {page === "invoices" && <InvoicesPage />}
              {page === "payments" && <PaymentsPage />}
              {page === "routers" && <RoutersPage />}
              {page === "ip-pool" && <IPPoolPage tenantSlug={tenantSlug} onNavigate={(p) => onNavigate(p)} />}
              {page === "tickets" && <TicketsPage />}
              {page === "sla-report" && <SLAReportPage />}
              {page === "odp" && <OdpPage tenantSlug={tenantSlug} onNavigate={(p) => onNavigate(p)} />}
              {page === "coverage" && (
                <CoveragePage
                  onNavigate={(p) => onNavigate(p)}
                  canEdit={
                    canAccessPage(perms, "clusters") ||
                    canAccessPage(perms, "odp") ||
                    canAccessPage(perms, "network")
                  }
                />
              )}
              {page === "vouchers" && <VouchersPage />}
              {page === "leads" && <LeadsPage />}
              {page === "accounting" && <AccountingPage />}
              {page === "resellers" && <ResellersPage />}
              {page === "general" && <GeneralSettingsPage />}
              {page === "audit-logs" && <AuditLogPage />}
              {page === "invoice-format" && <InvoiceSettingsPage />}
              {page === "isolir-template" && <IsolirTemplatePage />}
              {page === "jobs" && <JobsSettingsPage />}
              {page === "notifications" && <NotificationsPage />}
              {page === "roles" && <RolesSettingsPage />}
              {page === "users" && <UsersSettingsPage />}
              {page === "webhooks" && <WebhooksIntegrationPage />}
              {page === "payment-gw" && <PaymentGWPage />}
              {page === "messaging-gw" && <MessagingGWPage />}
              {page === "backup" && <BackupRestorePage />}
            </Suspense>
            </PageErrorBoundary>
          )}
        </main>
      </div>
    </div>
  );
}
