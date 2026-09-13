import { lazy, type ComponentType } from "react";

const RELOAD_KEY = "drp_chunk_reloaded_at";
/** Jeda minimum antar auto-reload (mencegah loop bila jaringan putus). */
const RELOAD_COOLDOWN_MS = 15_000;

/**
 * true bila error berasal dari gagal memuat JS chunk (biasanya setelah
 * deploy: shell lama meminta file hash lama yang sudah tidak ada).
 */
export function isChunkLoadError(e: unknown): boolean {
  const msg = e instanceof Error ? e.message : String(e ?? "");
  return /dynamically imported module|failed to fetch dynamically|loading chunk|chunkloaderror|failed to fetch|importing a module script/i.test(
    msg,
  );
}

/**
 * Pengganti React.lazy: saat chunk gagal dimuat (deploy baru), lakukan satu
 * kali hard reload agar shell + chunk versi baru terambil, lalu render normal.
 * Error lain diteruskan ke ErrorBoundary seperti biasa.
 */
export function lazyWithReload<T extends ComponentType<any>>( // eslint-disable-line @typescript-eslint/no-explicit-any
  importer: () => Promise<{ default: T }>,
) {
  return lazy(async () => {
    try {
      const mod = await importer();
      try {
        sessionStorage.removeItem(RELOAD_KEY);
      } catch {
        /* abaikan (mode privat) */
      }
      return mod;
    } catch (e) {
      if (isChunkLoadError(e)) {
        try {
          const last = Number(sessionStorage.getItem(RELOAD_KEY) || 0);
          if (!Number.isFinite(last) || Date.now() - last > RELOAD_COOLDOWN_MS) {
            sessionStorage.setItem(RELOAD_KEY, String(Date.now()));
            window.location.reload();
            // Jangan resolve saat reload berjalan.
            return new Promise<{ default: T }>(() => {});
          }
        } catch {
          /* abaikan, teruskan error ke boundary */
        }
      }
      throw e;
    }
  });
}
