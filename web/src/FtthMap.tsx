import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, getToken } from "./api";
import { formatDateTime } from "./tenantTime";
import { useAppDialog } from "./confirm";
import { toastError, toastSuccess } from "./swal";
import { FormDialog, IconButton, Table } from "./ui";
import { IconDownload, IconPencil, IconTrash, IconUpload } from "./icons";

/** Leaflet loaded via CDN in index.html */
declare global {
  interface Window {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    L: any;
  }
}

type LatLng = { lat: number; lng: number };
type MapODP = {
  id?: string;
  name: string;
  code: string;
  cluster_id?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  port_count: number;
  used_ports?: number;
  free_ports?: number;
  coverage_radius_km?: number | null;
};
type MapCustomer = {
  id: string;
  full_name: string;
  customer_code: string;
  cluster_id?: string | null;
  latitude?: number | null;
  longitude?: number | null;
};
type MapCluster = {
  id: string;
  name: string;
  code: string;
  latitude?: number | null;
  longitude?: number | null;
  coverage_radius_km?: number | null;
};
/** Tenant-custom map icon URLs (nil = use built-in SVG marker). */
type MapIcons = {
  pop?: string | null;
  odp?: string | null;
  customer?: string | null;
};
type CableRoute = {
  id: string;
  name: string;
  path: [number, number][] | string;
  color: string;
  cluster_id?: string | null;
};

function parsePath(path: CableRoute["path"]): [number, number][] {
  if (Array.isArray(path)) return path as [number, number][];
  try {
    return JSON.parse(path) as [number, number][];
  } catch {
    return [];
  }
}

/** Great-circle distance in meters (Haversine). Path is [[lng, lat], ...]. */
function pathLengthMeters(path: [number, number][]): number {
  if (path.length < 2) return 0;
  const R = 6371000;
  let total = 0;
  for (let i = 1; i < path.length; i++) {
    const [lng1, lat1] = path[i - 1];
    const [lng2, lat2] = path[i];
    const φ1 = (lat1 * Math.PI) / 180;
    const φ2 = (lat2 * Math.PI) / 180;
    const Δφ = ((lat2 - lat1) * Math.PI) / 180;
    const Δλ = ((lng2 - lng1) * Math.PI) / 180;
    const a = Math.sin(Δφ / 2) ** 2 + Math.cos(φ1) * Math.cos(φ2) * Math.sin(Δλ / 2) ** 2;
    total += 2 * R * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
  }
  return total;
}

function formatDistance(meters: number): string {
  if (!Number.isFinite(meters) || meters <= 0) return "—";
  if (meters < 1000) return `${Math.round(meters)} m`;
  return `${(meters / 1000).toFixed(meters < 10000 ? 2 : 1)} km`;
}

type MapMarkerKind = "pop" | "odp" | "customer";

export const MAP_MARKER = {
  pop: { color: "#d946ef", label: "POP" },
  odp: { color: "#2563eb", label: "ODP" },
  customer: { color: "#15803d", label: "Pelanggan" },
} as const;

/** Warna fix standar serat fiber (TIA-598) agar pilihan warna jalur konsisten. */
export const ROUTE_COLOR_PRESETS = [
  "#1971C2", // biru
  "#F08C00", // oranye
  "#2F9E44", // hijau
  "#8C7355", // cokelat (default)
  "#868E96", // abu
  "#F1F3F5", // putih
  "#E03131", // merah
  "#212529", // hitam
  "#FCC419", // kuning
  "#7048E8", // ungu
  "#E64980", // pink
  "#0C8599", // aqua
];

export const DEFAULT_ROUTE_COLOR = "#8C7355";

const ROUTE_COLOR_HISTORY_KEY = "d5net_route_colors";

export function normalizeHexColor(value: string): string | null {
  const m = value.trim().match(/^#?([0-9a-fA-F]{6})$/);
  return m ? `#${m[1].toLowerCase()}` : null;
}

function loadRecentRouteColors(): string[] {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(ROUTE_COLOR_HISTORY_KEY) || "[]");
    if (Array.isArray(raw)) {
      return raw
        .filter((c): c is string => typeof c === "string" && normalizeHexColor(c) !== null)
        .map((c) => (normalizeHexColor(c) as string))
        .slice(0, 8);
    }
  } catch {
    /* abaikan */
  }
  return [];
}

