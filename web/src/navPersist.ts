/** Persist admin sidebar page + in-page tabs across refresh. */

const adminPageKey = (slug: string) => `drp_admin_page:${slug}`;
const odpClusterKey = (slug: string) => `drp_odp_cluster:${slug}`;
const SIDEBAR_OPEN_KEY = "drp_sidebar_open";

export function getLastAdminPage(slug: string): string | null {
  try {
    return localStorage.getItem(adminPageKey(slug));
  } catch {
    return null;
  }
}

export function setLastAdminPage(slug: string, page: string) {
  try {
    localStorage.setItem(adminPageKey(slug), page);
  } catch {
    /* ignore quota / private mode */
  }
}

export function getLastOdpCluster(slug: string): string | null {
  try {
    return localStorage.getItem(odpClusterKey(slug));
  } catch {
    return null;
  }
}

export function setLastOdpCluster(slug: string, clusterId: string) {
  try {
    localStorage.setItem(odpClusterKey(slug), clusterId);
  } catch {
    /* ignore */
  }
}

export function getSidebarOpen(): boolean {
  try {
    const v = localStorage.getItem(SIDEBAR_OPEN_KEY);
    if (v === null) return true;
    return v !== "0" && v !== "false";
  } catch {
    return true;
  }
}

export function setSidebarOpen(open: boolean) {
  try {
    localStorage.setItem(SIDEBAR_OPEN_KEY, open ? "1" : "0");
  } catch {
    /* ignore */
  }
}
