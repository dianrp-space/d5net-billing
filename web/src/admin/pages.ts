export type AdminPage =
  | "dashboard"
  | "customers"
  | "clusters"
  | "plans"
  | "subscriptions"
  | "invoices"
  | "routers"
  | "ipam"
  | "tickets"
  | "sla-report"
  | "odp"
  | "vouchers"
  | "leads"
  | "accounting"
  | "resellers"
  | "tech"
  | "branding"
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
  "clusters",
  "plans",
  "subscriptions",
  "invoices",
  "routers",
  "ipam",
  "tickets",
  "sla-report",
  "odp",
  "vouchers",
  "leads",
  "accounting",
  "resellers",
  "tech",
  "branding",
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

export function isAdminPage(value: string): value is AdminPage {
  return (ADMIN_PAGES as string[]).includes(value);
}

export const pageTitles: Record<AdminPage, string> = {
  dashboard: "Overview",
  customers: "Pelanggan",
  clusters: "Cluster / POP",
  plans: "Paket",
  subscriptions: "Secrets",
  invoices: "Tagihan",
  routers: "Router",
  ipam: "IP Pool",
  tickets: "Tiket",
  "sla-report": "Laporan SLA",
  odp: "ODP / FTTH",
  vouchers: "Voucher",
  leads: "Lead",
  accounting: "Akunting",
  resellers: "Reseller & Komisi",
  tech: "Tiket",
  branding: "Umum",
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
