const ADMIN_TOKEN = "drp_admin_token";
const ADMIN_SLUG = "drp_admin_slug";
const PLATFORM_TOKEN = "drp_platform_token";
const CLIENT_SESSION = "drp_client_session";

export function getToken(): string | null {
  return localStorage.getItem(ADMIN_TOKEN);
}

export function setToken(token: string) {
  localStorage.setItem(ADMIN_TOKEN, token);
}

export function clearToken() {
  localStorage.removeItem(ADMIN_TOKEN);
  localStorage.removeItem(ADMIN_SLUG);
}

export function getAdminSlug(): string | null {
  return localStorage.getItem(ADMIN_SLUG);
}

export function setAdminSlug(slug: string) {
  localStorage.setItem(ADMIN_SLUG, slug);
}

export function getPlatformToken(): string | null {
  return localStorage.getItem(PLATFORM_TOKEN);
}

export function setPlatformToken(token: string) {
  localStorage.setItem(PLATFORM_TOKEN, token);
}

export function clearPlatformToken() {
  localStorage.removeItem(PLATFORM_TOKEN);
}

export function getClientSession<T = unknown>(): T | null {
  const raw = sessionStorage.getItem(CLIENT_SESSION);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as T;
  } catch {
    return null;
  }
}

export function setClientSession(data: unknown) {
  sessionStorage.setItem(CLIENT_SESSION, JSON.stringify(data));
}

export function clearClientSession() {
  sessionStorage.removeItem(CLIENT_SESSION);
}

/** Fired when access token cannot be refreshed — App should redirect to login. */
export const AUTH_EXPIRED_EVENT = "drp:auth-expired";

let refreshInFlight: Promise<boolean> | null = null;

async function refreshAccessToken(platform: boolean): Promise<boolean> {
  if (refreshInFlight) return refreshInFlight;
  refreshInFlight = (async () => {
    try {
      const body = platform ? {} : { tenant_slug: getAdminSlug() || undefined };
      const res = await fetch("/api/auth/refresh", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!res.ok) return false;
      const data = (await res.json()) as { access_token?: string };
      if (!data.access_token) return false;
      if (platform) setPlatformToken(data.access_token);
      else setToken(data.access_token);
      return true;
    } catch {
      return false;
    } finally {
      refreshInFlight = null;
    }
  })();
  return refreshInFlight;
}

function shouldAttemptRefresh(path: string, skip?: boolean): boolean {
  if (skip) return false;
  if (path.startsWith("/api/auth/")) return false;
  if (path.startsWith("/api/public/")) return false;
  if (path.startsWith("/api/portal/")) return false;
  return true;
}

export async function api<T>(
  path: string,
  init: RequestInit = {},
  opts?: { platform?: boolean; skipAuthRefresh?: boolean },
): Promise<T> {
  const platform = Boolean(opts?.platform);

  const doFetch = async () => {
    const headers = new Headers(init.headers);
    if (!headers.has("Content-Type")) headers.set("Content-Type", "application/json");
    const token = platform ? getPlatformToken() : getToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
    return fetch(path, { ...init, headers, credentials: "include" });
  };

  let res = await doFetch();

  if (res.status === 401 && shouldAttemptRefresh(path, opts?.skipAuthRefresh)) {
    const refreshed = await refreshAccessToken(platform);
    if (refreshed) {
      res = await doFetch();
    } else {
      if (platform) {
        clearPlatformToken();
      } else {
        localStorage.removeItem(ADMIN_TOKEN);
      }
      window.dispatchEvent(new CustomEvent(AUTH_EXPIRED_EVENT, { detail: { platform } }));
    }
  }

  if (!res.ok) {
    const text = await res.text();
    try {
      const j = JSON.parse(text) as {
        detail?: string;
        title?: string;
        error?: string;
        errors?: { message?: string; location?: string }[];
      };
      const parts = (j.errors ?? [])
        .map((e) => [e.location, e.message].filter(Boolean).join(": "))
        .filter(Boolean);
      const detail = parts.length ? `${j.detail || j.title || "validation failed"} (${parts.join("; ")})` : j.detail || j.title || j.error || text;
      throw new Error(detail);
    } catch (e) {
      if (e instanceof SyntaxError) throw new Error(text || res.statusText);
      throw e;
    }
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

/** Multipart upload — do not set Content-Type (browser sets boundary). */
export async function apiUpload<T>(
  path: string,
  file: File,
  opts?: { platform?: boolean; fieldName?: string },
): Promise<T> {
  const platform = Boolean(opts?.platform);
  const field = opts?.fieldName || "file";
  const doFetch = async () => {
    const headers = new Headers();
    const token = platform ? getPlatformToken() : getToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
    const body = new FormData();
    body.append(field, file);
    return fetch(path, { method: "POST", headers, body, credentials: "include" });
  };
  let res = await doFetch();
  if (res.status === 401 && shouldAttemptRefresh(path, false)) {
    const refreshed = await refreshAccessToken(platform);
    if (refreshed) res = await doFetch();
    else {
      if (platform) clearPlatformToken();
      else localStorage.removeItem(ADMIN_TOKEN);
      window.dispatchEvent(new CustomEvent(AUTH_EXPIRED_EVENT, { detail: { platform } }));
    }
  }
  if (!res.ok) {
    const text = await res.text();
    try {
      const j = JSON.parse(text) as { error?: string; detail?: string };
      throw new Error(j.error || j.detail || text);
    } catch (e) {
      if (e instanceof SyntaxError) throw new Error(text || res.statusText);
      throw e;
    }
  }
  return res.json() as Promise<T>;
}

/** Authenticated binary download (PDF/CSV) — bare &lt;a href&gt; cannot send Bearer. */
export async function apiDownload(
  path: string,
  filename: string,
  opts?: { platform?: boolean },
): Promise<void> {
  const platform = Boolean(opts?.platform);
  const doFetch = async () => {
    const headers = new Headers();
    const token = platform ? getPlatformToken() : getToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
    return fetch(path, { headers, credentials: "include" });
  };
  let res = await doFetch();
  if (res.status === 401 && shouldAttemptRefresh(path, false)) {
    const refreshed = await refreshAccessToken(platform);
    if (refreshed) res = await doFetch();
    else {
      if (platform) clearPlatformToken();
      else localStorage.removeItem(ADMIN_TOKEN);
      window.dispatchEvent(new CustomEvent(AUTH_EXPIRED_EVENT, { detail: { platform } }));
    }
  }
  if (!res.ok) {
    const text = await res.text();
    try {
      const j = JSON.parse(text) as { error?: string; detail?: string };
      throw new Error(j.error || j.detail || text || res.statusText);
    } catch (e) {
      if (e instanceof SyntaxError) throw new Error(text || res.statusText);
      throw e;
    }
  }
  const blob = await res.blob();
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  a.click();
  URL.revokeObjectURL(a.href);
}
