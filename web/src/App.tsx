import { useEffect, useState } from "react";
import { AdminApp, isAdminPage, type AdminPage } from "./AdminApp";
import { AdminLogin } from "./AdminLogin";
import { AUTH_EXPIRED_EVENT, clearClientSession, getAdminSlug, getClientSession, getPlatformToken, getToken } from "./api";
import { ClientHome } from "./ClientHome";
import { ClientLogin, type ClientPortalData } from "./ClientLogin";
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
  };
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
          const last = getLastAdminPage(adminRoute.slug);
          const section = last && isAdminPage(last) ? last : "dashboard";
          navigate(`/admin/${adminRoute.slug}/${section}`);
        }}
      />
    );
  }

  if (adminRoute && !adminRoute.login) {
    if (!adminAuthed || (storedAdminSlug && storedAdminSlug !== adminRoute.slug)) return null;
    if (!adminRoute.section || !isAdminPage(adminRoute.section)) return null;
    const page = adminRoute.section as AdminPage;
    return (
      <AdminApp
        tenantSlug={adminRoute.slug}
        page={page}
        onNavigate={(next) => {
          setLastAdminPage(adminRoute.slug, next);
          navigate(`/admin/${adminRoute.slug}/${next}`);
        }}
        onLogout={() => {
          refreshAuth();
          navigate(`/admin/${adminRoute.slug}/login`);
        }}
      />
    );
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
