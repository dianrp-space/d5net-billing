/** Coarse role permission keys (Roles UI) → admin page ids they unlock. */
export const PERMISSION_PAGE_MAP: Record<string, string[]> = {
  "*": ["*"],
  dashboard: ["dashboard"],
  customers: ["customers", "leads", "resellers", "sla-report"],
  leads: ["leads"],
  billing: ["plans", "invoices", "accounting", "sla-report"],
  network: ["clusters", "routers", "subscriptions", "ipam", "odp", "vouchers", "customers"],
  ops: ["dashboard", "tickets", "leads"],
  tickets: ["dashboard", "tickets", "leads"],
  // legacy key from older roles — fold into tickets (portal teknisi/WO digabung ke tiket)
  tech: ["dashboard", "tickets", "leads"],
  settings: ["branding", "isolir-template", "jobs", "notifications", "roles", "users", "backup", "webhooks", "payment-gw", "messaging-gw", "sla-report"],
  // page-level aliases (if checked directly in custom roles)
  plans: ["plans"],
  invoices: ["invoices"],
  accounting: ["accounting"],
  "sla-report": ["sla-report"],
  resellers: ["resellers"],
  clusters: ["clusters"],
  routers: ["routers"],
  // secrets now live under pelanggan; keep alias for older roles
  subscriptions: ["customers", "subscriptions"],
  ipam: ["ipam"],
  odp: ["odp"],
  vouchers: ["vouchers"],
  branding: ["branding"],
  "isolir-template": ["isolir-template"],
  jobs: ["jobs"],
  notifications: ["notifications"],
  roles: ["roles"],
  users: ["users"],
  backup: ["backup"],
  webhooks: ["webhooks"],
  "payment-gw": ["payment-gw"],
  "messaging-gw": ["messaging-gw"],
};

export type MePermissions = {
  role_slug: string;
  role_name: string;
  permissions: string[];
  full_name?: string;
  email?: string;
};

export function expandAllowedPages(permissions: string[] | undefined | null): Set<string> | "all" {
  if (!permissions || permissions.length === 0) {
    return new Set();
  }
  if (permissions.includes("*")) {
    return "all";
  }
  const pages = new Set<string>();
  for (const key of permissions) {
    const mapped = PERMISSION_PAGE_MAP[key];
    if (mapped) {
      for (const p of mapped) {
        if (p === "*") return "all";
        pages.add(p);
      }
    } else {
      // unknown key: treat as page id itself
      pages.add(key);
    }
  }
  return pages;
}

export function canAccessPage(permissions: string[] | undefined | null, page: string): boolean {
  const allowed = expandAllowedPages(permissions);
  if (allowed === "all") return true;
  return allowed.has(page);
}

export function firstAllowedPage(permissions: string[] | undefined | null, preferred = "dashboard"): string {
  if (canAccessPage(permissions, preferred)) return preferred;
  const allowed = expandAllowedPages(permissions);
  if (allowed === "all") return preferred;
  if (allowed.size === 0) return preferred;
  // stable order: prefer dashboard then alphabetical
  if (allowed.has("dashboard")) return "dashboard";
  return [...allowed].sort()[0];
}

/** Admin/dispatcher may create & assign tickets. Field teknisi cannot. */
export function canDispatchOps(permissions: string[] | undefined | null): boolean {
  if (!permissions?.length) return false;
  return (
    permissions.includes("*") ||
    permissions.includes("settings") ||
    permissions.includes("customers") ||
    permissions.includes("billing")
  );
}
