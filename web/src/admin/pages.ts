export type AdminPage =
  | "dashboard"
  | "customers"
  | "discounts"
  | "clusters"
  | "plans"
  | "offers"
  | "subscriptions"
  | "invoices"
  | "payments"
  | "routers"
  | "ip-pool"
  | "tickets"
  | "sla-report"
  | "odp"
  | "coverage"
  | "vouchers"
  | "leads"
  | "accounting"
  | "resellers"
  | "tech"
  | "general"
  | "invoice-format"
  | "isolir-template"
  | "jobs"
  | "notifications"
  | "roles"
  | "users"
  | "webhooks"
  | "payment-gw"
  | "messaging-gw"
  | "backup";

export const ADMIN_PAGES: AdminPage[] = [
  "dashboard",
  "customers",
  "discounts",
  "clusters",
  "plans",
  "offers",
  "subscriptions",
  "invoices",
  "payments",
  "routers",
  "ip-pool",
  "tickets",
  "sla-report",
  "odp",
  "coverage",
  "vouchers",
  "leads",
  "accounting",
  "resellers",
  "tech",
  "general",
  "invoice-format",
  "isolir-template",
  "jobs",
  "notifications",
  "roles",
  "users",
  "webhooks",
  "payment-gw",
  "messaging-gw",
  "backup",
];

export function normalizeAdminPage(value: string): string {
  if (value === "branding") return "general";
  if (value === "ipam") return "ip-pool";
  return value;
}

export function isAdminPage(value: string): value is AdminPage {
  return (ADMIN_PAGES as string[]).includes(normalizeAdminPage(value));
}

export const pageTitles: Record<AdminPage, string> = {
  dashboard: "Overview",
  customers: "Pelanggan",
  discounts: "Diskon",
  clusters: "Cluster / POP",
  plans: "Paket",
  offers: "Paket per Cluster",
  subscriptions: "Secrets",
  invoices: "Tagihan",
  payments: "Pembayaran",
  routers: "Router",
  "ip-pool": "IP Pool",
  tickets: "Tiket",
  "sla-report": "Laporan SLA",
  odp: "MAP FTTH",
  coverage: "Coverage",
  vouchers: "Voucher",
  leads: "Lead",
  accounting: "Akunting",
  resellers: "Reseller & Komisi",
  tech: "Tiket",
  general: "Umum",
  "invoice-format": "Format Invoice",
  "isolir-template": "Template Isolir",
  jobs: "Cronjob",
  notifications: "Notifikasi",
  roles: "Roles",
  users: "Users",
  webhooks: "Webhook",
  "payment-gw": "Payment Gateway",
  "messaging-gw": "Messaging Gateway",
  backup: "Backup / Restore",
};
