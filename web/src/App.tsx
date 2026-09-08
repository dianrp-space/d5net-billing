import { useEffect, useState } from "react";
import { AdminApp, isAdminPage, normalizeAdminPage, type AdminPage } from "./AdminApp";
import { AUTH_EXPIRED_EVENT, clearClientSession, getAdminSlug, getClientSession, getPlatformToken, getToken } from "./api";
import { ClientHome } from "./ClientHome";
import { Landing } from "./Landing";
import { PlatformApp } from "./PlatformApp";
import { PlatformLogin } from "./PlatformLogin";
import { TenantLogin, type ClientPortalData } from "./TenantLogin";
import { getLastAdminPage, setLastAdminPage } from "./navPersist";
import { usePath } from "./usePath";
import { TenantAccent } from "./theme";

function normalize(path: string) {
  if (path.length > 1 && path.endsWith("/")) return path.slice(0, -1);
  return path;
}

/** Top-level segments that are never tenant slugs (platform / isolir / legacy). */
const RESERVED_TOP = new Set(["login", "platform", "isolir", "admin", "client"]);
const SLUG_RE = /^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$/;

type TenantMode = "admin-login" | "client-login" | "client-home" | "admin-app";
type TenantRoute = { slug: string; mode: TenantMode; section: string; rest: string[] };

/** Tenant routes: /{slug}/login (admin) | /{slug}/client/login | /{slug} (client home) | /{slug}/{section} (admin). */
function parseTenantRoute(path: string): TenantRoute | null {
  const parts = path.slice(1).split("/").filter(Boolean);
  if (!parts.length) return null;
  if (RESERVED_TOP.has(parts[0])) return null;
  if (!SLUG_RE.test(parts[0])) return null;
  const slug = parts[0];
  const rest = parts.slice(1);
  if (rest[0] === "client" && rest[1] === "login") {
    return { slug, mode: "client-login", section: "", rest: rest.slice(2) };
  }
  if (rest[0] === "login") {
    return { slug, mode: "admin-login", section: "", rest: rest.slice(1) };
  }
  if (rest.length === 0 || rest[0] === "client") {
    return { slug, mode: "client-home", section: "", rest: [] };
  }
  return { slug, mode: "admin-app", section: rest[0], rest: rest.slice(1) };
}

/** Legacy /admin/{slug}/... and /client/{slug}/... → new paths. */
function parseLegacy(path: string): string | null {
  const m = path.match(/^\/(admin|client)\/([^/]+)(?:\/(.*))?$/);
  if (!m) return null;
  const kind = m[1];
  const slug = m[2];
  const rest = m[3] || "";
  if (slug === "login") return null;
  if (kind === "admin") return `/${slug}` + (rest ? `/${rest}` : "");
  if (rest === "login") return `/${slug}/client/login`;
  return `/${slug}`;
}

/** /{slug}/customers/{id}/secrets[/new] | /customers/{id}/gallery|galery */
function parseCustomerNestedRest(
  section: string,
  rest: string[],
): { customerId: string; kind: "secrets" | "gallery"; create?: boolean } | null {
  if (section !== "customers" || !rest[0]) return null;
  if (rest.length === 2 && rest[1] === "secrets") {
    return { customerId: rest[0], kind: "secrets", create: false };
  }
  if (rest.length === 3 && rest[1] === "secrets" && rest[2] === "new") {
    return { customerId: rest[0], kind: "secrets", create: true };
  }
  if (rest.length === 2 && (rest[1] === "gallery" || rest[1] === "galery")) {
    return { customerId: rest[0], kind: "gallery" };
  }
  return null;
}

function RouteLoading() {
  return (
    <div className="flex min-h-full items-center justify-center p-8">
      <p className="text-sm text-[var(--muted)]">Memuat…</p>
    </div>
  );
}

