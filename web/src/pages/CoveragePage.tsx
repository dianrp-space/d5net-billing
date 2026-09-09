import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { IconCheck } from "../icons";
import { toastError, toastSuccess } from "../swal";
import {
  addCoverageCircle,
  createBasemapLayer,
  createMapMarkerIcon,
  MAP_MARKER,
  readBasemap,
  type Basemap,
} from "../FtthMap";
import { IconButton, Button, Input, Label, Section, Table } from "../ui";
import { Checkbox } from "@/components/ui/checkbox";

type ClusterRow = {
  id: string;
  name: string;
  code: string;
  is_active: boolean;
  latitude?: number | null;
  longitude?: number | null;
  coverage_radius_km?: number | null;
};

type OdpRow = {
  id: string;
  name: string;
  code: string;
  latitude?: number | null;
  longitude?: number | null;
  coverage_radius_km?: number | null;
  free_ports?: number;
};

type MapIcons = { pop?: string | null; odp?: string | null };

type CoverageHit = {
  kind: "pop" | "odp" | string;
  id: string;
  name: string;
  code: string;
  distance_m: number;
  radius_km: number;
  covered: boolean;
  free_ports?: number | null;
};

type CoverageCheck = { covered: boolean; hits: CoverageHit[] };

function parseCoordPair(raw: string): { lat: number; lng: number } | null {
  const m = raw.trim().match(/(-?\d+(?:[.,]\d+)?)\s*[,;\s]\s*(-?\d+(?:[.,]\d+)?)/);
  if (!m) return null;
  let lat = Number(m[1].replace(",", "."));
  let lng = Number(m[2].replace(",", "."));
  if (!Number.isFinite(lat) || !Number.isFinite(lng)) return null;
  if (Math.abs(lat) > 90 && Math.abs(lng) <= 90) {
    const swap = lat;
    lat = lng;
    lng = swap;
  }
  if (lat < -90 || lat > 90 || lng < -180 || lng > 180) return null;
  return { lat, lng };
}

function parseLatLng(latRaw: string, lngRaw: string): { lat: number; lng: number } | null {
  const pasted = parseCoordPair(`${latRaw} ${lngRaw}`.trim()) || parseCoordPair(latRaw) || parseCoordPair(lngRaw);
  if (pasted) return pasted;
  const lat = Number(latRaw.trim().replace(",", "."));
  const lng = Number(lngRaw.trim().replace(",", "."));
  if (!Number.isFinite(lat) || !Number.isFinite(lng)) return null;
  if (lat < -90 || lat > 90 || lng < -180 || lng > 180) return null;
  return { lat, lng };
}

function parseCoverageKm(raw: string): number | null {
  const t = raw.trim().replace(",", ".");
  if (!t) return null;
  const n = Number(t);
  if (!Number.isFinite(n) || n <= 0) return null;
  return Math.min(50, Math.round(n * 1000) / 1000);
}

function formatKm(n?: number | null) {
  if (n == null || n <= 0) return "—";
  return `${n} km`;
}

function formatDistance(meters: number) {
  if (!Number.isFinite(meters) || meters < 0) return "—";
  if (meters < 1000) return `${Math.round(meters)} m`;
  return `${(meters / 1000).toFixed(meters < 10000 ? 2 : 1)} km`;
}

function RadiusEditor({
  value,
  disabled,
  saving,
  onSave,
}: {
  value?: number | null;
  disabled?: boolean;
  saving?: boolean;
  onSave: (km: number | null) => void;
}) {
  const [text, setText] = useState(value != null && value > 0 ? String(value) : "");
  useEffect(() => {
    setText(value != null && value > 0 ? String(value) : "");
  }, [value]);
  return (
    <span className="flex items-center gap-1.5">
      <Input
        className="h-8 w-20"
        type="number"
        min={0}
        max={50}
        step={0.05}
        placeholder="km"
        disabled={disabled || saving}
        value={text}
        onChange={(e) => setText(e.target.value)}
        aria-label="Radius coverage (km)"
      />
      <IconButton
        label="Simpan radius"
        disabled={disabled || saving}
        onClick={() => onSave(parseCoverageKm(text))}
      >
        <IconCheck />
      </IconButton>
    </span>
  );
}

