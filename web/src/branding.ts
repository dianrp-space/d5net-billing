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
  const href = opts.faviconUrl?.trim();
  let link = document.querySelector<HTMLLinkElement>("link[rel='icon'][data-drp-brand]");
  if (!href) {
    link?.remove();
    return;
  }
  if (!link) {
    link = document.createElement("link");
    link.rel = "icon";
    link.setAttribute("data-drp-brand", "1");
    document.head.appendChild(link);
  }
  link.href = href;
}
