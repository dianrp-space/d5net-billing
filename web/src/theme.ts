import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api";

const THEME_KEY = "drp_theme";
export const DEFAULT_PRIMARY = "#5a5a40";

const ACCENT_VARS = ["--accent", "--accent-hover", "--chart-bar", "--focus-ring", "--primary", "--ring"] as const;

export type Theme = "light" | "dark";

let tenantPrimary: string | null = null;

export function getStoredTheme(): Theme {
  const v = localStorage.getItem(THEME_KEY);
  if (v === "dark" || v === "light") return v;
  if (window.matchMedia("(prefers-color-scheme: dark)").matches) return "dark";
  return "light";
}

export function parseHexColor(input?: string | null): string | null {
  const s = String(input || "").trim();
  const m = s.match(/^#?([0-9a-fA-F]{6})$/);
  if (!m) return null;
  return `#${m[1].toLowerCase()}`;
}

function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const n = Number.parseInt(hex.slice(1), 16);
  return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
}

function mix(hex: string, toward: number, amount: number): string {
  const { r, g, b } = hexToRgb(hex);
  const f = (v: number) => Math.max(0, Math.min(255, Math.round(v + (toward - v) * amount)));
  return `#${[f(r), f(g), f(b)].map((x) => x.toString(16).padStart(2, "0")).join("")}`;
}

function applyPrimaryColorVars() {
  const root = document.documentElement;
  if (!tenantPrimary) {
    for (const key of ACCENT_VARS) root.style.removeProperty(key);
    return;
  }
  const dark = root.dataset.theme === "dark";
  const accent = dark ? mix(tenantPrimary, 255, 0.18) : tenantPrimary;
  const hover = dark ? mix(tenantPrimary, 255, 0.32) : mix(tenantPrimary, 0, 0.16);
  const { r, g, b } = hexToRgb(accent);
  root.style.setProperty("--accent", accent);
  root.style.setProperty("--accent-hover", hover);
  root.style.setProperty("--chart-bar", accent);
  root.style.setProperty("--primary", accent);
  root.style.setProperty("--ring", accent);
  root.style.setProperty("--focus-ring", `rgba(${r}, ${g}, ${b}, ${dark ? 0.25 : 0.12})`);
}

/** Preview or persist a tenant primary (empty/null restores theme default). */
export function setTenantPrimaryColor(hex?: string | null) {
  tenantPrimary = parseHexColor(hex);
  applyPrimaryColorVars();
}

export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem(THEME_KEY, theme);
  applyPrimaryColorVars();
}

export function toggleTheme(): Theme {
  const next: Theme = getStoredTheme() === "dark" ? "light" : "dark";
  applyTheme(next);
  return next;
}

/** Call once before React paint (also duplicated inline in index.html). */
export function initTheme() {
  applyTheme(getStoredTheme());
}

export type ChartColors = {
  bar: string;
  axis: string;
  grid: string;
  text: string;
  panel: string;
  border: string;
};

function readChartColors(): ChartColors {
  const cs = getComputedStyle(document.documentElement);
  const v = (key: string, fallback: string) => cs.getPropertyValue(key).trim() || fallback;
  return {
    bar: v("--chart-bar", "#5a5a40"),
    axis: v("--chart-axis", "#6b6962"),
    grid: v("--chart-grid", "#f1f0ea"),
    text: v("--text", "#1a1a1a"),
    panel: v("--panel", "#ffffff"),
    border: v("--border", "#e5e2d8"),
  };
}

/**
 * Warna chart yang di-resolve dari CSS vars (ECharts tidak memahami `var(--…)`).
 * Otomatis ikut berubah saat tema light/dark atau accent tenant berubah.
 */
export function useChartColors(): ChartColors {
  const [colors, setColors] = useState<ChartColors>(readChartColors);
  useEffect(() => {
    const obs = new MutationObserver(() => setColors(readChartColors()));
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme", "style"] });
    return () => obs.disconnect();
  }, []);
  return colors;
}

/** Loads provider primary color for login / admin / portal / isolir. */
export function TenantAccent() {
  const q = useQuery({
    queryKey: ["public-branding"],
    queryFn: () => api<{ primary_color?: string | null }>("/api/public/branding"),
    retry: false,
  });

  useEffect(() => {
    setTenantPrimaryColor(q.data?.primary_color);
    return () => setTenantPrimaryColor(null);
  }, [q.data?.primary_color]);

  return null;
}
