import { useEffect, useState } from "react";

export function usePath() {
  const [path, setPath] = useState(() => window.location.pathname);

  useEffect(() => {
    const onPop = () => setPath(window.location.pathname);
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  function navigate(to: string) {
    if (to === path) return;
    window.history.pushState({}, "", to);
    setPath(to);
  }

  return { path, navigate };
}

export function startsWith(path: string, prefix: string) {
  return path === prefix || path.startsWith(prefix + "/") || path === prefix.slice(0, -1);
}
