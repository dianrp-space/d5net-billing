const ADMIN_TOKEN = "drp_admin_token";
const CLIENT_SESSION = "drp_client_session";

export function getToken(): string | null {
  return localStorage.getItem(ADMIN_TOKEN);
}

export function setToken(token: string) {
  localStorage.setItem(ADMIN_TOKEN, token);
}

export function clearToken() {
  localStorage.removeItem(ADMIN_TOKEN);
}

export function getClientSession<T = unknown>(): T | null {
  // localStorage agar sesi tetap ada saat kembali dari tab baru payment gateway.
  const raw =
    localStorage.getItem(CLIENT_SESSION) ?? sessionStorage.getItem(CLIENT_SESSION);
  if (!raw) return null;
  try {
    const data = JSON.parse(raw) as T;
    // Migrasi sekali jalan: pindahkan sesi lama ke localStorage.
    if (!localStorage.getItem(CLIENT_SESSION)) {
      localStorage.setItem(CLIENT_SESSION, raw);
      sessionStorage.removeItem(CLIENT_SESSION);
    }
    return data;
  } catch {
    return null;
  }
}

export function setClientSession(data: unknown) {
  const raw = JSON.stringify(data);
  localStorage.setItem(CLIENT_SESSION, raw);
  sessionStorage.removeItem(CLIENT_SESSION);
}

export function clearClientSession() {
  localStorage.removeItem(CLIENT_SESSION);
  sessionStorage.removeItem(CLIENT_SESSION);
}

/** Fired when access token cannot be refreshed — App should redirect to login. */
export const AUTH_EXPIRED_EVENT = "drp:auth-expired";

let refreshInFlight: Promise<boolean> | null = null;

async function refreshAccessToken(): Promise<boolean> {
  if (refreshInFlight) return refreshInFlight;
  refreshInFlight = (async () => {
    try {
      const res = await fetch("/api/auth/refresh", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      });
      if (!res.ok) return false;
      const data = (await res.json()) as { access_token?: string };
      if (!data.access_token) return false;
      setToken(data.access_token);
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
  opts?: { skipAuthRefresh?: boolean },
): Promise<T> {
  const doFetch = async () => {
    const headers = new Headers(init.headers);
    if (init.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
    if (!headers.has("Authorization")) {
      const token = getToken();
      if (token) headers.set("Authorization", `Bearer ${token}`);
    }
    return fetch(path, { ...init, headers, credentials: "include" });
  };

  let res = await doFetch();

  if (res.status === 401 && shouldAttemptRefresh(path, opts?.skipAuthRefresh)) {
    const refreshed = await refreshAccessToken();
    if (refreshed) {
      res = await doFetch();
    } else {
      localStorage.removeItem(ADMIN_TOKEN);
      window.dispatchEvent(new CustomEvent(AUTH_EXPIRED_EVENT));
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
  opts?: { fieldName?: string; onProgress?: (percent: number) => void },
): Promise<T> {
  const field = opts?.fieldName || "file";
  const onProgress = opts?.onProgress;

  const uploadOnce = (token: string | null) =>
    new Promise<{ ok: boolean; status: number; text: string }>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open("POST", path);
      xhr.withCredentials = true;
      if (token) xhr.setRequestHeader("Authorization", `Bearer ${token}`);
      xhr.upload.onprogress = (ev) => {
        if (!onProgress || !ev.lengthComputable || ev.total <= 0) return;
        onProgress(Math.min(99, Math.round((ev.loaded / ev.total) * 100)));
      };
      xhr.onload = () => resolve({ ok: xhr.status >= 200 && xhr.status < 300, status: xhr.status, text: xhr.responseText || "" });
      xhr.onerror = () => reject(new Error("Jaringan gagal saat upload"));
      xhr.onabort = () => reject(new Error("Upload dibatalkan"));
      const body = new FormData();
      body.append(field, file);
      xhr.send(body);
    });

  let token = getToken();
  let res = await uploadOnce(token);
  if (res.status === 401 && shouldAttemptRefresh(path, false)) {
    const refreshed = await refreshAccessToken();
    if (refreshed) {
      token = getToken();
      res = await uploadOnce(token);
    } else {
      localStorage.removeItem(ADMIN_TOKEN);
      window.dispatchEvent(new CustomEvent(AUTH_EXPIRED_EVENT));
    }
  }
  if (!res.ok) {
    try {
      const j = JSON.parse(res.text) as { error?: string; detail?: string };
      throw new Error(j.error || j.detail || res.text || `HTTP ${res.status}`);
    } catch (e) {
      if (e instanceof SyntaxError) throw new Error(res.text || `HTTP ${res.status}`);
      throw e;
    }
  }
  onProgress?.(100);
  if (!res.text) return undefined as T;
  return JSON.parse(res.text) as T;
}

/** Authenticated binary download (PDF/CSV) — bare &lt;a href&gt; cannot send Bearer. */
export async function apiDownload(
  path: string,
  filename: string,
  opts?: { token?: string },
): Promise<void> {
  // When an explicit token is provided (e.g. portal session) use it directly and
  // skip admin refresh logic.
  const explicitToken = opts?.token?.trim();
  const doFetch = async () => {
    const headers = new Headers();
    const token = explicitToken || getToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
    return fetch(path, { headers, credentials: "include" });
  };
  if (explicitToken) {
    const res = await doFetch();
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
    return;
  }
  let res = await doFetch();
  if (res.status === 401 && shouldAttemptRefresh(path, false)) {
    const refreshed = await refreshAccessToken();
    if (refreshed) res = await doFetch();
    else {
      localStorage.removeItem(ADMIN_TOKEN);
      window.dispatchEvent(new CustomEvent(AUTH_EXPIRED_EVENT));
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
