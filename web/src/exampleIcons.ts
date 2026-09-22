/** Example map-pin icons tenants can adopt for POP / ODP / customer markers on the FTTH map. */

export type ExampleIconKind = "pop" | "odp" | "customer";

export type ExampleIcon = {
  key: string;
  label: string;
  kind: ExampleIconKind;
  svg: string;
};

const pin = (fill: string, glyph: string): string => `
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64" aria-hidden="true">
  <path d="M32 3C19.3 3 9 13.3 9 26c0 16.5 23 35 23 35s23-18.5 23-35C55 13.3 44.7 3 32 3z" fill="${fill}"/>
  <circle cx="32" cy="26" r="12" fill="#fff"/>
  <g fill="none" stroke="${fill}" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">${glyph}</g>
</svg>`;

const badge = (fill: string, shape: string, glyph: string): string => `
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64" aria-hidden="true">
  ${shape.replace("__FILL__", fill)}
  <g fill="none" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">${glyph}</g>
</svg>`;

const towerGlyph = `<path d="M32 18v28M25 23l7-6 7 6M23 46h18M27 30h10M28 38h8"/>`;
const boxGlyph = `<rect x="18" y="20" width="28" height="24" rx="4"/><path d="M24 27h16M24 33h16M24 39h10"/>`;
const homeGlyph = `<path d="M20 32l12-10 12 10M24 31v14h16V31M30 45v-7h4v7"/>`;

export const EXAMPLE_ICONS: ExampleIcon[] = [
  { key: "pop-tower", label: "POP menara", kind: "pop", svg: pin("#d946ef", `<path d="M32 18v16M28.6 20l3.4-3 3.4 3M27 36h10M29 24h6M30 29h4"/>`) },
  { key: "odp-box", label: "ODP kotak", kind: "odp", svg: pin("#2563eb", `<rect x="25" y="17" width="14" height="12" rx="2"/><path d="M28 21h8M28 24h8M28 27h5"/>`) },
  { key: "customer-home", label: "Pelanggan rumah", kind: "customer", svg: pin("#15803d", `<path d="M25 26l7-6 7 6M27 25.5V33h10v-7.5M30 33v-4h4v4"/>`) },
  { key: "pin-pop", label: "Badge POP", kind: "pop", svg: badge("#d946ef", `<circle cx="32" cy="32" r="30" fill="__FILL__" stroke="#fff" stroke-width="4"/>`, towerGlyph) },
  { key: "pin-odp", label: "Badge ODP", kind: "odp", svg: badge("#2563eb", `<rect x="4" y="4" width="56" height="56" rx="14" fill="__FILL__" stroke="#fff" stroke-width="4"/>`, boxGlyph) },
  { key: "pin-customer", label: "Badge Pelanggan", kind: "customer", svg: badge("#15803d", `<circle cx="32" cy="32" r="30" fill="__FILL__" stroke="#fff" stroke-width="4"/>`, homeGlyph) },
];

/**
 * Raster contoh ikon ke PNG via canvas sebelum diunggah. Server menolak SVG
 * (Stored XSS), jadi contoh bawaan wajib lewat jalur ini.
 * Ukuran 128px mengikuti MaxMapIconEdge di backend.
 */
export async function exampleIconPngFile(icon: ExampleIcon): Promise<File> {
  const url = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(icon.svg)}`;
  const img = new Image();
  await new Promise<void>((resolve, reject) => {
    img.onload = () => resolve();
    img.onerror = () => reject(new Error("gagal render contoh ikon"));
    img.src = url;
  });
  const size = 128;
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("canvas tidak didukung browser ini");
  ctx.drawImage(img, 0, 0, size, size);
  const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/png"));
  if (!blob) throw new Error("gagal render contoh ikon");
  return new File([blob], `${icon.key}.png`, { type: "image/png", lastModified: Date.now() });
}

export function exampleIconDataUrl(icon: ExampleIcon): string {
  return `data:image/svg+xml;utf8,${encodeURIComponent(icon.svg)}`;
}
