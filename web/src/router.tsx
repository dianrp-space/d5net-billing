import { useEffect } from "react";
import {
  Outlet,
  RouterProvider,
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
  useNavigate,
  useParams,
  type NavigateFn,
} from "@tanstack/react-router";
import {
  AUTH_EXPIRED_EVENT,
  clearClientSession,
  getAdminSlug,
  getClientSession,
  getPlatformToken,
  getToken,
} from "./api";
import { AdminApp, isAdminPage, normalizeAdminPage, type AdminPage } from "./AdminApp";
import { ClientHome } from "./ClientHome";
import { Landing } from "./Landing";
import { PlatformApp } from "./PlatformApp";
import { PlatformLogin } from "./PlatformLogin";
import { TenantLogin, type ClientPortalData } from "./TenantLogin";
import { getLastAdminPage, setLastAdminPage } from "./navPersist";
import { TenantAccent } from "./theme";

/** Top-level segments that are never tenant slugs (platform / isolir / legacy). */
const RESERVED_TOP = new Set(["login", "platform", "isolir", "admin", "client"]);
const SLUG_RE = /^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$/;

function clientSlug(): string {
  return getClientSession<ClientPortalData>()?.tenant_slug || "";
}

/** Admin logged in and either no stored slug or it matches this tenant. */
function isAdminFor(slug: string): boolean {
  const authed = Boolean(getToken());
  const stored = getAdminSlug();
  return authed && (!stored || stored === slug);
}

/** Admin logged in specifically for this tenant. */
function isAdminExact(slug: string): boolean {
  return Boolean(getToken()) && getAdminSlug() === slug;
}

function lastAdminTarget(slug: string): AdminPage {
  const last = getLastAdminPage(slug);
  const s = last && isAdminPage(last) ? normalizeAdminPage(last) : "dashboard";
  return s as AdminPage;
}

function adminRedirect(slug: string) {
  const s = lastAdminTarget(slug);
  return redirect({ to: "/$slug/$section", params: { slug, section: s } });
}

function adminNav(navigate: NavigateFn, slug: string, next: AdminPage, rest?: string[]) {
  if (next === "customers" && rest && rest.length) {
    const [id, kind, extra] = rest;
    if (kind === "secrets") {
      if (extra === "new") {
        void navigate({ to: "/$slug/customers/$customerId/secrets/new", params: { slug, customerId: id } });
      } else {
        void navigate({ to: "/$slug/customers/$customerId/secrets", params: { slug, customerId: id } });
      }
      return;
    }
    if (kind === "gallery" || kind === "galery") {
      void navigate({ to: "/$slug/customers/$customerId/gallery", params: { slug, customerId: id } });
      return;
    }
  }
  void navigate({ to: "/$slug/$section", params: { slug, section: next } });
}

function RouteLoading() {
  return (
    <div className="flex min-h-full items-center justify-center p-8">
      <p className="text-sm text-[var(--muted)]">Memuat…</p>
    </div>
  );
}

function useSlug(): string {
  const params = useParams({ strict: false }) as { slug?: string };
  return params.slug || "";
}

// ---------------------------------------------------------------------------
// Root
// ---------------------------------------------------------------------------

function RootLayout() {
  const navigate = useNavigate();

  useEffect(() => {
    const onExpired = (ev: Event) => {
      const platform = Boolean((ev as CustomEvent<{ platform?: boolean }>).detail?.platform);
      if (platform) {
        void navigate({ to: "/login" });
        return;
      }
      const slug = getAdminSlug();
      if (slug) void navigate({ to: "/$slug/login", params: { slug } });
      else void navigate({ to: "/" });
    };
    window.addEventListener(AUTH_EXPIRED_EVENT, onExpired);
    return () => window.removeEventListener(AUTH_EXPIRED_EVENT, onExpired);
  }, [navigate]);

  return <Outlet />;
}

function NotFoundView() {
  const navigate = useNavigate();
  useEffect(() => {
    void navigate({ to: "/" });
  }, [navigate]);
  return <RouteLoading />;
}

