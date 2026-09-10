import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Home, PanelLeft, PanelLeftClose } from "lucide-react";
import { api, apiDownload, clearClientSession, getClientSession, setClientSession } from "./api";
import { applyBrandingMeta, DEFAULT_BRAND_LOGO } from "./branding";
import type { ClientPortalData } from "./TenantLogin";
import { toastError, toastSuccess } from "./swal";
import { ThemeToggle } from "./ThemeToggle";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { formatRp, IconButton, invoiceStatusLabel, paymentStatusLabel, Section, SecretInput, subscriptionStatusLabel, Table, ticketStatusHint, ticketStatusLabel, ticketStatusTone } from "./ui";
import { IconBan, IconBanknote, IconChart, IconDownload, IconGauge, IconLock, IconLogout, IconShield, IconTicket } from "./icons";
import { PortalPayHost } from "./PayMethodDialog";
import { clearSavedPayMethod, hasSavedPayMethod, invoiceRemaining, isInvoiceUnpaid, isIsolirStatus, paymentMethodLabel, type PayableInvoice } from "./payMethod";
import { canChangePortalPlan, PortalChangePlanDialog, PortalPlanCatalog, type PortalPlan, type PortalSub } from "./PortalChangePlan";
import { getSidebarOpen, setSidebarOpen, usePersistedTab } from "./navPersist";

type ClientPage = "home" | "plans" | "invoices" | "payments" | "tickets" | "account";

const CLIENT_PAGES: { id: ClientPage; label: string; icon: ReactNode }[] = [
  { id: "home", label: "Beranda", icon: <Home size={18} /> },
  { id: "plans", label: "Paket", icon: <IconGauge /> },
  { id: "invoices", label: "Tagihan", icon: <IconChart /> },
  { id: "payments", label: "Pembayaran", icon: <IconBanknote /> },
  { id: "tickets", label: "Keluhan", icon: <IconTicket /> },
  { id: "account", label: "Akun", icon: <IconLock /> },
];

const pageTitles: Record<ClientPage, string> = {
  home: "Beranda",
  plans: "Paket",
  invoices: "Tagihan",
  payments: "Riwayat pembayaran",
  tickets: "Keluhan",
  account: "Akun",
};

const TICKET_CATEGORIES: { id: string; label: string }[] = [
  { id: "outage", label: "Gangguan" },
  { id: "technical", label: "Teknis" },
  { id: "billing", label: "Tagihan" },
  { id: "installation", label: "Instalasi" },
  { id: "general", label: "Umum" },
];

const TICKET_PRIORITIES: { id: string; label: string }[] = [
  { id: "low", label: "Rendah" },
  { id: "normal", label: "Normal" },
  { id: "high", label: "Tinggi" },
  { id: "urgent", label: "Mendesak" },
];

type PortalTicket = {
  id: string;
  subject: string;
  description?: string | null;
  category: string;
  priority: string;
  status: string;
  customer_name?: string;
  created_at?: string;
};

type PortalTicketMessage = {
  id: string;
  sender_type: string;
  sender_name?: string;
  message: string;
  image_urls?: string[];
  created_at: string;
};

function formatPortalWhen(iso?: string) {
  if (!iso) return "";
  try {
    return new Date(iso).toLocaleString("id-ID", { dateStyle: "medium", timeStyle: "short" });
  } catch {
    return iso;
  }
}