/** Distinct SVG glyphs so POP / ODP / pelanggan are readable at a glance. */
function mapMarkerSvg(kind: MapMarkerKind, color: string): string {
  if (kind === "pop") {
    // Tower / mast — POP / cluster
    return `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M12 3v18M8 7l4-4 4 4M7 21h10M9 12h6M10 16h4" stroke="${color}" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
      <circle cx="12" cy="10" r="2.2" fill="${color}"/>
    </svg>`;
  }
  if (kind === "odp") {
    // Distribution box — ODP
    return `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="4" y="5" width="16" height="14" rx="2" stroke="${color}" stroke-width="2.2" fill="${color}" fill-opacity="0.18"/>
      <path d="M8 9h8M8 12h8M8 15h5" stroke="${color}" stroke-width="2" stroke-linecap="round"/>
    </svg>`;
  }
  // Home / customer
  return `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true">
    <path d="M4 11.5 12 4l8 7.5" stroke="${color}" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
    <path d="M7 10.5V20h10v-9.5" stroke="${color}" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
    <path d="M10 20v-5h4v5" stroke="${color}" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
  </svg>`;
}

export function createMapMarkerIcon(L: Window["L"], kind: MapMarkerKind, iconUrl?: string | null) {
  const { color, label } = MAP_MARKER[kind];
  // Tenant custom image marker — rendered as-is (transparency preserved), anchored bottom-centre.
  if (iconUrl) {
    return L.divIcon({
      className: `ftth-map-marker ftth-map-marker--${kind} ftth-map-marker--img`,
      html: `<div class="ftth-map-marker-pin ftth-map-marker-pin--img" title="${label}">
        <img class="ftth-map-marker-img" src="${iconUrl}" alt="${label}" />
      </div>`,
      iconSize: [34, 34],
      iconAnchor: [17, 34],
      popupAnchor: [0, -34],
    });
  }
  return L.divIcon({
    className: `ftth-map-marker ftth-map-marker--${kind}`,
    html: `<div class="ftth-map-marker-pin" style="--ftth-marker:${color}" title="${label}">
      <span class="ftth-map-marker-glyph">${mapMarkerSvg(kind, "#fff")}</span>
      <span class="ftth-map-marker-tail"></span>
    </div>`,
    iconSize: [32, 40],
    iconAnchor: [16, 40],
    popupAnchor: [0, -36],
  });
}

export type Basemap = "street" | "satellite";
const BASEMAP_KEY = "drp_ftth_basemap";

export function addCoverageCircle(
  L: Window["L"],
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  group: any,
  lat: number,
  lng: number,
  km?: number | null,
  color = "#5A5A40",
  emphasized = false,
) {
  if (km == null || !Number.isFinite(km) || km <= 0) return null;
  const circle = L.circle([lat, lng], {
    radius: km * 1000,
    color: emphasized ? "#15803d" : color,
    weight: emphasized ? 3.5 : 2.5,
    fillColor: color,
    fillOpacity: emphasized ? 0.34 : 0.22,
    opacity: emphasized ? 1 : 0.9,
    interactive: false,
  });
  circle.addTo(group);
  circle.bringToBack();
  return circle;
}

export function readBasemap(): Basemap {
  try {
    const v = localStorage.getItem(BASEMAP_KEY);
    if (v === "satellite" || v === "street") return v;
  } catch {
    /* ignore */
  }
  return "street";
}

export function createBasemapLayer(L: Window["L"], kind: Basemap) {
  if (kind === "satellite") {
    return L.tileLayer("https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}", {
      attribution: "Tiles &copy; Esri",
      maxZoom: 19,
    });
  }
  return L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
    attribution: "&copy; OpenStreetMap",
    maxZoom: 19,
  });
}