const rootRoute = createRootRoute({ component: RootLayout, notFoundComponent: NotFoundView });

// ---------------------------------------------------------------------------
// Platform / public
// ---------------------------------------------------------------------------

function LandingView() {
  const navigate = useNavigate();
  return <Landing onGoLogin={() => void navigate({ to: "/login" })} />;
}

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: LandingView,
});

function PlatformLoginView() {
  const navigate = useNavigate();
  return <PlatformLogin onSuccess={() => void navigate({ to: "/platform" })} />;
}

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  beforeLoad: () => {
    if (getPlatformToken()) throw redirect({ to: "/platform" });
  },
  component: PlatformLoginView,
});

function PlatformAppView() {
  const navigate = useNavigate();
  return <PlatformApp onLogout={() => void navigate({ to: "/login" })} />;
}

const platformRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/platform",
  beforeLoad: () => {
    if (!getPlatformToken()) throw redirect({ to: "/login" });
  },
  component: PlatformAppView,
});

// ---------------------------------------------------------------------------
// Legacy redirects: /admin/... and /client/...
// ---------------------------------------------------------------------------

const legacyAdminRootRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin",
  beforeLoad: () => {
    throw redirect({ to: "/" });
  },
});

const legacyClientRootRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/client",
  beforeLoad: () => {
    throw redirect({ to: "/" });
  },
});

const legacyAdminSplatRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/$",
  beforeLoad: ({ params }) => {
    const segs = (params._splat || "").split("/").filter(Boolean);
    const first = segs[0];
    if (first === "login") {
      const slug = getAdminSlug();
      if (slug) throw redirect({ to: "/$slug/login", params: { slug } });
      throw redirect({ to: "/" });
    }
    if (!first) throw redirect({ to: "/" });
    const rest = segs.slice(1);
    if (rest[0] === "customers" && rest[1]) {
      if (rest[2] === "secrets") {
        if (rest[3] === "new") {
          throw redirect({ to: "/$slug/customers/$customerId/secrets/new", params: { slug: first, customerId: rest[1] } });
        }
        throw redirect({ to: "/$slug/customers/$customerId/secrets", params: { slug: first, customerId: rest[1] } });
      }
      if (rest[2] === "gallery" || rest[2] === "galery") {
        throw redirect({ to: "/$slug/customers/$customerId/gallery", params: { slug: first, customerId: rest[1] } });
      }
    }
    if (!rest.length) throw redirect({ to: "/$slug", params: { slug: first } });
    throw redirect({ to: "/$slug/$section", params: { slug: first, section: rest[0] } });
  },
});

const legacyClientSplatRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/client/$",
  beforeLoad: ({ params }) => {
    const segs = (params._splat || "").split("/").filter(Boolean);
    const first = segs[0];
    if (first === "login") {
      const slug = clientSlug();
      if (slug) throw redirect({ to: "/$slug/client/login", params: { slug } });
      throw redirect({ to: "/" });
    }
    if (!first) throw redirect({ to: "/" });
    if (segs[1] === "login") throw redirect({ to: "/$slug/client/login", params: { slug: first } });
    throw redirect({ to: "/$slug", params: { slug: first } });
  },
});

const isolirRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/isolir/$slug",
  beforeLoad: ({ params }) => {
    throw redirect({ to: "/$slug/client", params: { slug: params.slug } });
  },
});

// ---------------------------------------------------------------------------
// Tenant layout
// ---------------------------------------------------------------------------

function TenantLayout() {
  const slug = useSlug();
  return (
    <>
      <TenantAccent slug={slug} />
      <Outlet />
    </>
  );
}

const tenantRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/$slug",
  beforeLoad: ({ params }) => {
    if (RESERVED_TOP.has(params.slug) || !SLUG_RE.test(params.slug)) {
      throw redirect({ to: "/" });
    }
  },
  component: TenantLayout,
});

// ---- client home ( /{slug} and /{slug}/client ) --------------------------

