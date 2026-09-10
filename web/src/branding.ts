/** Default app logo/favicon shipped with the app (web/public). */
export const DEFAULT_BRAND_LOGO = "/DRP-Gobill-logo.png";

/** Apply favicon (and optional document title) from branding URLs. */
export function applyBrandingMeta(opts: {
  appName?: string | null;
  faviconUrl?: string | null;
  titleSuffix?: string;
  separator?: string;
}) {
  const name = (opts.appName || "").trim();
  if (name) {
    const sep = opts.separator ?? "·";
    document.title = opts.titleSuffix ? `${name} ${sep} ${opts.titleSuffix}` : name;
  }
  const href = opts.faviconUrl?.trim() || DEFAULT_BRAND_LOGO;
  document
    .querySelectorAll("link[rel='icon'], link[rel='shortcut icon'], link[rel='apple-touch-icon']")
    .forEach((el) => {
      if (!el.hasAttribute("data-drp-brand")) el.remove();
    });
  let link = document.querySelector<HTMLLinkElement>("link[rel='icon'][data-drp-brand]");
  let apple = document.querySelector<HTMLLinkElement>("link[rel='apple-touch-icon'][data-drp-brand]");
  if (!link) {
    link = document.createElement("link");
    link.rel = "icon";
    link.setAttribute("data-drp-brand", "1");
    document.head.appendChild(link);
  }
  if (!apple) {
    apple = document.createElement("link");
    apple.rel = "apple-touch-icon";
    apple.setAttribute("data-drp-brand", "1");
    document.head.appendChild(apple);
  }
  const lower = href.toLowerCase();
  if (lower.endsWith(".webp")) link.type = "image/webp";
  else if (lower.endsWith(".png")) link.type = "image/png";
  else if (lower.endsWith(".svg")) link.type = "image/svg+xml";
  else if (lower.endsWith(".ico")) link.type = "image/x-icon";
  else link.removeAttribute("type");
  link.href = href;
  apple.href = href;
}