export default function App() {
  const { path, navigate } = usePath();
  const [, setAuthTick] = useState(0);
  const refreshAuth = () => setAuthTick((n) => n + 1);
  const p = normalize(path);
  const adminAuthed = Boolean(getToken());
  const platformAuthed = Boolean(getPlatformToken());
  const clientSession = getClientSession<ClientPortalData>();
  const storedAdminSlug = getAdminSlug();

  const tenantRoute = parseTenantRoute(p);
  const isolirMatch = p.match(/^\/isolir\/([^/]+)\/?$/);
  const isolirSlug = isolirMatch?.[1] || "";
  const clientTenantSlug = clientSession?.tenant_slug || "";

  useEffect(() => {
    const onExpired = (ev: Event) => {
      const platform = Boolean((ev as CustomEvent<{ platform?: boolean }>).detail?.platform);
      setAuthTick((n) => n + 1);
      if (platform) {
        navigate("/login");
        return;
      }
      const slug = getAdminSlug() || tenantRoute?.slug;
      navigate(slug ? `/${slug}/login` : "/");
    };
    window.addEventListener(AUTH_EXPIRED_EVENT, onExpired);
    return () => window.removeEventListener(AUTH_EXPIRED_EVENT, onExpired);
  }, [navigate, tenantRoute?.slug]);

  useEffect(() => {
    if (p === "/login" && platformAuthed) {
      navigate("/platform");
      return;
    }
    if (p === "/platform" && !platformAuthed) {
      navigate("/login");
      return;
    }
    const isolirRedirect = p.match(/^\/isolir\/([^/]+)\/?$/);
    if (isolirRedirect?.[1]) {
      navigate(`/${isolirRedirect[1]}/client`);
      return;
    }

    // Legacy admin/client forms & prefixes.
    if (p === "/admin/login") {
      navigate(storedAdminSlug ? `/${storedAdminSlug}/login` : "/");
      return;
    }
    if (p === "/client/login") {
      navigate(clientTenantSlug ? `/${clientTenantSlug}/client/login` : "/");
      return;
    }
    if (p === "/admin" || p === "/client") {
      navigate("/");
      return;
    }
    const legacy = parseLegacy(p);
    if (legacy) {
      navigate(legacy);
      return;
    }

    if (!tenantRoute) return;

    const { slug, mode, section, rest } = tenantRoute;

    if (mode === "admin-login") {
      if (adminAuthed && storedAdminSlug === slug) {
        const last = getLastAdminPage(slug);
        const s = last && isAdminPage(last) ? last : "dashboard";
        navigate(`/${slug}/${s}`);
        return;
      }
      if (clientTenantSlug === slug) {
        navigate(`/${slug}/client`);
        return;
      }
      return;
    }

    if (mode === "client-login") {
      if (clientTenantSlug === slug) {
        navigate(`/${slug}/client`);
        return;
      }
      if (adminAuthed && storedAdminSlug === slug) {
        const last = getLastAdminPage(slug);
        const s = last && isAdminPage(last) ? last : "dashboard";
        navigate(`/${slug}/${s}`);
        return;
      }
      return;
    }

    if (mode === "client-home") {
      // Admin takes priority at root, else client home, else client login.
      if (adminAuthed && (!storedAdminSlug || storedAdminSlug === slug)) {
        const last = getLastAdminPage(slug);
        const s = last && isAdminPage(last) ? last : "dashboard";
        navigate(`/${slug}/${s}`);
        return;
      }
      if (clientTenantSlug === slug) return;
      navigate(`/${slug}/client/login`);
      return;
    }

    // admin-app
    if (!adminAuthed || (storedAdminSlug && storedAdminSlug !== slug)) {
      if (clientTenantSlug === slug) {
        navigate(`/${slug}`);
        return;
      }
      navigate(`/${slug}/login`);
      return;
    }
    if (section === "branding") {
      navigate(`/${slug}/general`);
      return;
    }
    if (section === "ipam") {
      navigate(`/${slug}/ip-pool`);
      return;
    }
    if (!isAdminPage(section)) {
      const last = getLastAdminPage(slug);
      const s = last && isAdminPage(last) ? last : "dashboard";
      navigate(`/${slug}/${s}`);
      return;
    }
    if (section === "customers" && rest.length) {
      if (!parseCustomerNestedRest(section, rest)) {
        navigate(`/${slug}/customers`);
        return;
      }
    } else if (rest.length) {
      navigate(`/${slug}/${section}`);
      return;
    }
    setLastAdminPage(slug, section);
  }, [
    p,
    adminAuthed,
    platformAuthed,
    clientTenantSlug,
    navigate,
    storedAdminSlug,
    tenantRoute,
    clientSession,
  ]);

  if (p === "/" || p === "") {
    return <Landing onGoLogin={() => navigate("/login")} />;
  }

  if (p === "/login") {
    if (platformAuthed) return <RouteLoading />;
    return (
      <PlatformLogin
        onSuccess={() => {
          refreshAuth();
          navigate("/platform");
        }}
      />
    );
  }

  if (p === "/platform") {
    if (!platformAuthed) return <RouteLoading />;
    return (
      <PlatformApp
        onLogout={() => {
          refreshAuth();
          navigate("/login");
        }}
      />
    );
  }

  if (isolirSlug) {
    return <RouteLoading />;
  }

  if (tenantRoute) {
    const { slug, mode, section, rest } = tenantRoute;
    const accent = <TenantAccent slug={slug} />;

    if (mode === "admin-login") {
      if (adminAuthed && storedAdminSlug === slug) return <RouteLoading />;
      if (clientTenantSlug === slug) return <RouteLoading />;
      return (
        <>
          {accent}
          <TenantLogin
            slug={slug}
            mode="admin"
            onAdminSuccess={() => {
              refreshAuth();
              navigate(`/${slug}/dashboard`);
            }}
            onClientSuccess={() => {
              refreshAuth();
              navigate(`/${slug}/client`);
            }}
          />
        </>
      );
    }

    if (mode === "client-login") {
      if (clientTenantSlug === slug) return <RouteLoading />;
      if (adminAuthed && storedAdminSlug === slug) return <RouteLoading />;
      return (
        <>
          {accent}
          <TenantLogin
            slug={slug}
            mode="client"
            onAdminSuccess={() => {
              refreshAuth();
              navigate(`/${slug}/dashboard`);
            }}
            onClientSuccess={() => {
              refreshAuth();
              navigate(`/${slug}/client`);
            }}
          />
        </>
      );
    }

    if (mode === "client-home") {
      const adminForThis = adminAuthed && (!storedAdminSlug || storedAdminSlug === slug);
      if (!adminForThis && clientTenantSlug === slug && clientSession) {
        return (
          <>
            {accent}
            <ClientHome
              data={clientSession}
              onLogout={() => {
                clearClientSession();
                refreshAuth();
                navigate(`/${slug}/client/login`);
              }}
            />
          </>
        );
      }
      return <RouteLoading />;
    }

    // admin-app
    if (!adminAuthed || (storedAdminSlug && storedAdminSlug !== slug)) return <RouteLoading />;
    if (section === "branding") return <RouteLoading />;
    if (section === "ipam") return <RouteLoading />;
    if (!isAdminPage(section)) return <RouteLoading />;
    const page = normalizeAdminPage(section) as AdminPage;
    const nested = parseCustomerNestedRest(section, rest || []);
    return (
      <>
        {accent}
        <AdminApp
          tenantSlug={slug}
          page={page}
          customerSecretsId={nested?.kind === "secrets" ? nested.customerId : null}
          customerSecretsCreate={Boolean(nested?.kind === "secrets" && nested.create)}
          customerGalleryId={nested?.kind === "gallery" ? nested.customerId : null}
          onNavigate={(next, r) => {
            setLastAdminPage(slug, next);
            const suffix = r?.length ? `/${r.join("/")}` : "";
            navigate(`/${slug}/${next}${suffix}`);
          }}
          onLogout={() => {
            refreshAuth();
            navigate(`/${slug}/login`);
          }}
        />
      </>
    );
  }

  return <Landing onGoLogin={() => navigate("/login")} />;
}
