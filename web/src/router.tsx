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
  getClientSession,
  getToken,
} from "./api";
import { AdminApp, isAdminPage, normalizeAdminPage, type AdminPage } from "./AdminApp";
import { ClientHome } from "./ClientHome";
import { Landing } from "./Landing";
import { TenantLogin, type ClientPortalData } from "./TenantLogin";
import { IsolirPortalPage } from "./IsolirPortalPage";
import { getLastAdminPage, setLastAdminPage } from "./navPersist";
import { TenantAccent } from "./theme";

/** Admin is authenticated when an access token is present. */
function isAdmin(): boolean {
  return Boolean(getToken());
}

function hasClientSession(): boolean {
  return Boolean(getClientSession<ClientPortalData>());
}

function lastAdminTarget(): AdminPage {
  const last = getLastAdminPage("app");
  const s = last && isAdminPage(last) ? normalizeAdminPage(last) : "dashboard";
  return s as AdminPage;
}

function adminNav(navigate: NavigateFn, next: AdminPage, rest?: string[]) {
  if (next === "customers" && rest && rest.length) {
    const [id, kind, extra] = rest;
    if (kind === "secrets") {
      if (extra === "new") {
        void navigate({ to: "/admin/customers/$customerId/secrets/new", params: { customerId: id } });
      } else {
        void navigate({ to: "/admin/customers/$customerId/secrets", params: { customerId: id } });
      }
      return;
    }
    if (kind === "gallery" || kind === "galery") {
      void navigate({ to: "/admin/customers/$customerId/gallery", params: { customerId: id } });
      return;
    }
  }
  void navigate({ to: "/admin/$section", params: { section: next } });
}

function RouteLoading() {
  return (
    <div className="flex min-h-full items-center justify-center p-8">
      <p className="text-sm text-[var(--muted)]">Memuat…</p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Root
// ---------------------------------------------------------------------------

function RootLayout() {
  const navigate = useNavigate();

  useEffect(() => {
    const onExpired = () => {
      void navigate({ to: "/admin/login" });
    };
    window.addEventListener(AUTH_EXPIRED_EVENT, onExpired);
    return () => window.removeEventListener(AUTH_EXPIRED_EVENT, onExpired);
  }, [navigate]);

  return (
    <>
      <TenantAccent />
      <Outlet />
    </>
  );
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
// Public
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

function ClientLoginView() {
  const navigate = useNavigate();
  return (
    <TenantLogin
      mode="client"
      onAdminSuccess={() => void navigate({ to: "/admin/dashboard" })}
      onClientSuccess={() => void navigate({ to: "/client/dashboard" })}
    />
  );
}

const clientLoginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  beforeLoad: () => {
    if (isAdmin()) throw redirect({ to: "/admin/dashboard" });
    if (hasClientSession()) throw redirect({ to: "/client/dashboard" });
  },
  component: ClientLoginView,
});

function AdminLoginView() {
  const navigate = useNavigate();
  return (
    <TenantLogin
      mode="admin"
      onAdminSuccess={() => void navigate({ to: "/admin/dashboard" })}
      onClientSuccess={() => void navigate({ to: "/client/dashboard" })}
    />
  );
}

const adminLoginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/login",
  beforeLoad: () => {
    if (isAdmin()) throw redirect({ to: "/admin/dashboard" });
    if (hasClientSession()) throw redirect({ to: "/client/dashboard" });
  },
  component: AdminLoginView,
});

const isolirRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/isolir",
  component: IsolirPortalPage,
});

// ---------------------------------------------------------------------------
// Admin app
// ---------------------------------------------------------------------------

function adminGuard() {
  if (!isAdmin()) {
    if (hasClientSession()) throw redirect({ to: "/client/dashboard" });
    throw redirect({ to: "/admin/login" });
  }
}

function AdminSectionView() {
  const params = useParams({ strict: false }) as { section: string };
  const navigate = useNavigate();
  const page = normalizeAdminPage(params.section) as AdminPage;
  return (
    <AdminApp
      page={page}
      onNavigate={(next, rest) => adminNav(navigate, next, rest)}
      onLogout={() => void navigate({ to: "/admin/login" })}
    />
  );
}

const adminIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin",
  beforeLoad: () => {
    adminGuard();
    throw redirect({ to: "/admin/$section", params: { section: lastAdminTarget() } });
  },
});

const adminSectionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/$section",
  beforeLoad: ({ params }) => {
    adminGuard();
    const section = params.section;
    if (section === "login") throw redirect({ to: "/admin/login" });
    if (section === "branding") throw redirect({ to: "/admin/$section", params: { section: "general" } });
    if (section === "ipam") throw redirect({ to: "/admin/$section", params: { section: "ip-pool" } });
    if (section === "tech") throw redirect({ to: "/admin/$section", params: { section: "tickets" } });
    if (section === "subscriptions") throw redirect({ to: "/admin/$section", params: { section: "customers" } });
    if (!isAdminPage(section)) {
      throw redirect({ to: "/admin/$section", params: { section: lastAdminTarget() } });
    }
    setLastAdminPage("app", normalizeAdminPage(section));
  },
  component: AdminSectionView,
});

// ---- customer nested ------------------------------------------------------

function customerBeforeLoad() {
  adminGuard();
  setLastAdminPage("app", "customers");
}

function CustomerSecretsView() {
  const params = useParams({ strict: false }) as { customerId: string };
  const navigate = useNavigate();
  return (
    <AdminApp
      page="customers"
      customerSecretsId={params.customerId}
      onNavigate={(next, rest) => adminNav(navigate, next, rest)}
      onLogout={() => void navigate({ to: "/admin/login" })}
    />
  );
}

function CustomerSecretsNewView() {
  const params = useParams({ strict: false }) as { customerId: string };
  const navigate = useNavigate();
  return (
    <AdminApp
      page="customers"
      customerSecretsId={params.customerId}
      customerSecretsCreate
      onNavigate={(next, rest) => adminNav(navigate, next, rest)}
      onLogout={() => void navigate({ to: "/admin/login" })}
    />
  );
}

function CustomerGalleryView() {
  const params = useParams({ strict: false }) as { customerId: string };
  const navigate = useNavigate();
  return (
    <AdminApp
      page="customers"
      customerGalleryId={params.customerId}
      onNavigate={(next, rest) => adminNav(navigate, next, rest)}
      onLogout={() => void navigate({ to: "/admin/login" })}
    />
  );
}

const customerSecretsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/customers/$customerId/secrets",
  beforeLoad: customerBeforeLoad,
  component: CustomerSecretsView,
});

const customerSecretsNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/customers/$customerId/secrets/new",
  beforeLoad: customerBeforeLoad,
  component: CustomerSecretsNewView,
});

const customerGalleryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/customers/$customerId/gallery",
  beforeLoad: customerBeforeLoad,
  component: CustomerGalleryView,
});

const customerGaleryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/customers/$customerId/galery",
  beforeLoad: ({ params }) => {
    customerBeforeLoad();
    throw redirect({ to: "/admin/customers/$customerId/gallery", params: { customerId: params.customerId } });
  },
});

// ---------------------------------------------------------------------------
// Client portal
// ---------------------------------------------------------------------------

function ClientHomeView() {
  const navigate = useNavigate();
  const data = getClientSession<ClientPortalData>();
  if (!data) return <RouteLoading />;
  return (
    <ClientHome
      data={data}
      onLogout={() => {
        clearClientSession();
        void navigate({ to: "/login" });
      }}
    />
  );
}

function clientGuard() {
  if (isAdmin()) throw redirect({ to: "/admin/dashboard" });
  if (!hasClientSession()) throw redirect({ to: "/login" });
}

const clientIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/client",
  beforeLoad: () => {
    clientGuard();
    throw redirect({ to: "/client/dashboard" });
  },
});

const clientSplatRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/client/$",
  beforeLoad: clientGuard,
  component: ClientHomeView,
});

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

const routeTree = rootRoute.addChildren([
  indexRoute,
  clientLoginRoute,
  adminLoginRoute,
  isolirRoute,
  adminIndexRoute,
  adminSectionRoute,
  customerSecretsRoute,
  customerSecretsNewRoute,
  customerGalleryRoute,
  customerGaleryRoute,
  clientIndexRoute,
  clientSplatRoute,
]);

export const router = createRouter({ routeTree });

export function AppRouter() {
  return <RouterProvider router={router} />;
}