function ClientHomeView() {
  const slug = useSlug();
  const navigate = useNavigate();
  const data = getClientSession<ClientPortalData>();
  if (!data) return <RouteLoading />;
  return (
    <ClientHome
      data={data}
      onLogout={() => {
        clearClientSession();
        void navigate({ to: "/$slug/client/login", params: { slug } });
      }}
    />
  );
}

function clientHomeBeforeLoad({ params }: { params: { slug: string } }) {
  if (isAdminFor(params.slug)) throw adminRedirect(params.slug);
  if (clientSlug() === params.slug) return;
  throw redirect({ to: "/$slug/client/login", params: { slug: params.slug } });
}

const tenantIndexRoute = createRoute({
  getParentRoute: () => tenantRoute,
  path: "/",
  beforeLoad: clientHomeBeforeLoad,
  component: ClientHomeView,
});

const clientRoute = createRoute({
  getParentRoute: () => tenantRoute,
  path: "client",
  beforeLoad: clientHomeBeforeLoad,
  component: ClientHomeView,
});

// ---- logins ---------------------------------------------------------------

function AdminLoginView() {
  const slug = useSlug();
  const navigate = useNavigate();
  return (
    <TenantLogin
      slug={slug}
      mode="admin"
      onAdminSuccess={() => void navigate({ to: "/$slug/$section", params: { slug, section: "dashboard" } })}
      onClientSuccess={() => void navigate({ to: "/$slug/client", params: { slug } })}
    />
  );
}

const adminLoginRoute = createRoute({
  getParentRoute: () => tenantRoute,
  path: "login",
  beforeLoad: ({ params }) => {
    if (isAdminExact(params.slug)) throw adminRedirect(params.slug);
    if (clientSlug() === params.slug) throw redirect({ to: "/$slug/client", params: { slug: params.slug } });
  },
  component: AdminLoginView,
});

function ClientLoginView() {
  const slug = useSlug();
  const navigate = useNavigate();
  return (
    <TenantLogin
      slug={slug}
      mode="client"
      onAdminSuccess={() => void navigate({ to: "/$slug/$section", params: { slug, section: "dashboard" } })}
      onClientSuccess={() => void navigate({ to: "/$slug/client", params: { slug } })}
    />
  );
}

const clientLoginRoute = createRoute({
  getParentRoute: () => tenantRoute,
  path: "client/login",
  beforeLoad: ({ params }) => {
    if (clientSlug() === params.slug) throw redirect({ to: "/$slug/client", params: { slug: params.slug } });
    if (isAdminExact(params.slug)) throw adminRedirect(params.slug);
  },
  component: ClientLoginView,
});

// ---- admin app ------------------------------------------------------------

function adminGuard({ params }: { params: { slug: string } }) {
  if (!isAdminFor(params.slug)) {
    if (clientSlug() === params.slug) throw redirect({ to: "/$slug", params: { slug: params.slug } });
    throw redirect({ to: "/$slug/login", params: { slug: params.slug } });
  }
}

function AdminSectionView() {
  const params = useParams({ strict: false }) as { slug: string; section: string };
  const navigate = useNavigate();
  const page = normalizeAdminPage(params.section) as AdminPage;
  return (
    <AdminApp
      tenantSlug={params.slug}
      page={page}
      onNavigate={(next, rest) => adminNav(navigate, params.slug, next, rest)}
      onLogout={() => void navigate({ to: "/$slug/login", params: { slug: params.slug } })}
    />
  );
}

const adminSectionRoute = createRoute({
  getParentRoute: () => tenantRoute,
  path: "$section",
  beforeLoad: ({ params }) => {
    adminGuard({ params });
    const { slug, section } = params;
    if (section === "branding") {
      throw redirect({ to: "/$slug/$section", params: { slug, section: "general" } });
    }
    if (section === "ipam") {
      throw redirect({ to: "/$slug/$section", params: { slug, section: "ip-pool" } });
    }
    if (section === "tech") {
      throw redirect({ to: "/$slug/$section", params: { slug, section: "tickets" } });
    }
    if (section === "subscriptions") {
      throw redirect({ to: "/$slug/$section", params: { slug, section: "customers" } });
    }
    if (!isAdminPage(section)) throw adminRedirect(slug);
    setLastAdminPage(slug, normalizeAdminPage(section));
  },
  component: AdminSectionView,
});