export function MapODP({
  odps,
  clusterId,
  clusterCenter,
}: {
  odps: MapODP[];
  clusterId?: string | null;
  clusterCenter?: { lat: number; lng: number } | null;
}) {
  const qc = useQueryClient();
  const { confirm, alert } = useAppDialog();
  const mapRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const mapObj = useRef<any>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const layers = useRef<any>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const draftLayer = useRef<any>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const baseLayer = useRef<any>(null);

  const [basemap, setBasemap] = useState<Basemap>(() => readBasemap());
  const [drawMode, setDrawMode] = useState(false);
  const drawModeRef = useRef(false);
  drawModeRef.current = drawMode;
  const [editId, setEditId] = useState<string | null>(null);
  const editIdRef = useRef<string | null>(null);
  editIdRef.current = editId;
  const [draft, setDraft] = useState<[number, number][]>([]);
  const [routeName, setRouteName] = useState("");
  const [routeColor, setRouteColor] = useState(DEFAULT_ROUTE_COLOR);
  const [hexDraft, setHexDraft] = useState<string | null>(null);
  const [recentColors, setRecentColors] = useState<string[]>(loadRecentRouteColors);

  // Draft hex mengikuti perubahan warna dari picker/swatch/edit.
  useEffect(() => {
    setHexDraft(null);
  }, [routeColor]);

  function applyHex(value: string): boolean {
    const normalized = normalizeHexColor(value);
    if (!normalized) {
      setSaveErr("Format hex tidak valid, contoh #8C7355.");
      return false;
    }
    setSaveErr("");
    setRouteColor(normalized);
    return true;
  }

  function pickSwatch(color: string) {
    setSaveErr("");
    setRouteColor(color);
  }
  const [saveOpen, setSaveOpen] = useState(false);
  const [saveErr, setSaveErr] = useState("");
  const [odpOpen, setOdpOpen] = useState(false);
  const [odpForm, setOdpForm] = useState({ name: "", code: "", latitude: "", longitude: "", port_count: 8 });
  const [odpErr, setOdpErr] = useState("");
  const [placeOdpMode, setPlaceOdpMode] = useState(false);
  const placeOdpModeRef = useRef(false);
  placeOdpModeRef.current = placeOdpMode;
  const [backupOpen, setBackupOpen] = useState(false);
  const [backupMsg, setBackupMsg] = useState("");
  const [backupBusy, setBackupBusy] = useState(false);

  const addDraftPoint = (lat: number, lng: number) => {
    setDraft((prev) => [...prev, [lng, lat]]);
  };

  const cancelDraw = () => {
    setDrawMode(false);
    setEditId(null);
    setDraft([]);
    setRouteName("");
    setRouteColor(DEFAULT_ROUTE_COLOR);
    setSaveErr("");
  };

  const assetsQ = useQuery({
    queryKey: ["ftth-map"],
    queryFn: () =>
      api<{ odps: MapODP[]; customers: MapCustomer[]; clusters: MapCluster[]; icons: MapIcons }>("/api/ftth/map-assets"),
  });
  const routesQ = useQuery({
    queryKey: ["cable-routes", clusterId ?? "all"],
    queryFn: () => {
      if (clusterId && clusterId !== "__none__") {
        return api<CableRoute[]>(`/api/cable-routes?cluster_id=${encodeURIComponent(clusterId)}`);
      }
      return api<CableRoute[]>("/api/cable-routes");
    },
  });

  const saveRoute = useMutation({
    mutationFn: () => {
      const body: Record<string, unknown> = {
        name: routeName.trim() || `Jalur ${formatDateTime(new Date())}`,
        path: draft,
        color: routeColor || DEFAULT_ROUTE_COLOR,
      };
      if (clusterId && clusterId !== "__none__") body.cluster_id = clusterId;
      if (editId) {
        body.is_active = true;
        return api(`/api/cable-routes/${editId}`, { method: "PUT", body: JSON.stringify(body) });
      }
      return api("/api/cable-routes", { method: "POST", body: JSON.stringify(body) });
    },
    onSuccess: () => {
      setSaveOpen(false);
      setHexDraft(null);
      setRecentColors((prev) => {
        const c = normalizeHexColor(routeColor) ?? DEFAULT_ROUTE_COLOR;
        const next = [c, ...prev.filter((x) => x.toLowerCase() !== c.toLowerCase())].slice(0, 8);
        try {
          localStorage.setItem(ROUTE_COLOR_HISTORY_KEY, JSON.stringify(next));
        } catch {
          /* abaikan */
        }
        return next;
      });
      cancelDraw();
      qc.invalidateQueries({ queryKey: ["cable-routes"] });
      void toastSuccess(editId ? "Jalur diperbarui" : "Jalur ditambahkan");
    },
    onError: (e: Error) => {
      setSaveErr(e.message);
      void toastError(e.message);
    },
  });

  const removeRoute = useMutation({
    mutationFn: (id: string) => api(`/api/cable-routes/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      if (editId === removeRoute.variables) cancelDraw();
      qc.invalidateQueries({ queryKey: ["cable-routes"] });
      void toastSuccess("Jalur dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const createOdp = useMutation({
    mutationFn: () => {
      const body: Record<string, unknown> = {
        name: odpForm.name,
        code: odpForm.code,
        port_count: odpForm.port_count,
      };
      if (clusterId && clusterId !== "__none__") body.cluster_id = clusterId;
      if (odpForm.latitude.trim()) body.latitude = Number(odpForm.latitude);
      if (odpForm.longitude.trim()) body.longitude = Number(odpForm.longitude);
      return api("/api/odps", { method: "POST", body: JSON.stringify(body) });
    },
    onSuccess: () => {
      setOdpOpen(false);
      setPlaceOdpMode(false);
      setOdpErr("");
      setOdpForm({ name: "", code: "", latitude: "", longitude: "", port_count: 8 });
      qc.invalidateQueries({ queryKey: ["odps"] });
      qc.invalidateQueries({ queryKey: ["ftth-map"] });
      void toastSuccess("ODP ditambahkan");
    },
    onError: (e: Error) => {
      setOdpErr(e.message);
      void toastError(e.message);
    },
  });

  function openCreateOdpAt(lat: number, lng: number) {
    setOdpErr("");
    setPlaceOdpMode(false);
    setOdpForm({
      name: "",
      code: "",
      latitude: String(Number(lat.toFixed(6))),
      longitude: String(Number(lng.toFixed(6))),
      port_count: 8,
    });
    setOdpOpen(true);
  }

  function startPlaceOdp() {
    if (!clusterId || clusterId === "__none__") return;
    cancelDraw();
    setOdpErr("");
    setPlaceOdpMode(true);
  }

  function cancelPlaceOdp() {
    setPlaceOdpMode(false);
  }

  function openCreateOdp() {
    setPlaceOdpMode(false);
    setOdpErr("");
    setOdpForm({
      name: "",
      code: "",
      latitude: clusterCenter ? String(clusterCenter.lat) : "",
      longitude: clusterCenter ? String(clusterCenter.lng) : "",
      port_count: 8,
    });
    setOdpOpen(true);
  }

  async function startEdit(r: CableRoute) {
    const path = parsePath(r.path);
    if (path.length < 2) {
      await alert({ title: "Jalur tidak valid", description: "Jalur ini tidak punya titik yang valid." });
      return;
    }
    setPlaceOdpMode(false);
    setEditId(r.id);
    setRouteName(r.name);
    setRouteColor(r.color || DEFAULT_ROUTE_COLOR);
    setDraft(path);
    setDrawMode(true);
    setSaveErr("");
    const L = window.L;
    const map = mapObj.current;
    if (L && map) {
      const latlngs = path.map(([lng, lat]) => L.latLng(lat, lng));
      map.fitBounds(L.latLngBounds(latlngs), { padding: [40, 40], maxZoom: 17 });
    }
  }

  function startNewDraw() {
    setPlaceOdpMode(false);
    setEditId(null);
    setRouteName("");
    setRouteColor(DEFAULT_ROUTE_COLOR);
    setDraft([]);
    setDrawMode(true);
    setSaveErr("");
  }

  async function exportRoutesBackup() {
    try {
      const qs =
        clusterId && clusterId !== "__none__" ? `?cluster_id=${encodeURIComponent(clusterId)}` : "";
      const res = await fetch(`/api/cable-routes/export.json${qs}`, {
        credentials: "include",
        headers: { Authorization: getToken() ? `Bearer ${getToken()}` : "" },
      });
      if (!res.ok) throw new Error(await res.text());
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = clusterId && clusterId !== "__none__" ? `cable-routes-${clusterId.slice(0, 8)}.json` : "cable-routes.json";
      a.click();
      URL.revokeObjectURL(url);
    } catch (e: unknown) {
      setBackupMsg(e instanceof Error ? e.message : "Export gagal");
      setBackupOpen(true);
    }
  }

  async function importRoutesBackup(file: File) {
    setBackupBusy(true);
    setBackupMsg("");
    try {
      const text = await file.text();
      const parsed = JSON.parse(text) as {
        routes?: { name: string; path: unknown; color?: string; notes?: string | null; is_active?: boolean }[];
        cluster_code?: string;
      };
      if (!parsed.routes?.length) throw new Error("File backup tidak berisi routes");
      const body: Record<string, unknown> = {
        routes: parsed.routes,
        cluster_code: parsed.cluster_code,
      };
      if (clusterId && clusterId !== "__none__") body.cluster_id = clusterId;
      const res = await api<{ created: number; updated: number; skipped: number; errors?: string[] }>(
        "/api/cable-routes/import",
        { method: "POST", body: JSON.stringify(body) },
      );
      const errHint = res.errors?.length ? ` · ${res.errors.slice(0, 3).join("; ")}` : "";
      setBackupMsg(`Restore selesai: ${res.created} baru, ${res.updated} diupdate, ${res.skipped} dilewati${errHint}`);
      qc.invalidateQueries({ queryKey: ["cable-routes"] });
    } catch (e: unknown) {
      setBackupMsg(e instanceof Error ? e.message : "Import gagal");
    } finally {
      setBackupBusy(false);
    }
  }

  const matchCluster = (id?: string | null) => {
    if (!clusterId) return true;
    if (clusterId === "__none__") return !id;
    return id === clusterId;
  };

  const odpsList = (assetsQ.data?.odps ?? odps).filter((o) => matchCluster(o.cluster_id));
  const customers = (assetsQ.data?.customers ?? []).filter((c) => matchCluster(c.cluster_id));
  const clusters = (assetsQ.data?.clusters ?? []).filter((c) => {
    if (!clusterId || clusterId === "__none__") return false;
    return c.id === clusterId;
  });
  const mapIcons = assetsQ.data?.icons;
  const routesRaw = Array.isArray(routesQ.data) ? routesQ.data : [];
  const routes = routesRaw.filter((r) => matchCluster(r.cluster_id));

  useEffect(() => {
    if (!mapRef.current || mapObj.current || !window.L) return;
    const L = window.L;
    const start = clusterCenter ?? { lat: -6.2, lng: 106.8 };
    const map = L.map(mapRef.current).setView([start.lat, start.lng], clusterCenter ? 15 : 13);
    layers.current = L.layerGroup().addTo(map);
    draftLayer.current = L.layerGroup().addTo(map);
    mapObj.current = map;
    const onMapClick = (e: { latlng: { lat: number; lng: number } }) => {
      if (drawModeRef.current) {
        addDraftPoint(e.latlng.lat, e.latlng.lng);
        return;
      }
      if (placeOdpModeRef.current) {
        openCreateOdpAt(e.latlng.lat, e.latlng.lng);
      }
    };
    map.on("click", onMapClick);
    setTimeout(() => map.invalidateSize(), 100);
    const ro = typeof ResizeObserver !== "undefined"
      ? new ResizeObserver(() => {
          map.invalidateSize({ animate: false });
        })
      : null;
    if (ro && mapRef.current) ro.observe(mapRef.current);
    return () => {
      ro?.disconnect();
      map.off("click", onMapClick);
      map.remove();
      mapObj.current = null;
      baseLayer.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- init once
  }, []);

  useEffect(() => {
    const L = window.L;
    const map = mapObj.current;
    if (!L || !map) return;
    try {
      localStorage.setItem(BASEMAP_KEY, basemap);
    } catch {
      /* ignore */
    }
    const next = createBasemapLayer(L, basemap);
    if (baseLayer.current) {
      map.removeLayer(baseLayer.current);
    }
    next.addTo(map);
    next.bringToBack?.();
    baseLayer.current = next;
  }, [basemap]);

  useEffect(() => {
    const map = mapObj.current;
    if (!map) return;
    setDrawMode(false);
    setPlaceOdpMode(false);
    setEditId(null);
    setDraft([]);
    setRouteName("");
    setRouteColor(DEFAULT_ROUTE_COLOR);
    setSaveErr("");
    if (!clusterCenter) return;
    map.setView([clusterCenter.lat, clusterCenter.lng], 15, { animate: true });
    setTimeout(() => map.invalidateSize(), 50);
  }, [clusterId, clusterCenter?.lat, clusterCenter?.lng]);

  useEffect(() => {
    const L = window.L;
    const map = mapObj.current;
    const group = layers.current;
    if (!L || !map || !group) return;
    group.clearLayers();

    const pts: LatLng[] = [];
    const bounds: [number, number][] = [];
    const snapClick = (lat: number, lng: number) => (e: { originalEvent?: Event }) => {
      if (!drawModeRef.current) return;
      e.originalEvent?.preventDefault?.();
      e.originalEvent?.stopPropagation?.();
      addDraftPoint(lat, lng);
    };
    for (const c of clusters) {
      if (c.latitude == null || c.longitude == null) continue;
      pts.push({ lat: c.latitude, lng: c.longitude });
      const ring = addCoverageCircle(L, group, c.latitude, c.longitude, c.coverage_radius_km, MAP_MARKER.pop.color);
      if (ring?.getBounds) {
        const b = ring.getBounds();
        bounds.push([b.getSouth(), b.getWest()], [b.getNorth(), b.getEast()]);
      } else {
        bounds.push([c.latitude, c.longitude]);
      }
      L.marker([c.latitude, c.longitude], { icon: createMapMarkerIcon(L, "pop", mapIcons?.pop), zIndexOffset: 300 })
        .bindPopup(
          `<b>POP ${c.name}</b><br/>${c.code}` +
            (c.coverage_radius_km ? `<br/>Coverage ${c.coverage_radius_km} km` : "") +
            `<br/><span style="opacity:.7">Klik saat mode gambar untuk snap</span>`,
        )
        .on("click", snapClick(c.latitude, c.longitude))
        .addTo(group);
    }
    for (const o of odpsList) {
      if (o.latitude == null || o.longitude == null) continue;
      pts.push({ lat: o.latitude, lng: o.longitude });
      const ring = addCoverageCircle(L, group, o.latitude, o.longitude, o.coverage_radius_km, MAP_MARKER.odp.color);
      if (ring?.getBounds) {
        const b = ring.getBounds();
        bounds.push([b.getSouth(), b.getWest()], [b.getNorth(), b.getEast()]);
      } else {
        bounds.push([o.latitude, o.longitude]);
      }
      L.marker([o.latitude, o.longitude], { icon: createMapMarkerIcon(L, "odp", mapIcons?.odp), zIndexOffset: 200 })
        .bindPopup(
          `<b>ODP ${o.name}</b><br/>${o.code}<br/>Port terpakai ${o.used_ports ?? 0}/${o.port_count}` +
            (o.free_ports != null ? ` · sisa ${o.free_ports}` : "") +
            (o.coverage_radius_km ? `<br/>Coverage ${o.coverage_radius_km} km` : ""),
        )
        .on("click", snapClick(o.latitude, o.longitude))
        .addTo(group);
    }
    for (const c of customers) {
      if (c.latitude == null || c.longitude == null) continue;
      pts.push({ lat: c.latitude, lng: c.longitude });
      L.marker([c.latitude, c.longitude], { icon: createMapMarkerIcon(L, "customer", mapIcons?.customer), zIndexOffset: 100 })
        .bindPopup(`<b>${c.full_name}</b><br/>${c.customer_code}`)
        .on("click", snapClick(c.latitude, c.longitude))
        .addTo(group);
    }
    for (const r of routes) {
      if (editIdRef.current && r.id === editIdRef.current) continue;
      const path = parsePath(r.path);
      if (path.length < 2) continue;
      const latlngs = path.map(([lng, lat]) => L.latLng(lat, lng));
      L.polyline(latlngs, { color: r.color || DEFAULT_ROUTE_COLOR, weight: 4 })
        .bindPopup(`<b>${r.name}</b><br/>${formatDistance(pathLengthMeters(path))} · ${path.length} titik`)
        .addTo(group);
      for (const ll of latlngs) pts.push({ lat: ll.lat, lng: ll.lng });
    }

    if ((bounds.length > 0 || pts.length > 0) && !clusterCenter) {
      const fit = bounds.length > 0 ? bounds : pts.map((p) => [p.lat, p.lng] as [number, number]);
      map.fitBounds(L.latLngBounds(fit), { padding: [40, 40], maxZoom: 16 });
    }
  }, [odpsList, customers, clusters, routes, editId, clusterCenter, mapIcons]);

  useEffect(() => {
    const el = mapRef.current;
    if (!el) return;
    el.style.cursor = placeOdpMode || drawMode ? "crosshair" : "";
    return () => {
      el.style.cursor = "";
    };
  }, [placeOdpMode, drawMode]);

  useEffect(() => {
    if (!placeOdpMode) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") cancelPlaceOdp();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [placeOdpMode]);

  useEffect(() => {
    const L = window.L;
    const group = draftLayer.current;
    if (!L || !group) return;
    group.clearLayers();
    if (draft.length === 0) return;
    const latlngs = draft.map(([lng, lat]) => L.latLng(lat, lng));
    L.polyline(latlngs, { color: "#dc2626", weight: 3, dashArray: "6 4" }).addTo(group);
    for (const ll of latlngs) {
      L.circleMarker(ll, { radius: 4, color: "#dc2626", fillOpacity: 1 }).addTo(group);
    }
  }, [draft]);

  const saving = saveRoute.isPending;
  const draftMeters = pathLengthMeters(draft);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <button type="button" className="btn" onClick={openCreateOdp} disabled={!clusterId || clusterId === "__none__" || placeOdpMode}>
          + ODP
        </button>
        {!placeOdpMode ? (
          <button
            type="button"
            className="btn-ghost"
            onClick={startPlaceOdp}
            disabled={!clusterId || clusterId === "__none__" || drawMode}
            title="Klik di peta untuk menentukan koordinat ODP"
          >
            Pasang di peta
          </button>
        ) : (
          <>
            <span className="text-sm font-semibold text-[var(--accent)]">Klik di peta untuk pasang ODP</span>
            <button type="button" className="btn-ghost" onClick={cancelPlaceOdp}>
              Batal
            </button>
          </>
        )}
        {!drawMode ? (
          <button type="button" className="btn-ghost" onClick={startNewDraw} disabled={placeOdpMode}>
            Gambar jalur kabel
          </button>
        ) : (
          <>
            <span className="text-sm text-[var(--muted)]">
              {editId ? "Edit jalur" : "Gambar jalur"} — {draft.length} titik
              {draft.length >= 2 ? ` · ${formatDistance(draftMeters)}` : ""}
            </span>
            <button type="button" className="btn-ghost" onClick={() => setDraft((d) => d.slice(0, -1))} disabled={draft.length === 0}>
              Undo
            </button>
            <button
              type="button"
              className="btn-ghost"
              onClick={() => setDraft([])}
              disabled={draft.length === 0}
              title="Hapus semua titik"
            >
              Reset titik
            </button>
            <button
              type="button"
              className="btn"
              disabled={draft.length < 2}
              onClick={() => {
                setSaveErr("");
                setSaveOpen(true);
              }}
            >
              {editId ? "Simpan perubahan" : "Simpan jalur"}
            </button>
            <button type="button" className="btn-ghost" onClick={cancelDraw}>
              Batal
            </button>
          </>
        )}
        <span className="ml-auto flex items-center gap-1.5">
          <IconButton label="Backup jalur (JSON)" onClick={() => void exportRoutesBackup()}>
            <IconDownload />
          </IconButton>
          <IconButton
            label="Restore jalur (JSON)"
            onClick={() => {
              setBackupMsg("");
              setBackupOpen(true);
            }}
          >
            <IconUpload />
          </IconButton>
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
        <div className="ftth-map-legend" aria-label="Legenda marker peta">
          <span className="ftth-map-legend-item">
            <span className="ftth-map-legend-swatch ftth-map-legend-swatch--pop" aria-hidden />
            POP
          </span>
          <span className="ftth-map-legend-item">
            <span className="ftth-map-legend-swatch ftth-map-legend-swatch--odp" aria-hidden />
            ODP
          </span>
          <span className="ftth-map-legend-item">
            <span className="ftth-map-legend-swatch ftth-map-legend-swatch--customer" aria-hidden />
            Pelanggan
          </span>
          <span className="ftth-map-legend-item">
            <span className="ftth-map-legend-swatch ftth-map-legend-swatch--ring-pop" aria-hidden />
            Area POP
          </span>
          <span className="ftth-map-legend-item">
            <span className="ftth-map-legend-swatch ftth-map-legend-swatch--ring-odp" aria-hidden />
            Area ODP
          </span>
        </div>
      </div>
      <div className="ftth-map-shell">
        <div className="ftth-map-basemap" role="group" aria-label="Tampilan peta">
          <button
            type="button"
            className={`ftth-map-basemap-btn${basemap === "street" ? " is-active" : ""}`}
            aria-pressed={basemap === "street"}
            onClick={() => setBasemap("street")}
          >
            Peta
          </button>
          <button
            type="button"
            className={`ftth-map-basemap-btn${basemap === "satellite" ? " is-active" : ""}`}
            aria-pressed={basemap === "satellite"}
            onClick={() => setBasemap("satellite")}
          >
            Satelit
          </button>
        </div>
        <div
          ref={mapRef}
          className="ftth-map-frame z-0 w-full overflow-hidden rounded-xl border border-[var(--border)]"
        />
      </div>

      {routes.length > 0 && (
        <Table
          columns={["Jalur", "Titik", "Jarak", "Aksi"]}
          rows={routes.map((r) => {
            const path = parsePath(r.path);
            return [
              <span key="n" className={editId === r.id ? "font-semibold text-[var(--accent)]" : undefined}>
                {r.name}
                {editId === r.id ? " (sedang diedit)" : ""}
              </span>,
              String(path.length),
              formatDistance(pathLengthMeters(path)),
              <span key="act" className="flex flex-wrap items-center gap-1.5">
                <IconButton
                  label="Edit jalur"
                  disabled={drawMode && editId !== r.id}
                  onClick={() => startEdit(r)}
                >
                  <IconPencil />
                </IconButton>
                <IconButton
                  label="Hapus jalur"
                  danger
                  disabled={removeRoute.isPending}
                  onClick={async () => {
                    const ok = await confirm({
                      title: "Hapus jalur",
                      description: `Hapus jalur "${r.name}"?`,
                      confirmLabel: "Hapus",
                    });
                    if (!ok) return;
                    removeRoute.mutate(r.id);
                  }}
                >
                  <IconTrash />
                </IconButton>
              </span>,
            ];
          })}
        />
      )}

      <FormDialog
        open={saveOpen}
        title={editId ? "Simpan perubahan jalur" : "Simpan jalur kabel"}
        onClose={() => setSaveOpen(false)}
      >
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            saveRoute.mutate();
          }}
        >
          <input
            className="input"
            placeholder="Nama jalur (mis. POP Delima → ODP-01)"
            value={routeName}
            onChange={(e) => setRouteName(e.target.value)}
          />
          <div className="grid gap-2">
            <span className="text-sm text-[var(--muted)]">Warna jalur</span>
            <div className="flex items-center gap-2">
              <input
                type="color"
                value={routeColor}
                onChange={(e) => pickSwatch(e.target.value)}
                className="h-9 w-14 shrink-0 cursor-pointer rounded border border-[var(--border)] bg-transparent p-0.5"
                aria-label="Pilih warna"
              />
              <input
                className="input w-28 font-mono uppercase"
                value={hexDraft ?? routeColor}
                maxLength={7}
                spellCheck={false}
                placeholder="#8C7355"
                aria-label="Kode hex warna"
                onChange={(e) => {
                  const v = e.target.value;
                  setHexDraft(v);
                  if (normalizeHexColor(v)) applyHex(v);
                }}
                onBlur={(e) => {
                  if (hexDraft !== null && !normalizeHexColor(e.target.value)) setHexDraft(null);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") (e.target as HTMLInputElement).blur();
                }}
              />
              <span
                aria-hidden
                className="h-6 flex-1 rounded-md border border-[var(--border)]"
                style={{ background: routeColor }}
              />
            </div>
            <div className="flex flex-wrap gap-1.5" role="group" aria-label="Warna standar fiber">
              {ROUTE_COLOR_PRESETS.map((c) => {
                const active = c.toLowerCase() === routeColor.toLowerCase();
                return (
                  <button
                    key={c}
                    type="button"
                    title={c}
                    aria-label={`Warna ${c}`}
                    aria-pressed={active}
                    onClick={() => pickSwatch(c)}
                    className="h-7 w-7 rounded-md border border-[var(--border)]"
                    style={{
                      background: c,
                      boxShadow: active ? "0 0 0 2px var(--panel), 0 0 0 4px var(--accent)" : undefined,
                    }}
                  />
                );
              })}
            </div>
            {recentColors.length > 0 ? (
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-xs text-[var(--muted)]">Terakhir:</span>
                {recentColors.map((c) => (
                  <button
                    key={c}
                    type="button"
                    title={c}
                    aria-label={`Warna ${c}`}
                    onClick={() => pickSwatch(c)}
                    className="h-6 w-6 rounded-md border border-[var(--border)]"
                    style={{ background: c }}
                  />
                ))}
              </div>
            ) : null}
          </div>
          <p className="text-xs text-[var(--muted)]">
            {draft.length} titik · panjang sekitar <strong className="text-[var(--text)]">{formatDistance(draftMeters)}</strong>
            {draftMeters >= 1 ? ` (${Math.round(draftMeters)} m)` : ""}.
          </p>
          <div className="flex gap-2">
            <button className="btn" disabled={saving}>
              {saving ? "Menyimpan…" : "Simpan"}
            </button>
            <button type="button" className="btn-ghost" onClick={() => setSaveOpen(false)}>
              Batal
            </button>
          </div>
          {saveErr && <p className="text-sm text-[var(--danger)]">{saveErr}</p>}
        </form>
      </FormDialog>

      <FormDialog open={backupOpen} title="Restore jalur kabel" onClose={() => setBackupOpen(false)}>
        <div className="grid gap-3">
          <p className="text-sm text-[var(--muted)]">
            Upload file backup <code className="text-xs">.json</code> (hasil export). Upsert berdasarkan nama jalur dalam cluster tab aktif.
          </p>
          <div className="flex flex-wrap gap-2">
            <label className="btn cursor-pointer">
              {backupBusy ? "Memulihkan…" : "Pilih file JSON"}
              <input
                type="file"
                accept=".json,application/json"
                className="hidden"
                disabled={backupBusy}
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  e.target.value = "";
                  if (f) void importRoutesBackup(f);
                }}
              />
            </label>
            <button type="button" className="btn-ghost" onClick={() => setBackupOpen(false)}>
              Tutup
            </button>
          </div>
          {backupMsg && <p className="text-sm text-[var(--text-body)] whitespace-pre-wrap">{backupMsg}</p>}
        </div>
      </FormDialog>

      <FormDialog open={odpOpen} wide title="Tambah ODP" onClose={() => setOdpOpen(false)}>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            createOdp.mutate();
          }}
        >
          <input className="input" placeholder="Nama" value={odpForm.name} onChange={(e) => setOdpForm({ ...odpForm, name: e.target.value })} required autoFocus />
          <input className="input" placeholder="Kode" value={odpForm.code} onChange={(e) => setOdpForm({ ...odpForm, code: e.target.value })} required />
          <input className="input" type="number" step="any" placeholder="Latitude" value={odpForm.latitude} onChange={(e) => setOdpForm({ ...odpForm, latitude: e.target.value })} />
          <input className="input" type="number" step="any" placeholder="Longitude" value={odpForm.longitude} onChange={(e) => setOdpForm({ ...odpForm, longitude: e.target.value })} />
          <input className="input" type="number" placeholder="Jumlah port" value={odpForm.port_count} onChange={(e) => setOdpForm({ ...odpForm, port_count: Number(e.target.value) || 8 })} />
          <p className="text-xs text-[var(--muted)] sm:col-span-2">
            ODP terhubung ke cluster tab aktif. Lat/long bisa diisi manual atau dari Pasang di peta.
          </p>
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <button className="btn" disabled={createOdp.isPending}>
              {createOdp.isPending ? "Menyimpan…" : "Simpan"}
            </button>
            <button type="button" className="btn-ghost" onClick={() => setOdpOpen(false)}>
              Batal
            </button>
          </div>
          {odpErr && <p className="text-sm text-[var(--danger)] sm:col-span-2">{odpErr}</p>}
        </form>
      </FormDialog>
    </div>
  );
}