function PortalTicketCard({
  ticket,
  headers,
  tenantSlug,
  multi,
  categoryLabel,
  priorityLabel,
}: {
  ticket: PortalTicket;
  headers?: HeadersInit;
  tenantSlug: string;
  multi: boolean;
  categoryLabel: string;
  priorityLabel: string;
}) {
  const [open, setOpen] = useState(false);
  const [reply, setReply] = useState("");
  const qc = useQueryClient();
  const canReply = ticket.status === "open" || ticket.status === "in_progress";
  const tone = ticketStatusTone(ticket.status);
  const hint = ticketStatusHint(ticket.status);

  const msgsQ = useQuery({
    queryKey: ["portal-ticket-messages", ticket.id],
    queryFn: () => api<PortalTicketMessage[]>(`/api/portal/tickets/${ticket.id}/messages`, { headers }),
    enabled: open && Boolean(headers),
    retry: false,
  });

  const sendReply = useMutation({
    mutationFn: () =>
      api<PortalTicketMessage>(`/api/portal/tickets/${ticket.id}/messages`, {
        method: "POST",
        headers,
        body: JSON.stringify({ message: reply.trim() }),
      }),
    onSuccess: () => {
      setReply("");
      void qc.invalidateQueries({ queryKey: ["portal-ticket-messages", ticket.id] });
      void qc.invalidateQueries({ queryKey: ["portal-tickets", tenantSlug] });
      void toastSuccess("Balasan terkirim");
    },
    onError: (e: Error) => void toastError(e.message || "Gagal mengirim balasan"),
  });

  const messages = Array.isArray(msgsQ.data) ? msgsQ.data : [];

  return (
    <article className="portal-item-card portal-ticket-card">
      <div className="flex items-start justify-between gap-3">
        <p className="min-w-0 text-sm font-semibold leading-snug">{ticket.subject}</p>
        <span className={`portal-status is-${tone}`}>{ticketStatusLabel(ticket.status)}</span>
      </div>
      {hint ? <p className="text-sm font-medium text-[var(--text)]">{hint}</p> : null}
      <p className="text-xs text-[var(--muted)]">
        {categoryLabel}
        {" · "}
        {priorityLabel}
        {ticket.created_at ? ` · ${formatPortalWhen(ticket.created_at)}` : ""}
      </p>
      {multi && ticket.customer_name ? (
        <p className="text-xs text-[var(--muted)]">{ticket.customer_name}</p>
      ) : null}
      {ticket.description && !open ? (
        <p className="line-clamp-3 whitespace-pre-wrap text-sm">{ticket.description}</p>
      ) : null}
      <button type="button" className="btn-ghost w-fit text-sm" onClick={() => setOpen((v) => !v)}>
        {open ? "Tutup percakapan" : "Lihat percakapan"}
      </button>
      {open ? (
        <div className="portal-ticket-thread">
          {msgsQ.isLoading ? (
            <p className="text-sm text-[var(--muted)]">Memuat percakapan…</p>
          ) : messages.length === 0 ? (
            ticket.description ? (
              <div className="portal-msg is-customer">
                <p className="portal-msg-meta">Anda · keluhan awal</p>
                <p className="whitespace-pre-wrap text-sm">{ticket.description}</p>
              </div>
            ) : (
              <p className="text-sm text-[var(--muted)]">Belum ada percakapan.</p>
            )
          ) : (
            messages.map((m) => {
              const kind =
                m.sender_type === "system" ? "system" : m.sender_type === "staff" ? "staff" : "customer";
              const who =
                kind === "system"
                  ? "Status"
                  : kind === "staff"
                    ? m.sender_name
                      ? `Tim · ${m.sender_name}`
                      : "Tim support"
                    : m.sender_name
                      ? `Anda · ${m.sender_name}`
                      : "Anda";
              return (
                <div key={m.id} className={`portal-msg is-${kind}`}>
                  <p className="portal-msg-meta">
                    {who}
                    {m.created_at ? ` · ${formatPortalWhen(m.created_at)}` : ""}
                  </p>
                  {m.message ? <p className="whitespace-pre-wrap text-sm">{m.message}</p> : null}
                  {(m.image_urls?.length ?? 0) > 0 ? (
                    <div className="mt-2 flex flex-wrap gap-2">
                      {m.image_urls!.map((url) => (
                        <a key={url} href={url} target="_blank" rel="noreferrer">
                          <img src={url} alt="" className="h-16 w-16 rounded object-cover" />
                        </a>
                      ))}
                    </div>
                  ) : null}
                </div>
              );
            })
          )}
          {canReply ? (
            <form
              className="grid gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (!reply.trim()) return;
                sendReply.mutate();
              }}
            >
              <textarea
                className="input min-h-24"
                placeholder="Tulis balasan…"
                value={reply}
                onChange={(e) => setReply(e.target.value)}
                required
              />
              <button className="btn w-fit" disabled={sendReply.isPending || !reply.trim()}>
                {sendReply.isPending ? "Mengirim…" : "Kirim balasan"}
              </button>
            </form>
          ) : (
            <p className="text-xs text-[var(--muted)]">Tiket ini sudah tidak menerima balasan baru.</p>
          )}
        </div>
      ) : null}
    </article>
  );
}