// ---- customer nested ------------------------------------------------------

function customerBeforeLoad({ params }: { params: { slug: string } }) {
  adminGuard({ params });
  setLastAdminPage(params.slug, "customers");
}

function CustomerSecretsView() {
  const params = useParams({ strict: false }) as { slug: string; customerId: string };
  const navigate = useNavigate();
  return (
    <AdminApp
      tenantSlug={params.slug}
      page="customers"
      customerSecretsId={params.customerId}
      onNavigate={(next, rest) => adminNav(navigate, params.slug, next, rest)}
      onLogout={() => void navigate({ to: "/$slug/login", params: { slug: params.slug } })}
    />
  );
}

function CustomerSecretsNewView() {
  const params = useParams({ strict: false }) as { slug: string; customerId: string };
  const navigate = useNavigate();
  return (
    <AdminApp
      tenantSlug={params.slug}
      page="customers"
      customerSecretsId={params.customerId}
      customerSecretsCreate
      onNavigate={(next, rest) => adminNav(navigate, params.slug, next, rest)}
      onLogout={() => void navigate({ to: "/$slug/login", params: { slug: params.slug } })}
    />
  );
}

function CustomerGalleryView() {
  const params = useParams({ strict: false }) as { slug: string; customerId: string };
  const navigate = useNavigate();
  return (
    <AdminApp
      tenantSlug={params.slug}
      page="customers"
      customerGalleryId={params.customerId}
      onNavigate={(next, rest) => adminNav(navigate, params.slug, next, rest)}
      onLogout={() => void navigate({ to: "/$slug/login", params: { slug: params.slug } })}
    />
  );
}

const customerSecretsRoute = createRoute({
  getParentRoute: () => tenantRoute,
  path: "customers/$customerId/secrets",
  beforeLoad: customerBeforeLoad,
  component: CustomerSecretsView,
});

const customerSecretsNewRoute = createRoute({
  getParentRoute: () => tenantRoute,
  path: "customers/$customerId/secrets/new",
  beforeLoad: customerBeforeLoad,
  component: CustomerSecretsNewView,
});

const customerGalleryRoute = createRoute({
  getParentRoute: () => tenantRoute,
  path: "customers/$customerId/gallery",
  beforeLoad: customerBeforeLoad,
  component: CustomerGalleryView,
});

const customerGaleryRoute = createRoute({
  getParentRoute: () => tenantRoute,
  path: "customers/$customerId/galery",
  beforeLoad: ({ params }) => {
    customerBeforeLoad({ params });
    throw redirect({ to: "/$slug/customers/$customerId/gallery", params: { slug: params.slug, customerId: params.customerId } });
  },
});

// ---- tenant catch-all (unknown deep paths) --------------------------------

const tenantSplatRoute = createRoute({
  getParentRoute: () => tenantRoute,
  path: "$",
  beforeLoad: ({ params }) => {
    const segs = (params._splat || "").split("/").filter(Boolean);
    if (segs[0]) {
      throw redirect({ to: "/$slug/$section", params: { slug: params.slug, section: segs[0] } });
    }
    throw redirect({ to: "/$slug", params: { slug: params.slug } });
  },
});

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  platformRoute,
  legacyAdminRootRoute,
  legacyClientRootRoute,
  legacyAdminSplatRoute,
  legacyClientSplatRoute,
  isolirRoute,
  tenantRoute.addChildren([
    tenantIndexRoute,
    clientRoute,
    adminLoginRoute,
    clientLoginRoute,
    customerSecretsRoute,
    customerSecretsNewRoute,
    customerGalleryRoute,
    customerGaleryRoute,
    adminSectionRoute,
    tenantSplatRoute,
  ]),
]);

export const router = createRouter({ routeTree });

export function AppRouter() {
  return <RouterProvider router={router} />;
}
