/** Persist admin sidebar page + in-page tabs across refresh. */

import { useCallback, useState } from "react";

const adminPageKey = (slug: string) => `drp_admin_page:${slug}`;
const odpClusterKey = (slug: string) => `drp_odp_cluster:${slug}`;
const ipPoolClusterKey = (slug: string) => `drp_ippool_cluster:${slug}`;
const SIDEBAR_OPEN_KEY = "drp_sidebar_open";

function tabStorageKey(id: string) {
  let scope = "app";
  try {
    const seg = window.location.pathname.split("/").filter(Boolean)[0];
    if (seg) scope = seg;
  } catch {
    /* ignore */
  }
  return `drp_tab:${scope}:${id}`;
}

export function getLastTab(id: string): string | null {
  try {
    return localStorage.getItem(tabStorageKey(id));
  } catch {
    return null;
  }
}

export function setLastTab(id: string, value: string) {
  try {
    localStorage.setItem(tabStorageKey(id), value);
  } catch {
    /* ignore quota / private mode */
  }
}

/** In-page tab that survives refresh. */
export function usePersistedTab(id: string, fallback: string, allowed?: readonly string[]): [string, (next: string) => void] {
  const [tab, setTabState] = useState(() => {
    const raw = getLastTab(id);
    if (raw && (!allowed || allowed.includes(raw))) return raw;
    return fallback;
  });
  const setTab = useCallback(
    (next: string) => {
      setTabState(next);
      setLastTab(id, next);
    },
    [id],
  );
  return [tab, setTab];
}

export function getLastAdminPage(slug: string): string | null {
  try {
    const v = localStorage.getItem(adminPageKey(slug));
    if (v === "branding") return "general";
    if (v === "tech") return "tickets";
    if (v === "subscriptions") return "customers";
    if (v === "ipam") return "ip-pool";
    return v;
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

export function getLastIpPoolCluster(slug: string): string | null {
  try {
    return localStorage.getItem(ipPoolClusterKey(slug));
  } catch {
    return null;
  }
}

export function setLastIpPoolCluster(slug: string, clusterId: string) {
  try {
    localStorage.setItem(ipPoolClusterKey(slug), clusterId);
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
