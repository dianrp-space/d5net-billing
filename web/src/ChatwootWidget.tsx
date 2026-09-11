import { useEffect, useRef } from "react";
import { api } from "./api";

declare global {
  interface Window {
    chatwootSDK?: {
      run: (opts: { websiteToken: string; baseUrl: string }) => void;
      setUser: (id: string | number, attrs?: Record<string, unknown>) => void;
    };
  }
}

type ChatwootPublic = { base_url: string; website_token: string };

export type ChatwootIdentity = {
  id: string;
  name?: string;
  phone?: string;
  email?: string;
};

/** Live-chat Chatwoot di halaman publik/portal. Diam bila tidak dikonfigurasi. */
export function ChatwootWidget({ identity }: { identity?: ChatwootIdentity | null }) {
  const identityRef = useRef(identity);
  identityRef.current = identity;

  useEffect(() => {
    let cancelled = false;
    let timer: number | undefined;
    (async () => {
      let cfg: ChatwootPublic | null = null;
      try {
        const b = await api<{ chatwoot?: ChatwootPublic }>("/api/public/branding");
        cfg = b.chatwoot || null;
      } catch {
        return;
      }
      if (cancelled || !cfg?.base_url || !cfg?.website_token) return;
      const base = cfg.base_url.replace(/\/$/, "");
      try {
        await new Promise<void>((resolve, reject) => {
          if (window.chatwootSDK) return resolve();
          const s = document.createElement("script");
          s.src = `${base}/packs/js/sdk.js`;
          s.async = true;
          s.dataset.chatwootWidget = "1";
          s.onload = () => resolve();
          s.onerror = () => reject(new Error("gagal memuat widget chat"));
          document.head.appendChild(s);
        });
      } catch {
        return;
      }
      if (cancelled || !window.chatwootSDK) return;
      try {
        window.chatwootSDK.run({ websiteToken: cfg.website_token, baseUrl: base });
      } catch {
        return;
      }
      const id = identityRef.current;
      if (id) {
        const apply = () => {
          try {
            window.chatwootSDK?.setUser(id.id, {
              ...(id.name ? { name: id.name } : {}),
              ...(id.phone ? { phone_number: id.phone } : {}),
              ...(id.email ? { email: id.email } : {}),
            });
          } catch {
            /* abaikan */
          }
        };
        window.addEventListener("chatwoot:ready", apply, { once: true });
        timer = window.setTimeout(apply, 4000);
      }
    })();
    return () => {
      cancelled = true;
      if (timer !== undefined) window.clearTimeout(timer);
      document.querySelectorAll("script[data-chatwoot-widget]").forEach((el) => el.remove());
      document.getElementById("chatwoot_live_chat_widget")?.remove();
    };
  }, []);

  return null;
}
