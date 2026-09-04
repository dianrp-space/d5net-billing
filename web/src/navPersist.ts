/** Persist admin sidebar page + in-page tabs across refresh. */

const adminPageKey = (slug: string) => `drp_admin_page:${slug}`;
const odpClusterKey = (slug: string) => `drp_odp_cluster:${slug}`;

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