export function ClientHome({
  data,
  onLogout,
}: {
  data: ClientPortalData;
  onLogout: () => void;
}) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [formErr, setFormErr] = useState("");
  const [page, setPage] = usePersistedTab("client-portal", "home", ["home", "plans", "invoices", "payments", "tickets", "account"] as const) as [
    ClientPage,
    (next: ClientPage) => void,
  ];
  const [sidebarOpen, setSidebarOpenState] = useState(() => getSidebarOpen());
  const qc = useQueryClient();

  const branding = useQuery({
    queryKey: ["public-tenant-branding", data.tenant_slug],
    queryFn: () =>
      api<{
        app_name?: string;
        name?: string;
        logo_url?: string | null;
        favicon_url?: string | null;
      }>(`/api/public/tenants/${encodeURIComponent(data.tenant_slug || "")}`),
    enabled: Boolean(data.tenant_slug),
    retry: false,
  });

  const appName = (branding.data?.name || branding.data?.app_name || data.tenant_name || data.tenant_slug || "Portal").trim();
  const logoUrl = branding.data?.logo_url;
  const faviconUrl = branding.data?.favicon_url;

  useEffect(() => {
    applyBrandingMeta({
      appName,
      faviconUrl,
      titleSuffix: pageTitles[page] || "Portal Pelanggan",
      separator: "-",
    });
  }, [appName, faviconUrl, page]);

  const accounts = data.customers?.length
    ? data.customers
    : data.customer
      ? [{ id: data.customer.id || "", full_name: data.customer.full_name, phone: data.customer.phone, customer_code: data.customer.customer_code }]
      : [];
  const multi = accounts.length > 1;
  const [pwAccount, setPwAccount] = useState("");
  const [payInv, setPayInv] = useState<PayableInvoice | null>(null);
  const [changePlanSub, setChangePlanSub] = useState<PortalSub | null>(null);
  const [changePlanInitialId, setChangePlanInitialId] = useState("");
  const [ticketAccount, setTicketAccount] = useState("");
  const [ticketSubject, setTicketSubject] = useState("");
  const [ticketDesc, setTicketDesc] = useState("");
  const [ticketCategory, setTicketCategory] = useState("outage");
  const [ticketPriority, setTicketPriority] = useState("normal");
  const pwAccountId = pwAccount || accounts[0]?.id || "";
  const accountLabel = (code?: string, name?: string) =>
    code ? `${code}${name ? ` · ${name}` : ""}` : name || "—";
  const portalHeaders: HeadersInit | undefined = data.portal_token
    ? { Authorization: `Bearer ${data.portal_token}` }
    : undefined;
  const ticketAccountId = ticketAccount || accounts[0]?.id || "";

  const ticketsQ = useQuery({
    queryKey: ["portal-tickets", data.tenant_slug],
    queryFn: () => api<{ data: PortalTicket[] }>("/api/portal/tickets", { headers: portalHeaders }),
    enabled: Boolean(data.portal_token),
    retry: false,
  });

  const subsQ = useQuery({
    queryKey: ["portal-subscriptions", data.tenant_slug],
    queryFn: () => api<{ data: PortalSub[] }>("/api/portal/subscriptions", { headers: portalHeaders }),
    enabled: Boolean(data.portal_token),
    retry: false,
    refetchOnWindowFocus: true,
    refetchInterval: 15000,
  });
  const subscriptions = subsQ.isSuccess ? (subsQ.data?.data ?? []) : (data.subscriptions ?? []);

  const invoicesQ = useQuery({
    queryKey: ["portal-invoices", data.tenant_slug],
    queryFn: () =>
      api<{ data: NonNullable<ClientPortalData["invoices"]> }>("/api/portal/invoices", { headers: portalHeaders }),
    enabled: Boolean(data.portal_token),
    retry: false,
    refetchOnWindowFocus: true,
    refetchInterval: 15000,
  });
  const invoices = invoicesQ.isSuccess ? (invoicesQ.data?.data ?? []) : (data.invoices ?? []);

  const paymentsQ = useQuery({
    queryKey: ["portal-payments", data.tenant_slug],
    queryFn: () =>
      api<{ data: NonNullable<ClientPortalData["payments"]> }>("/api/portal/payments", { headers: portalHeaders }),
    enabled: Boolean(data.portal_token),
    retry: false,
    refetchOnWindowFocus: true,
    refetchInterval: 15000,
  });
  const payments = paymentsQ.isSuccess ? (paymentsQ.data?.data ?? []) : (data.payments ?? []);

  useEffect(() => {
    const session = getClientSession<ClientPortalData>() || data;
    setClientSession({
      ...session,
      ...(subsQ.data?.data ? { subscriptions: subsQ.data.data } : {}),
      ...(invoicesQ.data?.data ? { invoices: invoicesQ.data.data } : {}),
      ...(paymentsQ.data?.data ? { payments: paymentsQ.data.data } : {}),
    });
  }, [data, invoicesQ.data, paymentsQ.data, subsQ.data]);

  const plansQ = useQuery({
    queryKey: ["portal-plans", data.tenant_slug],
    queryFn: () => api<{ data: PortalPlan[] }>("/api/portal/plans", { headers: portalHeaders }),
    enabled: Boolean(data.portal_token),
    retry: false,
  });
  const portalPlans = Array.isArray(plansQ.data?.data) ? plansQ.data.data : [];
  const currentPlanIds = new Set(subscriptions.map((s) => s.plan_id).filter((id): id is string => Boolean(id)));
  const changeableSubs = subscriptions.filter((s) => canChangePortalPlan(s.status) && s.id);

  function openChangePlan(sub: PortalSub, planId?: string) {
    setChangePlanInitialId(planId || "");
    setChangePlanSub(sub);
  }

  function subForPlan(plan: PortalPlan): PortalSub | null {
    const typed = changeableSubs.find(
      (s) => s.plan_id !== plan.id && (!s.service_type || !plan.service_type || s.service_type === plan.service_type),
    );
    if (typed) return typed;
    return changeableSubs.find((s) => s.plan_id !== plan.id) ?? null;
  }

  const createTicket = useMutation({
    mutationFn: () =>
      api<PortalTicket>("/api/portal/tickets", {
        method: "POST",
        headers: portalHeaders,
        body: JSON.stringify({
          customer_id: ticketAccountId || undefined,
          subject: ticketSubject.trim(),
          description: ticketDesc.trim(),
          category: ticketCategory,
          priority: ticketPriority,
        }),
      }),
    onSuccess: () => {
      setTicketSubject("");
      setTicketDesc("");
      void qc.invalidateQueries({ queryKey: ["portal-tickets", data.tenant_slug] });
      void toastSuccess("Keluhan terkirim");
    },
    onError: (e: Error) => void toastError(e.message || "Gagal kirim keluhan"),
  });

  function logout() {
    clearClientSession();
    onLogout();
  }

  function toggleSidebar() {
    setSidebarOpenState((prev) => {
      const next = !prev;
      setSidebarOpen(next);
      return next;
    });
  }

  function startPay(inv: PayableInvoice) {
    if (!inv.id) {
      void toastError("Tagihan tidak memiliki ID. Silakan login ulang.");
      return;
    }
    if (!data.portal_token) {
      void toastError("Sesi portal lama. Keluar lalu login ulang untuk membayar.");
      return;
    }
    if (!isInvoiceUnpaid(inv)) return;
    setPayInv(inv);
  }

  // Cancel any pending checkout and forget the saved method so the picker shows
  // again next time. Ignores "no pending payment" responses.
  async function resetPayMethod(inv: PayableInvoice) {
    clearSavedPayMethod(data.tenant_slug);
    if (inv.id && data.portal_token) {
      try {
        await api(`/api/portal/invoices/${inv.id}/payment-intent/cancel`, {
          method: "POST",
          headers: portalHeaders,
        });
      } catch {
        /* no pending payment — fine */
      }
    }
    void toastSuccess("Metode pembayaran direset. Silakan pilih ulang saat bayar.");
  }

  function downloadInvoice(inv: PayableInvoice) {
    if (!inv.id) return;
    void apiDownload(`/api/portal/invoices/${inv.id}/pdf`, `${inv.invoice_number || inv.id}.pdf`, {
      token: data.portal_token,
    }).catch((e: Error) => void toastError(e.message || "Gagal unduh invoice"));
  }

  async function onChangePassword(e: React.FormEvent) {
    e.preventDefault();
    setFormErr("");
    const phone = data.customer?.phone?.trim() || "";
    const slug = data.tenant_slug?.trim() || "";
    if (!phone || !slug) {
      setFormErr("Sesi portal tidak lengkap. Silakan login ulang.");
      return;
    }
    if (multi && !pwAccountId) {
      setFormErr("Pilih akun yang passwordnya diubah.");
      return;
    }
    if (newPassword.length < 6) {
      setFormErr("Password baru minimal 6 karakter.");
      return;
    }
    if (newPassword !== confirmPassword) {
      setFormErr("Konfirmasi password tidak cocok.");
      return;
    }
    setBusy(true);
    try {
      await api("/api/portal/change-password", {
        method: "POST",
        body: JSON.stringify({
          tenant_slug: slug,
          phone,
          current_password: currentPassword,
          new_password: newPassword,
          ...(multi && pwAccountId ? { customer_id: pwAccountId } : {}),
        }),
      });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      void toastSuccess("Password berhasil diubah");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Gagal ganti password";
      setFormErr(msg);
      void toastError(msg);
    } finally {
      setBusy(false);
    }
  }

  const greeting = data.customer?.full_name || "Pelanggan";
  const isolirSubs = subscriptions.filter((s) => isIsolirStatus(s.status));
  const unpaidInvoices = invoices.filter(isInvoiceUnpaid);
  const unpaidTotal = unpaidInvoices.reduce((sum, i) => sum + invoiceRemaining(i), 0);
  const firstUnpaid = unpaidInvoices[0] ?? null;

  const invoiceRows = invoices.map((i) => {
    const unpaid = isInvoiceUnpaid(i);
    const paidSomething = i.status === "paid" || (i.paid_amount ?? 0) > 0;
    const action = (
      <span className="flex flex-wrap items-center justify-end gap-1.5">
        {unpaid ? (
          <>
            <button type="button" className="btn whitespace-nowrap" onClick={() => startPay(i)}>
              Bayar sekarang
            </button>
            {hasSavedPayMethod(data.tenant_slug) ? (
              <IconButton label="Batalkan / ganti metode" onClick={() => void resetPayMethod(i)}>
                <IconBan />
              </IconButton>
            ) : null}
          </>
        ) : null}
        {paidSomething ? (
          <IconButton label="Unduh invoice" onClick={() => downloadInvoice(i)}>
            <IconDownload />
          </IconButton>
        ) : null}
        {!unpaid && !paidSomething ? <span className="text-[var(--muted)]">—</span> : null}
      </span>
    );
    return multi
      ? [
          accountLabel(i.customer_code, i.customer_name),
          i.invoice_number,
          formatRp(i.total_amount),
          i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—",
          invoiceStatusLabel(i.status),
          action,
        ]
      : [
          i.invoice_number,
          formatRp(i.total_amount),
          i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—",
          invoiceStatusLabel(i.status),
          action,
        ];
  });

  const paymentRows = payments.map((p) =>
    multi
      ? [
          accountLabel(p.customer_code, p.customer_name),
          p.invoice_number || "—",
          p.paid_at || p.created_at ? new Date(p.paid_at || p.created_at!).toLocaleString("id-ID") : "—",
          formatRp(p.amount),
          paymentMethodLabel(p.method),
          paymentStatusLabel(p.status),
        ]
      : [
          p.invoice_number || "—",
          p.paid_at || p.created_at ? new Date(p.paid_at || p.created_at!).toLocaleString("id-ID") : "—",
          formatRp(p.amount),
          paymentMethodLabel(p.method),
          paymentStatusLabel(p.status),
        ],
  );

  return (
    <div className={`app-shell app-shell--portal${sidebarOpen ? "" : " is-sidebar-collapsed"}`}>
      <aside className="app-sidebar" aria-label="Navigasi portal pelanggan">
        <div className="app-sidebar-brand">
          <img src={logoUrl || DEFAULT_BRAND_LOGO} alt="" className="app-sidebar-logo object-contain" />
          <div className="app-sidebar-brand-text">
            <p className="text-[10px] font-medium text-[var(--stone)]">Portal pelanggan</p>
            <h1 className="text-sm font-bold">{appName}</h1>
          </div>
        </div>
        <nav className="flex-1 px-3 py-2">
          <div className="app-nav-group">
            <p className="app-nav-label">Menu</p>
            <div className="space-y-0.5">
              {CLIENT_PAGES.map((n) => (
                <button
                  key={n.id}
                  type="button"
                  title={n.label}
                  aria-label={n.label}
                  onClick={() => setPage(n.id)}
                  className={`app-nav-item ${page === n.id ? "is-active" : ""}`}
                >
                  <span className="opacity-80">{n.icon}</span>
                  <span className="app-nav-item-label">{n.label}</span>
                </button>
              ))}
            </div>
          </div>
        </nav>
        <div className="app-sidebar-foot">
          <div className="app-user-chip">
            <div className="app-user-avatar">{(greeting.trim()[0] || "P").toUpperCase()}</div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold">{greeting}</p>
              <p className="truncate text-xs text-[var(--muted)]">{data.customer?.phone || data.customer?.customer_code || ""}</p>
            </div>
            <IconButton label="Keluar" onClick={logout}>
              <IconLogout />
            </IconButton>
          </div>
        </div>
      </aside>

      <div className="app-main">
        <header className="app-header">
          <div className="app-header-start">
            <IconButton
              className="app-header-sidebar-toggle"
              label={sidebarOpen ? "Sembunyikan sidebar" : "Tampilkan sidebar"}
              onClick={toggleSidebar}
            >
              {sidebarOpen ? <PanelLeftClose /> : <PanelLeft />}
            </IconButton>
            <Breadcrumb className="app-header-breadcrumb">
              <BreadcrumbList>
                <BreadcrumbItem>
                  <button
                    type="button"
                    className="inline-flex items-center gap-1.5 text-[var(--muted)]"
                    onClick={() => setPage("home")}
                    title="Beranda"
                    aria-label="Beranda"
                  >
                    <Home className="size-4" />
                    <span className="sr-only">Beranda</span>
                  </button>
                </BreadcrumbItem>
                <BreadcrumbSeparator />
                <BreadcrumbItem>
                  <BreadcrumbPage className="truncate">{pageTitles[page]}</BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>
          </div>
          <div className="flex items-center gap-2">
            <ThemeToggle />
            <span className="portal-header-logout">
              <IconButton label="Keluar" onClick={logout}>
                <IconLogout />
              </IconButton>
            </span>
          </div>
        </header>

        <main className="app-content">
          {page === "home" ? (
            <div className="grid gap-6">
              <div>
                <p className="text-sm text-[var(--muted)]">{appName}</p>
                <h2 className="text-xl font-semibold">Halo, {greeting}</h2>
                {multi ? (
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {accounts.map((a) => (
                      <span
                        key={a.id || a.customer_code}
                        className="rounded-full border border-[var(--border)] bg-[var(--panel)] px-2 py-0.5 text-xs text-[var(--muted)]"
                      >
                        {accountLabel(a.customer_code, a.full_name)}
                      </span>
                    ))}
                  </div>
                ) : null}
              </div>
              {isolirSubs.length > 0 || unpaidInvoices.length > 0 ? (
                <div
                  className="panel-card p-4"
                  style={{
                    borderColor: isolirSubs.length > 0
                      ? "color-mix(in srgb, var(--danger) 35%, var(--border))"
                      : "color-mix(in srgb, var(--warn, #b7791f) 40%, var(--border))",
                    background: isolirSubs.length > 0
                      ? "color-mix(in srgb, var(--danger) 7%, var(--panel))"
                      : "color-mix(in srgb, var(--warn, #b7791f) 8%, var(--panel))",
                  }}
                >
                  <div className="flex items-start gap-3">
                    <span
                      className="mt-0.5"
                      style={{ color: isolirSubs.length > 0 ? "var(--danger)" : "var(--warn, #b7791f)" }}
                      aria-hidden
                    >
                      <IconShield />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-semibold">
                        {isolirSubs.length > 0 ? "Layanan diisolir" : "Tagihan belum dibayar"}
                      </p>
                      <p className="mt-1 text-sm text-[var(--muted)]">
                        {isolirSubs.length > 0
                          ? isolirSubs.length === 1
                            ? "Akun berikut sedang diisolir. Bayar tagihan agar koneksi dipulihkan."
                            : `${isolirSubs.length} akun sedang diisolir. Bayar tagihan agar koneksi dipulihkan.`
                          : "Ada tagihan yang belum dibayar. Segera lakukan pembayaran agar layanan tidak diisolir."}
                      </p>
                      {isolirSubs.length > 0 ? (
                        <ul className="mt-2 grid gap-1 text-sm">
                          {isolirSubs.map((s) => (
                            <li key={`${s.customer_code || ""}-${s.username}`}>
                              <span className="font-medium">{s.username}</span>
                              {s.plan_name ? <span className="text-[var(--muted)]"> · {s.plan_name}</span> : null}
                              {multi && (s.customer_code || s.customer_name) ? (
                                <span className="text-[var(--muted)]"> · {accountLabel(s.customer_code, s.customer_name)}</span>
                              ) : null}
                            </li>
                          ))}
                        </ul>
                      ) : null}
                      {unpaidTotal > 0 ? (
                        <p className="mt-2 text-sm font-semibold">Tagihan terbuka {formatRp(unpaidTotal)}</p>
                      ) : isolirSubs.length > 0 ? (
                        <p className="mt-2 text-sm text-[var(--muted)]">
                          Tidak ada tagihan terbuka. Hubungi admin jika internet masih terisolir.
                        </p>
                      ) : null}
                      <div className="mt-3 flex flex-wrap gap-2">
                        {firstUnpaid ? (
                          <button type="button" className="btn" onClick={() => startPay(firstUnpaid)}>
                            Bayar sekarang
                          </button>
                        ) : null}
                        <button type="button" className="btn-ghost" onClick={() => setPage("invoices")}>
                          Lihat tagihan
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              ) : null}
              <Section title="Paket / Langganan">
                <div className="portal-table-desktop">
                  <Table
                    columns={multi ? ["Akun", "Username", "Paket", "Status"] : ["Username", "Paket", "Status"]}
                    rows={subscriptions.map((s) =>
                      multi
                        ? [accountLabel(s.customer_code, s.customer_name), s.username, s.plan_name, subscriptionStatusLabel(s.status)]
                        : [s.username, s.plan_name, subscriptionStatusLabel(s.status)],
                    )}
                  />
                </div>
                <div className="portal-cards-mobile">
                  {subscriptions.length === 0 ? (
                    <p className="text-sm text-[var(--muted)]">Belum ada langganan.</p>
                  ) : (
                    subscriptions.map((s) => (
                      <article key={s.id || `${s.customer_code || ""}-${s.username}`} className="portal-item-card">
                        <p className="text-sm font-semibold">{s.username}</p>
                        <p className="text-xs text-[var(--muted)]">{s.plan_name || "—"}</p>
                        {multi ? <p className="text-xs text-[var(--muted)]">{accountLabel(s.customer_code, s.customer_name)}</p> : null}
                        <p className="text-sm">{subscriptionStatusLabel(s.status)}</p>
                      </article>
                    ))
                  )}
                </div>
              </Section>
              <Section title="Paket tersedia">
                {plansQ.isLoading ? (
                  <p className="text-sm text-[var(--muted)]">Memuat paket…</p>
                ) : (
                  <PortalPlanCatalog plans={portalPlans} currentPlanIds={currentPlanIds} />
                )}
                {portalPlans.length > 0 ? (
                  <p className="mt-3 text-sm text-[var(--muted)]">
                    Upgrade atau downgrade lewat menu{" "}
                    <button type="button" className="font-medium text-[var(--accent)] underline-offset-2 hover:underline" onClick={() => setPage("plans")}>
                      Paket
                    </button>
                    .
                  </p>
                ) : null}
              </Section>
            </div>
          ) : null}

          {page === "plans" ? (
            <div className="grid gap-6">
              <Section title="Langganan Anda">
                {subscriptions.length === 0 ? (
                  <p className="text-sm text-[var(--muted)]">Belum ada langganan.</p>
                ) : (
                  <div className="grid gap-3">
                    {subscriptions.map((s) => (
                      <article key={s.id || `${s.customer_code || ""}-${s.username}`} className="portal-plan-card">
                        <div className="min-w-0">
                          <p className="text-sm font-semibold">{s.plan_name || "—"}</p>
                          <p className="text-xs text-[var(--muted)]">
                            {s.username}
                            {multi ? ` · ${accountLabel(s.customer_code, s.customer_name)}` : ""}
                            {" · "}
                            {subscriptionStatusLabel(s.status)}
                          </p>
                        </div>
                        {canChangePortalPlan(s.status) && s.id ? (
                          <button type="button" className="btn shrink-0" onClick={() => openChangePlan(s)}>
                            Ganti paket
                          </button>
                        ) : null}
                      </article>
                    ))}
                  </div>
                )}
              </Section>
              <Section title="Paket tersedia">
                {plansQ.isLoading ? (
                  <p className="text-sm text-[var(--muted)]">Memuat paket…</p>
                ) : (
                  <PortalPlanCatalog
                    plans={portalPlans}
                    currentPlanIds={currentPlanIds}
                    onPick={
                      changeableSubs.length
                        ? (plan) => {
                            const sub = subForPlan(plan);
                            if (!sub) {
                              void toastError("Tidak ada langganan yang bisa diganti ke paket ini.");
                              return;
                            }
                            openChangePlan(sub, plan.id);
                          }
                        : undefined
                    }
                  />
                )}
              </Section>
            </div>
          ) : null}

          {page === "invoices" ? (
            <Section title="Tagihan">
              <div className="portal-table-desktop">
                <Table
                  columns={multi ? ["Akun", "Nomor", "Total", "Jatuh tempo", "Status", "Aksi"] : ["Nomor", "Total", "Jatuh tempo", "Status", "Aksi"]}
                  rows={invoiceRows}
                />
              </div>
              <div className="portal-cards-mobile">
                {invoices.length === 0 ? (
                  <p className="text-sm text-[var(--muted)]">Belum ada tagihan.</p>
                ) : (
                  invoices.map((i) => {
                    const unpaid = isInvoiceUnpaid(i);
                    return (
                      <article key={i.id || i.invoice_number} className="portal-item-card">
                        <div className="flex items-start justify-between gap-2">
                          <p className="text-sm font-semibold">{i.invoice_number}</p>
                          <p className="text-xs text-[var(--muted)]">{invoiceStatusLabel(i.status)}</p>
                        </div>
                        {multi ? <p className="text-xs text-[var(--muted)]">{accountLabel(i.customer_code, i.customer_name)}</p> : null}
                        <p className="text-base font-bold">{formatRp(i.total_amount)}</p>
                        <p className="text-xs text-[var(--muted)]">
                          Jatuh tempo {i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—"}
                        </p>
                        {unpaid ? (
                          <button type="button" className="btn" onClick={() => startPay(i)}>
                            Bayar sekarang
                          </button>
                        ) : null}
                      </article>
                    );
                  })
                )}
              </div>
            </Section>
          ) : null}

          {page === "payments" ? (
            <Section title="Riwayat pembayaran">
              <div className="portal-table-desktop">
                <Table
                  columns={multi ? ["Akun", "Tagihan", "Tanggal", "Jumlah", "Metode", "Status"] : ["Tagihan", "Tanggal", "Jumlah", "Metode", "Status"]}
                  rows={paymentRows}
                />
              </div>
              <div className="portal-cards-mobile">
                {payments.length === 0 ? (
                  <p className="text-sm text-[var(--muted)]">Belum ada pembayaran.</p>
                ) : (
                  payments.map((p, idx) => (
                    <article key={`${p.invoice_number || ""}-${p.created_at || p.paid_at || idx}`} className="portal-item-card">
                      <div className="flex items-start justify-between gap-2">
                        <p className="text-sm font-semibold">{p.invoice_number || "Pembayaran"}</p>
                        <p className="text-xs text-[var(--muted)]">{paymentStatusLabel(p.status)}</p>
                      </div>
                      {multi ? <p className="text-xs text-[var(--muted)]">{accountLabel(p.customer_code, p.customer_name)}</p> : null}
                      <p className="text-base font-bold">{formatRp(p.amount)}</p>
                      <p className="text-xs text-[var(--muted)]">
                        {paymentMethodLabel(p.method)}
                        {" · "}
                        {p.paid_at || p.created_at ? new Date(p.paid_at || p.created_at!).toLocaleString("id-ID") : "—"}
                      </p>
                    </article>
                  ))
                )}
              </div>
            </Section>
          ) : null}

          {page === "tickets" ? (
            <div className="grid gap-6">
              <Section title="Buat keluhan">
                <form
                  className="grid max-w-md gap-3"
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (!data.portal_token) {
                      void toastError("Sesi portal lama. Keluar lalu login ulang.");
                      return;
                    }
                    if (multi && !ticketAccountId) {
                      void toastError("Pilih akun untuk keluhan ini.");
                      return;
                    }
                    createTicket.mutate();
                  }}
                >
                  {multi ? (
                    <label className="grid gap-1 text-sm">
                      <span className="text-[var(--muted)]">Akun</span>
                      <select
                        className="input"
                        value={ticketAccountId}
                        onChange={(e) => setTicketAccount(e.target.value)}
                      >
                        {accounts.map((a) => (
                          <option key={a.id || a.customer_code} value={a.id}>
                            {accountLabel(a.customer_code, a.full_name)}
                          </option>
                        ))}
                      </select>
                    </label>
                  ) : null}
                  <label className="grid gap-1 text-sm">
                    <span className="text-[var(--muted)]">Kategori</span>
                    <select className="input" value={ticketCategory} onChange={(e) => setTicketCategory(e.target.value)}>
                      {TICKET_CATEGORIES.map((c) => (
                        <option key={c.id} value={c.id}>
                          {c.label}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="grid gap-1 text-sm">
                    <span className="text-[var(--muted)]">Prioritas</span>
                    <select className="input" value={ticketPriority} onChange={(e) => setTicketPriority(e.target.value)}>
                      {TICKET_PRIORITIES.map((c) => (
                        <option key={c.id} value={c.id}>
                          {c.label}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="grid gap-1 text-sm">
                    <span className="text-[var(--muted)]">Subjek</span>
                    <input
                      className="input"
                      value={ticketSubject}
                      onChange={(e) => setTicketSubject(e.target.value)}
                      required
                      placeholder="Contoh: Internet putus sejak pagi"
                    />
                  </label>
                  <label className="grid gap-1 text-sm">
                    <span className="text-[var(--muted)]">Deskripsi</span>
                    <textarea
                      className="input min-h-28"
                      value={ticketDesc}
                      onChange={(e) => setTicketDesc(e.target.value)}
                      required
                      placeholder="Jelaskan keluhan Anda"
                    />
                  </label>
                  <button className="btn w-fit" disabled={createTicket.isPending}>
                    {createTicket.isPending ? "Mengirim…" : "Kirim keluhan"}
                  </button>
                </form>
              </Section>
              <Section title="Keluhan saya">
                {ticketsQ.isLoading ? (
                  <p className="text-sm text-[var(--muted)]">Memuat…</p>
                ) : (ticketsQ.data?.data ?? []).length === 0 ? (
                  <p className="text-sm text-[var(--muted)]">Belum ada keluhan.</p>
                ) : (
                  <div className="grid gap-3">
                    {(ticketsQ.data?.data ?? []).map((t) => (
                      <PortalTicketCard
                        key={t.id}
                        ticket={t}
                        headers={portalHeaders}
                        tenantSlug={data.tenant_slug || ""}
                        multi={multi}
                        categoryLabel={TICKET_CATEGORIES.find((c) => c.id === t.category)?.label || t.category}
                        priorityLabel={TICKET_PRIORITIES.find((c) => c.id === t.priority)?.label || t.priority}
                      />
                    ))}
                  </div>
                )}
              </Section>
            </div>
          ) : null}

          {page === "account" ? (
            <Section title="Ganti password">
              <form className="grid max-w-md gap-3" onSubmit={onChangePassword}>
                {multi ? (
                  <label className="grid gap-1 text-sm">
                    <span className="text-[var(--muted)]">Akun</span>
                    <select
                      className="input"
                      value={pwAccountId}
                      onChange={(e) => setPwAccount(e.target.value)}
                    >
                      {accounts.map((a) => (
                        <option key={a.id || a.customer_code} value={a.id}>
                          {accountLabel(a.customer_code, a.full_name)}
                        </option>
                      ))}
                    </select>
                  </label>
                ) : null}
                <SecretInput
                  placeholder="Password saat ini"
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  required
                  autoComplete="current-password"
                />
                <SecretInput
                  placeholder="Password baru (min 6)"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  required
                  minLength={6}
                  autoComplete="new-password"
                />
                <SecretInput
                  placeholder="Ulangi password baru"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  required
                  minLength={6}
                  autoComplete="new-password"
                />
                <button className="btn w-fit" disabled={busy}>
                  {busy ? "Menyimpan..." : "Simpan password"}
                </button>
                {formErr && <p className="text-sm text-[var(--danger)]">{formErr}</p>}
              </form>
            </Section>
          ) : null}
        </main>
      </div>

      <nav className="portal-bottom-nav" aria-label="Menu portal">
        {CLIENT_PAGES.map((n) => (
          <button
            key={n.id}
            type="button"
            className={`portal-bottom-nav-item${page === n.id ? " is-active" : ""}`}
            onClick={() => setPage(n.id)}
          >
            <span className="opacity-80">{n.icon}</span>
            <span>{n.label}</span>
          </button>
        ))}
      </nav>

      <PortalChangePlanDialog
        sub={changePlanSub}
        headers={portalHeaders}
        initialPlanId={changePlanInitialId}
        onClose={() => {
          setChangePlanSub(null);
          setChangePlanInitialId("");
        }}
        onChanged={(invoice) => {
          setChangePlanSub(null);
          setChangePlanInitialId("");
          void qc.invalidateQueries({ queryKey: ["portal-subscriptions", data.tenant_slug] });
          void qc.invalidateQueries({ queryKey: ["portal-plans", data.tenant_slug] });
          void qc.invalidateQueries({ queryKey: ["portal-invoices", data.tenant_slug] });
          if (invoice?.id) {
            setPayInv(invoice);
          }
        }}
      />

      <PortalPayHost
        invoice={payInv}
        tenantSlug={data.tenant_slug}
        headers={portalHeaders}
        onClose={() => setPayInv(null)}
        onPaid={() => {
          setPayInv(null);
          void toastSuccess("Pembayaran diterima");
          void qc.invalidateQueries({ queryKey: ["portal-invoices", data.tenant_slug] });
          void qc.invalidateQueries({ queryKey: ["portal-payments", data.tenant_slug] });
          void qc.invalidateQueries({ queryKey: ["portal-subscriptions", data.tenant_slug] });
        }}
      />
    </div>
  );
}
