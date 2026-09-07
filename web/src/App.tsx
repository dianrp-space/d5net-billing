import { useEffect, useState } from "react";
import { AdminApp, isAdminPage, type AdminPage } from "./AdminApp";
import { AdminLogin } from "./AdminLogin";
import { AUTH_EXPIRED_EVENT, clearClientSession, getAdminSlug, getClientSession, getPlatformToken, getToken } from "./api";
import { ClientHome } from "./ClientHome";
import { ClientLogin, type ClientPortalData } from "./ClientLogin";
import { IsolirPortalPage } from "./IsolirPortalPage";
import { Landing } from "./Landing";
import { PlatformApp } from "./PlatformApp";
import { PlatformLogin } from "./PlatformLogin";
import { getLastAdminPage, setLastAdminPage } from "./navPersist";
import { usePath } from "./usePath";

function normalize(path: string) {
  if (path.length > 1 && path.endsWith("/")) return path.slice(0, -1);
  return path;
}

function parseTenantRoute(path: string, kind: "admin" | "client") {
  const prefix = `/${kind}/`;
  if (!path.startsWith(prefix)) return null;
  const parts = path.slice(prefix.length).split("/").filter(Boolean);
  if (!parts.length || parts[0] === "login") return null;
  return {
    slug: parts[0],
    login: parts[1] === "login",
    section: parts[1] && parts[1] !== "login" ? parts[1] : "",
    rest: parts.slice(2),
  };
}

/** /admin/{slug}/customers/{id}/secrets[/new] | /customers/{id}/gallery|galery */
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