export function CoveragePage({ canEdit }: { canEdit: boolean }) {
  const qc = useQueryClient();
  const mapRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const mapObj = useRef<any>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const layers = useRef<any>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const probeLayer = useRef<any>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const baseLayer = useRef<any>(null);

  const [basemap, setBasemap] = useState<Basemap>(() => readBasemap());
  const [showPop, setShowPop] = useState(true);
  const [showOdp, setShowOdp] = useState(true);
  const [probe, setProbe] = useState<{ lat: number; lng: number } | null>(null);
  const [latText, setLatText] = useState("");
  const [lngText, setLngText] = useState("");
  const [pasteText, setPasteText] = useState("");
  const [coordErr, setCoordErr] = useState("");

  const clustersQ = useQuery({
    queryKey: ["clusters"],
    queryFn: () => api<ClusterRow[]>("/api/clusters"),
  });
  const odpsQ = useQuery({
    queryKey: ["odps"],
    queryFn: () => api<OdpRow[]>("/api/odps"),
  });
  const assetsQ = useQuery({
    queryKey: ["ftth-map"],
    queryFn: () => api<{ icons?: MapIcons }>("/api/ftth/map-assets"),
  });

  const clusters = (Array.isArray(clustersQ.data) ? clustersQ.data : []).filter((c) => c.is_active);
  const odps = Array.isArray(odpsQ.data) ? odpsQ.data : [];
  const icons = assetsQ.data?.icons;

  const checkQ = useQuery({
    queryKey: ["coverage-check", probe?.lat, probe?.lng],
    queryFn: () =>
      api<CoverageCheck>(`/api/coverage-check?lat=${probe!.lat}&lng=${probe!.lng}`),
    enabled: Boolean(probe),
  });

  const savePop = useMutation({
    mutationFn: ({ id, km }: { id: string; km: number | null }) =>
      api(`/api/clusters/${id}/coverage`, {
        method: "PUT",
        body: JSON.stringify({ coverage_radius_km: km }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["clusters"] });
      void qc.invalidateQueries({ queryKey: ["ftth-map"] });
      if (probe) void qc.invalidateQueries({ queryKey: ["coverage-check"] });
      void toastSuccess("Coverage POP disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });
  const saveOdp = useMutation({
    mutationFn: ({ id, km }: { id: string; km: number | null }) =>
      api(`/api/odps/${id}/coverage`, {
        method: "PUT",
        body: JSON.stringify({ coverage_radius_km: km }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["odps"] });
      void qc.invalidateQueries({ queryKey: ["ftth-map"] });
      if (probe) void qc.invalidateQueries({ queryKey: ["coverage-check"] });
      void toastSuccess("Coverage ODP disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const popsWithCoords = useMemo(
    () => clusters.filter((c) => c.latitude != null && c.longitude != null),
    [clusters],
  );
  const odpsWithCoords = useMemo(
    () => odps.filter((o) => o.latitude != null && o.longitude != null),
    [odps],
  );
  const coveredIds = useMemo(() => {
    const hits = checkQ.data?.hits ?? [];
    return new Set(hits.filter((h) => h.covered).map((h) => `${h.kind}:${h.id}`));
  }, [checkQ.data?.hits]);
  const hasAnyRadius =
    popsWithCoords.some((c) => c.coverage_radius_km != null && c.coverage_radius_km > 0) ||
    odpsWithCoords.some((o) => o.coverage_radius_km != null && o.coverage_radius_km > 0);

  const applyProbe = useCallback((next: { lat: number; lng: number }, source: "map" | "form") => {
    setProbe(next);
    setCoordErr("");
    const lat = next.lat.toFixed(6);
    const lng = next.lng.toFixed(6);
    setLatText(lat);
    setLngText(lng);
    setPasteText(`${lat}, ${lng}`);
    const map = mapObj.current;
    if (map && source === "form") {
      map.setView([next.lat, next.lng], Math.max(map.getZoom?.() ?? 15, 15), { animate: true });
    }
  }, []);
  const applyProbeRef = useRef(applyProbe);
  applyProbeRef.current = applyProbe;

  useEffect(() => {
    if (!mapRef.current || mapObj.current || !window.L) return;
    const L = window.L;
    const map = L.map(mapRef.current).setView([-6.2, 106.8], 12);
    layers.current = L.layerGroup().addTo(map);
    probeLayer.current = L.layerGroup().addTo(map);
    mapObj.current = map;
    const onClick = (e: { latlng: { lat: number; lng: number } }) => {
      applyProbeRef.current({ lat: e.latlng.lat, lng: e.latlng.lng }, "map");
    };
    map.on("click", onClick);
    const ro =
      typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(() => map.invalidateSize({ animate: false }))
        : null;
    if (ro && mapRef.current) ro.observe(mapRef.current);
    setTimeout(() => map.invalidateSize(), 100);
    return () => {
      ro?.disconnect();
      map.off("click", onClick);
      map.remove();
      mapObj.current = null;
      baseLayer.current = null;
    };
  }, []);

  useEffect(() => {
    const L = window.L;
    const map = mapObj.current;
    if (!L || !map) return;
    try {
      localStorage.setItem("drp_ftth_basemap", basemap);
    } catch {
      /* ignore */
    }
    const next = createBasemapLayer(L, basemap);
    if (baseLayer.current) map.removeLayer(baseLayer.current);
    next.addTo(map);
    next.bringToBack?.();
    baseLayer.current = next;
  }, [basemap]);

  useEffect(() => {
    const L = window.L;
    const map = mapObj.current;
    const group = layers.current;
    if (!L || !map || !group) return;
    group.clearLayers();
    const bounds: [number, number][] = [];
    if (showPop) {
      for (const c of popsWithCoords) {
        const lat = c.latitude!;
        const lng = c.longitude!;
        const ring = addCoverageCircle(
          L,
          group,
          lat,
          lng,
          c.coverage_radius_km,
          MAP_MARKER.pop.color,
          coveredIds.has(`pop:${c.id}`),
        );
        if (ring?.getBounds) {
          const b = ring.getBounds();
          bounds.push([b.getSouth(), b.getWest()], [b.getNorth(), b.getEast()]);
        } else {
          bounds.push([lat, lng]);
        }
        L.marker([lat, lng], { icon: createMapMarkerIcon(L, "pop", icons?.pop), zIndexOffset: 300 })
          .bindPopup(
            `<b>POP ${c.name}</b><br/>${c.code}<br/>Coverage ${formatKm(c.coverage_radius_km)}`,
          )
          .addTo(group);
      }
    }
    if (showOdp) {
      for (const o of odpsWithCoords) {
        const lat = o.latitude!;
        const lng = o.longitude!;
        const ring = addCoverageCircle(
          L,
          group,
          lat,
          lng,
          o.coverage_radius_km,
          MAP_MARKER.odp.color,
          coveredIds.has(`odp:${o.id}`),
        );
        if (ring?.getBounds) {
          const b = ring.getBounds();
          bounds.push([b.getSouth(), b.getWest()], [b.getNorth(), b.getEast()]);
        } else {
          bounds.push([lat, lng]);
        }
        L.marker([lat, lng], { icon: createMapMarkerIcon(L, "odp", icons?.odp), zIndexOffset: 200 })
          .bindPopup(
            `<b>ODP ${o.name}</b><br/>${o.code}<br/>Coverage ${formatKm(o.coverage_radius_km)}` +
              (o.free_ports != null ? `<br/>Sisa port ${o.free_ports}` : ""),
          )
          .addTo(group);
      }
    }
    if (bounds.length > 0 && !probe) {
      map.fitBounds(L.latLngBounds(bounds), { padding: [40, 40], maxZoom: 15 });
    }
  }, [popsWithCoords, odpsWithCoords, showPop, showOdp, icons, probe, coveredIds]);

  useEffect(() => {
    const L = window.L;
    const group = probeLayer.current;
    if (!L || !group) return;
    group.clearLayers();
    if (!probe) return;
    L.circleMarker([probe.lat, probe.lng], {
      radius: 8,
      color: "#8C7355",
      weight: 2,
      fillColor: "#8C7355",
      fillOpacity: 0.9,
    })
      .bindPopup("Lokasi cek")
      .addTo(group)
      .openPopup();
  }, [probe]);

  const hits = checkQ.data?.hits ?? [];
  const coveredHits = hits.filter((h) => h.covered);

  function onCheckCoords() {
    const fromFields = parseLatLng(latText, lngText);
    const fromPaste = parseCoordPair(pasteText);
    const next = fromFields || fromPaste;
    if (!next) {
      setCoordErr("Isi latitude & longitude, atau tempel pasangan koordinat.");
      return;
    }
    applyProbe(next, "form");
  }

  function clearProbe() {
    setProbe(null);
    setCoordErr("");
  }

  return (
    <Section title="Coverage">
      <p className="mb-4 max-w-3xl text-sm text-[var(--muted)]">
        Lingkaran berwarna di peta adalah area coverage: hijau zaitun dari pusat POP, biru dari pusat ODP.
        Isi koordinat atau klik peta untuk cek apakah titik itu masuk jangkauan.
        {canEdit ? " Radius (km) diatur di tabel di bawah." : " Radius diatur oleh tim jaringan di Cluster/POP atau ODP."}
      </p>

      <div className="panel-card mb-4 space-y-3 p-4">
        <form
          className="grid gap-3 lg:grid-cols-[minmax(0,1.4fr)_minmax(8rem,0.7fr)_minmax(8rem,0.7fr)_auto] lg:items-end"
          onSubmit={(e) => {
            e.preventDefault();
            onCheckCoords();
          }}
        >
          <div className="grid gap-1.5">
            <Label htmlFor="coverage-paste">Tempel koordinat</Label>
            <Input
              id="coverage-paste"
              placeholder="-6.200000, 106.816666"
              value={pasteText}
              autoComplete="off"
              onChange={(e) => {
                const v = e.target.value;
                setPasteText(v);
                const parsed = parseCoordPair(v);
                if (parsed) {
                  setLatText(parsed.lat.toFixed(6));
                  setLngText(parsed.lng.toFixed(6));
                  setCoordErr("");
                }
              }}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="coverage-lat">Latitude</Label>
            <Input
              id="coverage-lat"
              inputMode="decimal"
              placeholder="-6.2"
              value={latText}
              autoComplete="off"
              onChange={(e) => setLatText(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="coverage-lng">Longitude</Label>
            <Input
              id="coverage-lng"
              inputMode="decimal"
              placeholder="106.8"
              value={lngText}
              autoComplete="off"
              onChange={(e) => setLngText(e.target.value)}
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="submit">Cek coverage</Button>
            {probe ? (
              <Button type="button" variant="secondary" onClick={clearProbe}>
                Hapus pin
              </Button>
            ) : null}
          </div>
        </form>
        {coordErr ? <p className="text-sm text-[var(--danger)]">{coordErr}</p> : null}

        <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={showPop} onCheckedChange={(v) => setShowPop(v === true)} />
            POP
          </label>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={showOdp} onCheckedChange={(v) => setShowOdp(v === true)} />
            ODP
          </label>
          <div className="ftth-map-legend" aria-label="Legenda area coverage">
            <span className="ftth-map-legend-item">
              <span className="ftth-map-legend-swatch ftth-map-legend-swatch--ring-pop" aria-hidden />
              Area POP
            </span>
            <span className="ftth-map-legend-item">
              <span className="ftth-map-legend-swatch ftth-map-legend-swatch--ring-odp" aria-hidden />
              Area ODP
            </span>
            <span className="ftth-map-legend-item">
              <span className="ftth-map-legend-swatch ftth-map-legend-swatch--ring-hit" aria-hidden />
              Masuk jangkauan
            </span>
          </div>
        </div>

        {!hasAnyRadius ? (
          <p className="text-xs text-[var(--muted)]">
            Lingkaran belum tampil karena radius coverage belum diisi. Isi radius POP/ODP di tabel di bawah.
          </p>
        ) : null}

        {probe ? (
          <div>
            <p className="text-xs text-[var(--muted)]">
              {probe.lat.toFixed(6)}, {probe.lng.toFixed(6)}
            </p>
            {checkQ.isFetching ? (
              <p className="mt-1 text-sm text-[var(--muted)]">Mengecek…</p>
            ) : checkQ.data?.covered ? (
              <p className="mt-1 text-sm font-semibold text-[var(--accent)]">Masuk coverage</p>
            ) : (
              <p className="mt-1 text-sm font-semibold text-[var(--danger)]">Di luar coverage</p>
            )}
            {coveredHits.length > 0 ? (
              <ul className="mt-2 grid gap-1 text-sm sm:grid-cols-2">
                {coveredHits.map((h) => (
                  <li key={`${h.kind}-${h.id}`}>
                    <span className="uppercase text-xs text-[var(--muted)]">{h.kind === "pop" ? "POP" : "ODP"}</span>{" "}
                    {h.name} ({h.code}) · {formatDistance(h.distance_m)} / {formatKm(h.radius_km)}
                    {h.kind === "odp" && h.free_ports != null ? ` · sisa ${h.free_ports} port` : ""}
                  </li>
                ))}
              </ul>
            ) : !checkQ.isFetching && hits.length > 0 ? (
              <div className="mt-2">
                <p className="text-xs text-[var(--muted)]">Terdekat (di luar radius):</p>
                <ul className="mt-1 grid gap-1 text-sm sm:grid-cols-2">
                  {hits.slice(0, 3).map((h) => (
                    <li key={`${h.kind}-${h.id}`}>
                      <span className="uppercase text-xs text-[var(--muted)]">{h.kind === "pop" ? "POP" : "ODP"}</span>{" "}
                      {h.name} ({h.code}) · {formatDistance(h.distance_m)} / {formatKm(h.radius_km)}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>

      <div className="ftth-map-shell">
        <div className="ftth-map-basemap" role="group" aria-label="Jenis peta">
          <button
            type="button"
            className={`ftth-map-basemap-btn${basemap === "street" ? " is-active" : ""}`}
            onClick={() => setBasemap("street")}
          >
            Peta
          </button>
          <button
            type="button"
            className={`ftth-map-basemap-btn${basemap === "satellite" ? " is-active" : ""}`}
            onClick={() => setBasemap("satellite")}
          >
            Satelit
          </button>
        </div>
        <div ref={mapRef} className="ftth-map-frame rounded-[var(--radius-lg,0.85rem)] border border-[var(--border)]" />
      </div>

      <h2 className="mb-2 mt-8 text-sm font-semibold">POP</h2>
      <Table
        columns={canEdit ? ["Nama", "Kode", "Koordinat", "Radius", "Aksi"] : ["Nama", "Kode", "Koordinat", "Radius"]}
        rows={popsWithCoords.map((c) => {
          const cells: (string | number | ReactNode)[] = [
            c.name,
            c.code,
            `${c.latitude!.toFixed(5)}, ${c.longitude!.toFixed(5)}`,
            formatKm(c.coverage_radius_km),
          ];
          if (canEdit) {
            cells.push(
              <RadiusEditor
                key={c.id}
                value={c.coverage_radius_km}
                saving={savePop.isPending}
                onSave={(km) => savePop.mutate({ id: c.id, km })}
              />,
            );
          }
          return cells;
        })}
      />

      <h2 className="mb-2 mt-8 text-sm font-semibold">ODP</h2>
      <Table
        columns={canEdit ? ["Nama", "Kode", "Sisa port", "Koordinat", "Radius", "Aksi"] : ["Nama", "Kode", "Sisa port", "Koordinat", "Radius"]}
        rows={odpsWithCoords.map((o) => {
          const cells: (string | number | ReactNode)[] = [
            o.name,
            o.code,
            o.free_ports ?? "—",
            `${o.latitude!.toFixed(5)}, ${o.longitude!.toFixed(5)}`,
            formatKm(o.coverage_radius_km),
          ];
          if (canEdit) {
            cells.push(
              <RadiusEditor
                key={o.id}
                value={o.coverage_radius_km}
                saving={saveOdp.isPending}
                onSave={(km) => saveOdp.mutate({ id: o.id, km })}
              />,
            );
          }
          return cells;
        })}
      />
      {popsWithCoords.length === 0 && odpsWithCoords.length === 0 ? (
        <p className="mt-3 text-sm text-[var(--muted)]">
          Belum ada POP/ODP berkoordinat. Isi lat/long di Cluster / POP atau MAP FTTH.
        </p>
      ) : null}
    </Section>
  );
}
