import { useDeferredValue, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { toastError, toastSuccess } from "./swal";
import { Button, Input, SearchableSelect, Section } from "./ui";

type IsolirNetwork = {
  profile_name: string;
  router_id?: string | null;
  ip_pool_id?: string | null;
  pool_name?: string;
  pool_ranges?: string;
  portal_base_url: string;
};

type IsolirSettings = {
  network: IsolirNetwork;
  html: string;
  isolir_url: string;
  docs_hint: string;
};

type RouterRow = { id: string; name: string; is_active: boolean };
type PoolRow = {
  id: string;
  name: string;
  network: string;
  router_id?: string | null;
  router_name?: string | null;
};

const emptyNetwork: IsolirNetwork = {
  profile_name: "isolir",
  router_id: "",
  ip_pool_id: "",
  portal_base_url: typeof window !== "undefined" ? window.location.origin : "",
};

function buildDocsHint(isolirURL: string, poolLabel: string) {
  const url = isolirURL || "{portal_base_url}/{tenantSlug}/client";
  return `URL isolir (portal pelanggan):
${url}

Pool: ${poolLabel || "(pilih IP pool isolir)"}

Redirect Web Proxy ke /{slug}/client.
IP → Web Proxy: enable proxy 8080, allow host billing, redirect HTTP ke URL isolir
(RouterOS 7: action=redirect + action-data; v6: deny + redirect-to),
NAT tcp/80 → 8080, allow DNS + HTTPS portal (address-list FQDN, bukan IP publik).`;
}

function defaultPreviewHTML(appName: string, logoURL: string, loginURL: string) {
  const logo = logoURL
    ? `<img src="${logoURL}" alt="" style="max-height:48px;margin-bottom:1rem"/>`
    : "";
  return `<!DOCTYPE html>
<html lang="id"><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>${appName} — Isolir</title>
<style>
body{margin:0;font-family:system-ui,sans-serif;background:#F7F6F2;color:#1a1a14;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:1.5rem}
.card{background:#fff;border:1px solid #e5e2d9;border-radius:12px;padding:2rem;max-width:420px;width:100%;box-shadow:0 8px 24px rgba(0,0,0,.06);text-align:center}
h1{font-size:1.35rem;margin:0 0 .5rem;color:#5A5A40}
p{color:#5c584c;line-height:1.5;margin:0 0 1.25rem}
a.btn{display:inline-block;background:#5A5A40;color:#fff;text-decoration:none;padding:.7rem 1.25rem;border-radius:8px;font-weight:600}
</style></head><body><div class="card">${logo}
<h1>Layanan diisolir</h1>
<p>Internet Anda dibatasi karena ada tagihan yang belum lunas. Silakan masuk untuk melihat tagihan dan membayar.</p>
<a class="btn" href="${loginURL}">Login &amp; bayar tagihan</a>
</div></body></html>`;
}

function fillPlaceholders(raw: string, vars: Record<string, string>) {
  let out = raw;
  for (const [k, v] of Object.entries(vars)) {
    out = out.split(`{{${k}}}`).join(v);
  }
  return out;
}

export function IsolirTemplatePage({ tenantSlug = "" }: { tenantSlug?: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["settings-isolir"],
    queryFn: () => api<IsolirSettings>("/api/settings/isolir"),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    retry: 1,
  });
  const routersQ = useQuery({
    queryKey: ["routers"],
    queryFn: () => api<RouterRow[]>("/api/routers"),
    staleTime: 60_000,
  });
  const poolsQ = useQuery({
    queryKey: ["ip-pools"],
    queryFn: () => api<PoolRow[]>("/api/ip-pools"),
    staleTime: 60_000,
  });
  const brandingQ = useQuery({
    queryKey: ["public-tenant-branding", tenantSlug],
    queryFn: () =>
      api<{ app_name?: string; logo_url?: string | null; name?: string }>(
        `/api/public/tenants/${encodeURIComponent(tenantSlug)}`,
      ),
    enabled: Boolean(tenantSlug),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  });

  const [network, setNetwork] = useState<IsolirNetwork>(emptyNetwork);
  const [html, setHtml] = useState("");
  const [hydrated, setHydrated] = useState(false);
  const [showDocs, setShowDocs] = useState(false);

  useEffect(() => {
    if (!q.data || hydrated) return;
    setNetwork({
      profile_name: q.data.network.profile_name || "isolir",
      router_id: q.data.network.router_id || "",
      ip_pool_id: q.data.network.ip_pool_id || "",
      pool_name: q.data.network.pool_name,
      pool_ranges: q.data.network.pool_ranges,
      portal_base_url: q.data.network.portal_base_url || window.location.origin,
    });
    setHtml(q.data.html || "");
    setHydrated(true);
  }, [q.data, hydrated]);

  useEffect(() => {
    if (q.isError && !hydrated) {
      setNetwork((n) => ({ ...n, portal_base_url: n.portal_base_url || window.location.origin }));
      setHydrated(true);
    }
  }, [q.isError, hydrated]);

  const poolsForRouter = useMemo(() => {
    const rid = network.router_id || "";
    const all = poolsQ.data || [];
    if (!rid) return all;
    return all.filter((p) => p.router_id === rid);
  }, [poolsQ.data, network.router_id]);

  const selectedPool = useMemo(
    () => (poolsQ.data || []).find((p) => p.id === network.ip_pool_id),
    [poolsQ.data, network.ip_pool_id],
  );

  const appName = brandingQ.data?.name || brandingQ.data?.app_name || tenantSlug || "ISP";
  const logoURL = brandingQ.data?.logo_url || "";
  const loginURL = network.portal_base_url
    ? `${network.portal_base_url.replace(/\/$/, "")}/${tenantSlug || "slug"}/client`
    : `/${tenantSlug || "slug"}/client`;

  const poolLabel = selectedPool
    ? `${selectedPool.name} · ${selectedPool.network}${selectedPool.router_name ? ` · ${selectedPool.router_name}` : ""}`
    : network.pool_ranges || "";

  const docsHint = q.data?.docs_hint?.trim() || buildDocsHint(loginURL, poolLabel);

  const deferredHtml = useDeferredValue(html);
  const previewSrcDoc = useMemo(() => {
    const vars = {
      app_name: appName,
      logo_url: logoURL,
      login_url: loginURL,
      tenant_slug: tenantSlug || "slug",
    };
    const custom = deferredHtml.trim();
    if (!custom) return defaultPreviewHTML(appName, logoURL, loginURL);
    return fillPlaceholders(custom, vars);
  }, [deferredHtml, appName, logoURL, loginURL, tenantSlug]);

  const save = useMutation({
    mutationFn: () =>
      api<IsolirSettings>("/api/settings/isolir", {
        method: "PUT",
        body: JSON.stringify({
          network: {
            profile_name: network.profile_name,
            router_id: network.router_id || null,
            ip_pool_id: network.ip_pool_id || null,
            portal_base_url: network.portal_base_url,
          },
          html,
        }),
      }),
    onSuccess: (data) => {
      void qc.setQueryData(["settings-isolir"], data);
      setNetwork({
        profile_name: data.network.profile_name || "isolir",
        router_id: data.network.router_id || "",
        ip_pool_id: data.network.ip_pool_id || "",
        pool_name: data.network.pool_name,
        pool_ranges: data.network.pool_ranges,
        portal_base_url: data.network.portal_base_url || window.location.origin,
      });
      void toastSuccess("Pengaturan isolir disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const sync = useMutation({
    mutationFn: () =>
      api<{ synced: number; errors: string[] }>("/api/settings/isolir/sync", { method: "POST" }),
    onSuccess: (r) => {
      if (r.errors?.length) {
        void toastError(`Synced ${r.synced}. Error: ${r.errors.join("; ")}`);
      } else {
        void toastSuccess(`Synced ke ${r.synced} router`);
      }
    },
    onError: (e: Error) => void toastError(e.message),
  });

  return (
    <Section title="Template Isolir">
      {q.isLoading && !hydrated ? <p className="mb-3 text-sm text-[var(--muted)]">Memuat pengaturan…</p> : null}

      <div className="grid gap-6 lg:grid-cols-2 lg:items-start">
        <div className="grid min-w-0 gap-4">
          <div className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/40 p-3">
            <p className="text-xs font-semibold uppercase tracking-wide text-[var(--stone)]">URL portal pelanggan (isolir)</p>
            <code className="mt-1 block break-all text-sm text-[var(--accent)]">{loginURL}</code>
            <button
              type="button"
              className="mt-2 text-xs font-medium text-[var(--muted)] underline"
              onClick={() => setShowDocs((v) => !v)}
            >
              {showDocs ? "Sembunyikan panduan RouterOS" : "Lihat panduan Web Proxy / firewall"}
            </button>
            {showDocs ? (
              <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap rounded-md bg-[var(--panel)] p-2 text-[11px] leading-relaxed text-[var(--muted)]">
                {docsHint}
              </pre>
            ) : null}
          </div>

          <div className="grid gap-3">
            <label className="grid gap-1.5 text-sm">
              <span className="font-medium">Portal base URL</span>
              <Input
                placeholder="https://billing.example.com"
                value={network.portal_base_url}
                onChange={(e) => setNetwork({ ...network, portal_base_url: e.target.value })}
              />
              <span className="text-xs text-[var(--muted)]">
                Redirect isolir ke <code>{"{base}"}/{tenantSlug || "slug"}/client</code>
              </span>
            </label>

            <label className="grid gap-1.5 text-sm">
              <span className="font-medium">Router</span>
              <SearchableSelect
                placeholder="— Pilih router —"
                searchPlaceholder="Cari router…"
                value={network.router_id || ""}
                onValueChange={(v) =>
                  setNetwork({
                    ...network,
                    router_id: v,
                    ip_pool_id: "",
                    pool_name: "",
                    pool_ranges: "",
                  })
                }
                options={(routersQ.data || [])
                  .filter((r) => r.is_active)
                  .map((r) => ({ value: r.id, label: r.name, keywords: r.name }))}
              />
            </label>

            <label className="grid gap-1.5 text-sm">
              <span className="font-medium">IP Pool isolir (IPAM)</span>
              <SearchableSelect
                placeholder={network.router_id ? "— Pilih pool di router ini —" : "— Pilih router dulu —"}
                searchPlaceholder="Cari pool / network…"
                value={network.ip_pool_id || ""}
                disabled={!network.router_id}
                onValueChange={(v) => {
                  const p = poolsForRouter.find((x) => x.id === v);
                  setNetwork({
                    ...network,
                    ip_pool_id: v,
                    pool_name: p?.name,
                    pool_ranges: p?.network,
                  });
                }}
                options={poolsForRouter.map((p) => ({
                  value: p.id,
                  label: `${p.name} · ${p.network}`,
                  keywords: `${p.name} ${p.network}`,
                }))}
              />
              {selectedPool ? (
                <span className="text-xs text-[var(--muted)]">
                  Sync Web Proxy + profil isolir ke router ini. Worker juga menerapkan profil isolir di router langganan yang statusnya isolir (bukan hanya router ini).
                </span>
              ) : (
                <span className="text-xs text-[var(--muted)]">Buat pool khusus isolir di menu IP Pool, lalu pilih di sini.</span>
              )}
            </label>

            <label className="grid gap-1.5 text-sm">
              <span className="font-medium">Nama profil RouterOS</span>
              <Input
                value={network.profile_name}
                onChange={(e) => setNetwork({ ...network, profile_name: e.target.value })}
                placeholder="isolir"
              />
            </label>
          </div>

          <label className="grid gap-1 text-sm">
            <span className="font-medium">HTML landing (opsional)</span>
            <textarea
              className="input min-h-[140px] font-mono text-xs"
              placeholder="Kosong = template bawaan. Placeholder: {{app_name}} {{logo_url}} {{login_url}} {{tenant_slug}}"
              value={html}
              onChange={(e) => setHtml(e.target.value)}
              spellCheck={false}
            />
          </label>

          <div className="flex flex-wrap gap-2">
            <Button type="button" onClick={() => save.mutate()} disabled={save.isPending}>
              {save.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => sync.mutate()} disabled={sync.isPending}>
              {sync.isPending ? "Sync…" : "Sync ke router terpilih"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setHtml("")}>
              Reset ke bawaan
            </Button>
          </div>
        </div>

        <div className="min-w-0">
          <div className="mb-2 flex items-center justify-between gap-2">
            <p className="text-sm font-semibold">Preview render</p>
            <span className="text-xs text-[var(--muted)]">{html.trim() ? "Custom HTML" : "Template bawaan"}</span>
          </div>
          <div className="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[#F7F6F2] shadow-sm">
            <iframe
              title="Preview halaman isolir"
              className="h-[min(70vh,560px)] w-full border-0 bg-[#F7F6F2]"
              sandbox=""
              srcDoc={previewSrcDoc}
            />
          </div>
        </div>
      </div>
    </Section>
  );
}