export default function App() {
  const { path, navigate } = usePath();
  const [, setAuthTick] = useState(0);
  const refreshAuth = () => setAuthTick((n) => n + 1);
  const p = normalize(path);
  const adminAuthed = Boolean(getToken());
  const platformAuthed = Boolean(getPlatformToken());
  const clientSession = getClientSession<ClientPortalData>();
  const storedAdminSlug = getAdminSlug();

  const adminRoute = parseTenantRoute(p, "admin");
  const clientRoute = parseTenantRoute(p, "client");
  const isolirMatch = p.match(/^\/isolir\/([^/]+)\/?$/);
  const isolirSlug = isolirMatch?.[1] || "";
  const adminKey = adminRoute ? `${adminRoute.slug}:${adminRoute.login ? "login" : "app"}` : "";
  const clientKey = clientRoute ? `${clientRoute.slug}:${clientRoute.login ? "login" : "app"}` : "";
  const clientTenantSlug = clientSession?.tenant_slug || "";

  useEffect(() => {
    const onExpired = (ev: Event) => {
      const platform = Boolean((ev as CustomEvent<{ platform?: boolean }>).detail?.platform);
      setAuthTick((n) => n + 1);
      if (platform) {
        navigate("/login");
        return;
      }
      const slug = getAdminSlug() || adminRoute?.slug;
      navigate(slug ? `/admin/${slug}/login` : "/");
    };
    window.addEventListener(AUTH_EXPIRED_EVENT, onExpired);
    return () => window.removeEventListener(AUTH_EXPIRED_EVENT, onExpired);
  }, [navigate, adminRoute?.slug]);

  useEffect(() => {
    if (p === "/admin/login") {
      navigate(storedAdminSlug ? `/admin/${storedAdminSlug}/login` : "/");
      return;
    }
    if (p === "/client/login") {
      navigate(clientTenantSlug ? `/client/${clientTenantSlug}/login` : "/");
      return;
    }
    if (p === "/admin" || p === "/client") {
      navigate("/");
      return;
    }

    if (p === "/login" && platformAuthed) {
      navigate("/platform");
      return;
    }
    if (p === "/platform" && !platformAuthed) {
      navigate("/login");
      return;
    }

    if (adminRoute) {
      if (adminRoute.login) {
        if (adminAuthed && storedAdminSlug === adminRoute.slug) {
          const last = getLastAdminPage(adminRoute.slug);
          const section = last && isAdminPage(last) ? last : "dashboard";
          navigate(`/admin/${adminRoute.slug}/${section}`);
        }
        return;
      }
      if (!adminAuthed || (storedAdminSlug && storedAdminSlug !== adminRoute.slug)) {
        navigate(`/admin/${adminRoute.slug}/login`);
        return;
      }
      if (!adminRoute.section) {
        const last = getLastAdminPage(adminRoute.slug);
        const section = last && isAdminPage(last) ? last : "dashboard";
        navigate(`/admin/${adminRoute.slug}/${section}`);
        return;
      }
      if (!isAdminPage(adminRoute.section)) {
        const last = getLastAdminPage(adminRoute.slug);
        const section = last && isAdminPage(last) ? last : "dashboard";
        navigate(`/admin/${adminRoute.slug}/${section}`);
        return;
      }
      // Allow nested customer secrets/gallery; ignore unknown nested paths.
      if (adminRoute.section === "customers" && adminRoute.rest.length) {
        if (!parseCustomerNestedRest(adminRoute.section, adminRoute.rest)) {
          navigate(`/admin/${adminRoute.slug}/customers`);
          return;
        }
      } else if (adminRoute.rest.length) {
        navigate(`/admin/${adminRoute.slug}/${adminRoute.section}`);
        return;
      }
      setLastAdminPage(adminRoute.slug, adminRoute.section);
      return;
    }

    if (clientRoute) {
      if (clientRoute.login) {
        if (clientTenantSlug === clientRoute.slug) navigate(`/client/${clientRoute.slug}`);
        return;
      }
      if (!clientSession || clientTenantSlug !== clientRoute.slug) {
        navigate(`/client/${clientRoute.slug}/login`);
      }
    }
  }, [
    p,
    adminAuthed,
    platformAuthed,
    clientTenantSlug,
    navigate,
    storedAdminSlug,
    adminKey,
    clientKey,
    adminRoute,
    clientRoute,
    clientSession,
  ]);

  if (p === "/" || p === "") {
    return <Landing onGoLogin={() => navigate("/login")} />;
  }

  if (p === "/login") {
    if (platformAuthed) return null;
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
    if (!platformAuthed) return null;
    return (
      <PlatformApp
        onLogout={() => {
          refreshAuth();
          navigate("/login");
        }}
      />
    );
  }

  if (adminRoute?.login) {
    if (adminAuthed && storedAdminSlug === adminRoute.slug) return null;
    return (
      <AdminLogin
        slug={adminRoute.slug}
        onSuccess={() => {
          refreshAuth();
          // Start at dashboard after login; permission filter will keep allowed menus.
          // Avoid restoring a last page that triggers settings-only APIs for limited roles.
          navigate(`/admin/${adminRoute.slug}/dashboard`);
        }}
      />
    );
  }

  if (adminRoute && !adminRoute.login) {
    if (!adminAuthed || (storedAdminSlug && storedAdminSlug !== adminRoute.slug)) return null;
    if (!adminRoute.section || !isAdminPage(adminRoute.section)) return null;
    const page = adminRoute.section as AdminPage;
    const nested = parseCustomerNestedRest(adminRoute.section, adminRoute.rest || []);
    return (
      <AdminApp
        tenantSlug={adminRoute.slug}
        page={page}
        customerSecretsId={nested?.kind === "secrets" ? nested.customerId : null}
        customerSecretsCreate={Boolean(nested?.kind === "secrets" && nested.create)}
        customerGalleryId={nested?.kind === "gallery" ? nested.customerId : null}
        onNavigate={(next, rest) => {
          setLastAdminPage(adminRoute.slug, next);
          const suffix = rest?.length ? `/${rest.join("/")}` : "";
          navigate(`/admin/${adminRoute.slug}/${next}${suffix}`);
        }}
        onLogout={() => {
          refreshAuth();
          navigate(`/admin/${adminRoute.slug}/login`);
        }}
      />
    );
  }

  if (isolirSlug) {
    return <IsolirPortalPage slug={isolirSlug} />;
  }

  if (clientRoute?.login) {
    if (clientTenantSlug === clientRoute.slug) return null;
    return (
      <ClientLogin
        slug={clientRoute.slug}
        onSuccess={() => {
          refreshAuth();
          navigate(`/client/${clientRoute.slug}`);
        }}
      />
    );
  }

  if (clientRoute && !clientRoute.login) {
    if (!clientSession || clientTenantSlug !== clientRoute.slug) return null;
    return (
      <ClientHome
        data={clientSession}
        onLogout={() => {
          clearClientSession();
          refreshAuth();
          navigate(`/client/${clientRoute.slug}/login`);
        }}
      />
    );
  }

  return <Landing onGoLogin={() => navigate("/login")} />;
}
