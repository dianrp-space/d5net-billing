import { useEffect, useRef, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Home, PanelLeft, PanelLeftClose } from "lucide-react";
import { api, apiDownload, clearClientSession, getClientSession, setClientSession } from "./api";
import { applyBrandingMeta, DEFAULT_BRAND_LOGO } from "./branding";
import type { ClientPortalData, PortalCustomer } from "./TenantLogin";
import { ClientIdCard } from "./ClientIdCard";
import { ClientBell } from "./ClientBell";
import { ChatwootWidget } from "./ChatwootWidget";
import { alertPaymentSuccess, toastError, toastSuccess } from "./swal";
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
import { clearSavedPayMethod, consumePaymentReturn, confirmPaymentAfterReturn, hasSavedPayMethod, invoiceRemaining, isInvoiceUnpaid, isIsolirStatus, paymentMethodLabel, type PayableInvoice } from "./payMethod";
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
  // Tandai body agar CSS bisa mengangkat widget Chatwoot di atas menu docking
  // (widget ditempel SDK ke document.body, di luar .app-shell--portal).
  useEffect(() => {
    document.body.classList.add("portal-dock");
    return () => {
      document.body.classList.remove("portal-dock");
    };
  }, []);
  const [page, setPage] = usePersistedTab("client-portal", "home", ["home", "plans", "invoices", "payments", "tickets", "account"] as const) as [
    ClientPage,
    (next: ClientPage) => void,
  ];
  const [sidebarOpen, setSidebarOpenState] = useState(() => getSidebarOpen());
  const qc = useQueryClient();

  const branding = useQuery({
    queryKey: ["public-branding"],
    queryFn: () =>
      api<{
        app_name?: string;
        name?: string;
        logo_url?: string | null;
        favicon_url?: string | null;
      }>("/api/public/branding"),
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
  const fullAccounts =
    data.customers?.length ? data.customers : data.customer ? [data.customer] : [];
  const multi = accounts.length > 1;
  const [pwAccount, setPwAccount] = useState("");
  const [payInv, setPayInv] = useState<PayableInvoice | null>(null);
  const paymentReturnHandled = useRef(false);
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

  // Foto profil: override lokal per akun + patch sesi portal.
  const [photoMap, setPhotoMap] = useState<Record<string, string | null>>({});
  const [photoBusy, setPhotoBusy] = useState(false);
  const [photoErr, setPhotoErr] = useState("");
  const [photoKeySel, setPhotoKeySel] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);

  const photoKeyOf = (c: { id?: string; customer_code: string }) => c.id || c.customer_code;
  function photoOf(c: PortalCustomer): string | null {
    const k = photoKeyOf(c);
    if (k && k in photoMap) return photoMap[k];
    return c.photo_url?.trim() || null;
  }
  const cardAccounts = fullAccounts.map((c) => ({ ...c, photo_url: photoOf(c) }));
  const cardCustomer = data.customer ? { ...data.customer, photo_url: photoOf(data.customer) } : null;
  const photoFirstKey = fullAccounts.length ? photoKeyOf(fullAccounts[0]) : "";
  const photoSelKey = photoKeySel || photoFirstKey;
  const photoTarget = fullAccounts.find((c) => photoKeyOf(c) === photoSelKey) || fullAccounts[0];
  const photoTargetUrl = photoTarget ? photoOf(photoTarget) : null;

  function applyPhoto(target: PortalCustomer, url: string | null) {
    const key = photoKeyOf(target);
    setPhotoMap((m) => ({ ...m, [key]: url }));
    const sess = getClientSession<ClientPortalData>();
    if (sess) {
      const same = (c: PortalCustomer) =>
        (target.id && c.id === target.id) || c.customer_code === target.customer_code;
      if (Array.isArray(sess.customers)) {
        sess.customers = sess.customers.map((c) => (same(c) ? { ...c, photo_url: url } : c));
      }
      if (sess.customer && same(sess.customer)) {
        sess.customer = { ...sess.customer, photo_url: url };
      }
      setClientSession(sess);
    }
  }

  async function uploadPhoto(file: File) {
    if (!photoTarget || photoBusy) return;
    setPhotoBusy(true);
    setPhotoErr("");
    try {
      const fd = new FormData();
      fd.append("file", file);
      if (multi) fd.append("customer_id", photoTarget.id || "");
      const res = await fetch("/api/portal/account/photo", {
        method: "POST",
        headers: data.portal_token ? { Authorization: `Bearer ${data.portal_token}` } : {},
        body: fd,
      });
      let body: { url?: string; error?: string; detail?: string } = {};
      try {
        body = (await res.json()) as typeof body;
      } catch {
        /* abaikan */
      }
      if (!res.ok || !body.url) throw new Error(body.error || body.detail || "Upload gagal");
      applyPhoto(photoTarget, body.url);
      void toastSuccess("Foto profil diperbarui");
    } catch (e: unknown) {
      setPhotoErr(e instanceof Error ? e.message : "Upload gagal");
    } finally {
      setPhotoBusy(false);
    }
  }

  async function removePhoto() {
    if (!photoTarget || photoBusy) return;
    setPhotoBusy(true);
    setPhotoErr("");
    try {
      const fd = new FormData();
      fd.append("remove", "true");
      if (multi) fd.append("customer_id", photoTarget.id || "");
      const res = await fetch("/api/portal/account/photo", {
        method: "POST",
        headers: data.portal_token ? { Authorization: `Bearer ${data.portal_token}` } : {},
        body: fd,
      });
      if (!res.ok) {
        let msg = "Hapus foto gagal";
        try {
          const body = (await res.json()) as { error?: string; detail?: string };
          msg = body.error || body.detail || msg;
        } catch {
          /* abaikan */
        }
        throw new Error(msg);
      }
      applyPhoto(photoTarget, null);
      void toastSuccess("Foto profil dihapus");
    } catch (e: unknown) {
      setPhotoErr(e instanceof Error ? e.message : "Hapus foto gagal");
    } finally {
      setPhotoBusy(false);
    }
  }

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
    if (paymentReturnHandled.current) return;
    const ret = consumePaymentReturn();
    if (!ret.returned) return;
    paymentReturnHandled.current = true;
    setPage("invoices");
    void (async () => {
      const confirmed = await confirmPaymentAfterReturn({
        resultCode: ret.resultCode,
        fetchInvoices: async () => {
          const res = await api<{ data: NonNullable<ClientPortalData["invoices"]> }>("/api/portal/invoices", {
            headers: portalHeaders,
          });
          return res.data ?? [];
        },
        fetchPaymentIntent: async (invoiceId) => {
          try {
            return await api<{ status?: string }>(`/api/portal/invoices/${invoiceId}/payment-intent`, {
              headers: portalHeaders,
            });
          } catch {
            return null;
          }
        },
        fetchPayments: async () => {
          const res = await api<{ data: NonNullable<ClientPortalData["payments"]> }>("/api/portal/payments", {
            headers: portalHeaders,
          });
          return res.data ?? [];
        },
      });
      void qc.invalidateQueries({ queryKey: ["portal-invoices", data.tenant_slug] });
      void qc.invalidateQueries({ queryKey: ["portal-payments", data.tenant_slug] });
      void qc.invalidateQueries({ queryKey: ["portal-subscriptions", data.tenant_slug] });
      if (confirmed) void alertPaymentSuccess();
    })();
    // Sekali saat kembali dari PG; jangan ikut re-render query.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data.tenant_slug, data.portal_token]);

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

  // Status payment-intent yang masih bisa dibatalkan pelanggan.
  // Intent lunas / sudah final (batal, kedaluwarsa, gagal, void) tidak ditawari tombol batal.
  function isCancellablePayment(status?: string | null) {
    const s = String(status || "").trim().toLowerCase();
    return ["pending", "created", "unpaid", "open", "waiting", "process", "processing", "initialized"].includes(s);
  }

  const [cancellingKey, setCancellingKey] = useState("");

  async function cancelPendingPayment(p: { invoice_id?: string; invoice_number?: string; created_at?: string; paid_at?: string }) {
    const invoiceId = (p.invoice_id || "").trim();
    if (!invoiceId) {
      void toastError("Pembayaran ini tidak terhubung ke tagihan.");
      return;
    }
    if (!data.portal_token) {
      void toastError("Sesi portal lama. Keluar lalu login ulang.");
      return;
    }
    const key = `${invoiceId}-${p.created_at || p.paid_at || p.invoice_number || ""}`;
    setCancellingKey(key);
    try {
      await api(`/api/portal/invoices/${invoiceId}/payment-intent/cancel`, {
        method: "POST",
        headers: portalHeaders,
      });
      clearSavedPayMethod(data.tenant_slug);
      void toastSuccess("Pembayaran pending dibatalkan.");
      void qc.invalidateQueries({ queryKey: ["portal-payments", data.tenant_slug] });
      void qc.invalidateQueries({ queryKey: ["portal-invoices", data.tenant_slug] });
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Gagal membatalkan pembayaran");
    } finally {
      setCancellingKey("");
    }
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
  const unpaidTotal = unpaidInvoices.reduce((sum, i) => {
    const payable = Math.max(0, Math.floor(Number(i.payable_amount) || 0));
    if (payable > 0) return sum + payable;
    return sum + invoiceRemaining(i);
  }, 0);
  const firstUnpaid = unpaidInvoices[0] ?? null;

  const invoiceRows = invoices.map((i) => {
    const unpaid = isInvoiceUnpaid(i);
    const paidSomething = i.status === "paid" || (i.paid_amount ?? 0) > 0;
    const itemLabel = (i.items_summary || "").trim() || "—";
    const paidWhen = i.paid_at ? new Date(i.paid_at).toLocaleString("id-ID") : "—";
    const adminFee = Math.max(0, Math.floor(Number(i.admin_fee) || 0));
    const amountLabel =
      adminFee > 0
        ? `${formatRp(i.total_amount)} + admin ${formatRp(adminFee)}`
        : formatRp(i.total_amount);
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
        <IconButton label="Unduh invoice" onClick={() => downloadInvoice(i)}>
          <IconDownload />
        </IconButton>
      </span>
    );
    return multi
      ? [
          accountLabel(i.customer_code, i.customer_name),
          i.invoice_number,
          itemLabel,
          amountLabel,
          i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—",
          paidWhen,
          invoiceStatusLabel(i.status),
          action,
        ]
      : [
          i.invoice_number,
          itemLabel,
          amountLabel,
          i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—",
          paidWhen,
          invoiceStatusLabel(i.status),
          action,
        ];
  });

  const paymentRows = payments.map((p) => {
    const cancellable = isCancellablePayment(p.status) && Boolean((p.invoice_id || "").trim());
    const cancelKey = `${p.invoice_id || ""}-${p.created_at || p.paid_at || p.invoice_number || ""}`;
    const action =
      cancellable ? (
        <span className="flex flex-wrap items-center justify-end gap-1.5">
          <button
            type="button"
            className="btn-ghost whitespace-nowrap text-sm"
            disabled={cancellingKey === cancelKey}
            onClick={() => void cancelPendingPayment(p)}
          >
            {cancellingKey === cancelKey ? "Membatalkan…" : "Batalkan"}
          </button>
        </span>
      ) : (
        <span className="text-[var(--muted)]">—</span>
      );
    return multi
      ? [
          accountLabel(p.customer_code, p.customer_name),
          p.invoice_number || "—",
          (p.items_summary || "").trim() || "—",
          p.paid_at || p.created_at ? new Date(p.paid_at || p.created_at!).toLocaleString("id-ID") : "—",
          formatRp(p.amount),
          paymentMethodLabel(p.method),
          paymentStatusLabel(p.status),
          action,
        ]
      : [
          p.invoice_number || "—",
          (p.items_summary || "").trim() || "—",
          p.paid_at || p.created_at ? new Date(p.paid_at || p.created_at!).toLocaleString("id-ID") : "—",
          formatRp(p.amount),
          paymentMethodLabel(p.method),
          paymentStatusLabel(p.status),
          action,
        ];
  });

  return (
    <div className={`app-shell app-shell--portal${sidebarOpen ? "" : " is-sidebar-collapsed"}`}>
      <ChatwootWidget
        identity={
          fullAccounts[0]
            ? {
                id: fullAccounts[0].id || fullAccounts[0].customer_code,
                name: fullAccounts[0].full_name,
                phone: fullAccounts[0].phone,
                email: fullAccounts[0].email || undefined,
              }
            : null
        }
      />
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
            {(() => {
              const first = fullAccounts[0];
              const url = first ? photoOf(first) : null;
              return url ? (
                <img src={url} alt="" className="app-user-avatar app-user-avatar--img object-cover" />
              ) : (
                <div className="app-user-avatar">{(greeting.trim()[0] || "P").toUpperCase()}</div>
              );
            })()}
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
            <ClientBell
              invoices={invoices}
              payments={payments}
              subscriptions={subscriptions}
              multi={multi}
              onNavigatePage={setPage}
            />
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
              <ClientIdCard
                customer={cardCustomer}
                accounts={cardAccounts}
                providerName={appName}
                logoUrl={logoUrl}
              />
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
                        <span className="shrink-0 text-[10px] font-semibold uppercase tracking-wide text-[var(--accent)]">
                          Paket Anda
                        </span>
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
                  columns={multi ? ["Akun", "Nomor", "Item", "Total", "Jatuh tempo", "Dibayar", "Status", "Aksi"] : ["Nomor", "Item", "Total", "Jatuh tempo", "Dibayar", "Status", "Aksi"]}
                  rows={invoiceRows}
                />
              </div>
              <div className="portal-cards-mobile">
                {invoices.length === 0 ? (
                  <p className="text-sm text-[var(--muted)]">Belum ada tagihan.</p>
                ) : (
                  invoices.map((i) => {
                    const unpaid = isInvoiceUnpaid(i);
                    const adminFee = Math.max(0, Math.floor(Number(i.admin_fee) || 0));
                    const payable = Math.max(0, Math.floor(Number(i.payable_amount) || 0));
                    return (
                      <article key={i.id || i.invoice_number} className="portal-item-card">
                        <div className="flex items-start justify-between gap-2">
                          <p className="text-sm font-semibold">{i.invoice_number}</p>
                          <p className="text-xs text-[var(--muted)]">{invoiceStatusLabel(i.status)}</p>
                        </div>
                        {multi ? <p className="text-xs text-[var(--muted)]">{accountLabel(i.customer_code, i.customer_name)}</p> : null}
                        {(i.items_summary || "").trim() ? (
                          <p className="text-xs text-[var(--muted)]">{i.items_summary}</p>
                        ) : null}
                        <p className="text-base font-bold">{formatRp(adminFee > 0 && payable > 0 ? payable : i.total_amount)}</p>
                        {adminFee > 0 ? (
                          <p className="text-[11px] text-[var(--muted)]">
                            Tagihan {formatRp(i.total_amount)} + biaya admin {formatRp(adminFee)}
                          </p>
                        ) : null}
                        <p className="text-xs text-[var(--muted)]">
                          Jatuh tempo {i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—"}
                          {i.paid_at ? ` · Dibayar ${new Date(i.paid_at).toLocaleString("id-ID")}` : ""}
                        </p>
                        <div className="flex flex-wrap items-center gap-1.5">
                          {unpaid ? (
                            <>
                              <button type="button" className="btn" onClick={() => startPay(i)}>
                                Bayar sekarang
                              </button>
                              {hasSavedPayMethod(data.tenant_slug) ? (
                                <IconButton label="Batalkan / ganti metode" onClick={() => void resetPayMethod(i)}>
                                  <IconBan />
                                </IconButton>
                              ) : null}
                            </>
                          ) : null}
                          <IconButton label="Unduh invoice" onClick={() => downloadInvoice(i)}>
                            <IconDownload />
                          </IconButton>
                        </div>
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
                  columns={multi ? ["Akun", "Tagihan", "Item", "Tanggal", "Jumlah", "Metode", "Status", "Aksi"] : ["Tagihan", "Item", "Tanggal", "Jumlah", "Metode", "Status", "Aksi"]}
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
                      {(p.items_summary || "").trim() ? (
                        <p className="text-xs text-[var(--muted)]">{p.items_summary}</p>
                      ) : null}
                      <p className="text-base font-bold">{formatRp(p.amount)}</p>
                      <p className="text-xs text-[var(--muted)]">
                        {paymentMethodLabel(p.method)}
                        {" · "}
                        {p.paid_at || p.created_at ? new Date(p.paid_at || p.created_at!).toLocaleString("id-ID") : "—"}
                      </p>
                      {isCancellablePayment(p.status) && (p.invoice_id || "").trim() ? (
                        <button
                          type="button"
                          className="btn-ghost w-fit text-sm"
                          disabled={cancellingKey === `${p.invoice_id || ""}-${p.created_at || p.paid_at || p.invoice_number || ""}`}
                          onClick={() => void cancelPendingPayment(p)}
                        >
                          {cancellingKey === `${p.invoice_id || ""}-${p.created_at || p.paid_at || p.invoice_number || ""}`
                            ? "Membatalkan…"
                            : "Batalkan pembayaran"}
                        </button>
                      ) : null}
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
            <Section title="Foto profil">
              {photoTarget ? (
                <div className="flex flex-wrap items-center gap-4">
                  {photoTargetUrl ? (
                    <img src={photoTargetUrl} alt="" className="h-20 w-20 rounded-2xl object-cover" />
                  ) : (
                    <span
                      className="flex h-20 w-20 items-center justify-center rounded-2xl text-2xl font-bold text-white"
                      style={{ background: "linear-gradient(140deg, var(--accent), color-mix(in srgb, var(--accent) 55%, #000))" }}
                      aria-hidden
                    >
                      {(photoTarget.full_name.trim()[0] || "P").toUpperCase()}
                    </span>
                  )}
                  <div className="grid min-w-0 flex-1 gap-2">
                    {multi ? (
                      <label className="grid max-w-md gap-1 text-sm">
                        <span className="text-[var(--muted)]">Akun</span>
                        <select
                          className="input"
                          value={photoSelKey}
                          onChange={(e) => setPhotoKeySel(e.target.value)}
                        >
                          {fullAccounts.map((a) => (
                            <option key={photoKeyOf(a)} value={photoKeyOf(a)}>
                              {accountLabel(a.customer_code, a.full_name)}
                            </option>
                          ))}
                        </select>
                      </label>
                    ) : (
                      <p className="text-sm font-medium">{accountLabel(photoTarget.customer_code, photoTarget.full_name)}</p>
                    )}
                    <div className="flex flex-wrap gap-2">
                      <button
                        type="button"
                        className="btn-ghost"
                        disabled={photoBusy}
                        onClick={() => fileRef.current?.click()}
                      >
                        {photoBusy ? "Mengunggah..." : "Pilih foto..."}
                      </button>
                      {photoTargetUrl ? (
                        <button
                          type="button"
                          className="btn-ghost"
                          disabled={photoBusy}
                          onClick={() => void removePhoto()}
                        >
                          Hapus foto
                        </button>
                      ) : null}
                    </div>
                    <input
                      ref={fileRef}
                      type="file"
                      accept="image/*"
                      className="hidden"
                      onChange={(e) => {
                        const f = e.target.files?.[0];
                        e.target.value = "";
                        if (f) void uploadPhoto(f);
                      }}
                    />
                    <p className="text-xs text-[var(--muted)]">JPG / PNG / WebP, otomatis dikompresi.</p>
                    {photoErr && <p className="text-sm text-[var(--danger)]">{photoErr}</p>}
                  </div>
                </div>
              ) : (
                <p className="text-sm text-[var(--muted)]">Belum ada akun.</p>
              )}
            </Section>
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
          void qc.invalidateQueries({ queryKey: ["portal-invoices", data.tenant_slug] });
          void qc.invalidateQueries({ queryKey: ["portal-payments", data.tenant_slug] });
          void qc.invalidateQueries({ queryKey: ["portal-subscriptions", data.tenant_slug] });
          void alertPaymentSuccess();
        }}
      />
    </div>
  );
}
