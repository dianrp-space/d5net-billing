import { useEffect, useState, type ReactNode } from "react";
import { IconMoon, IconSun } from "./icons";
import { applyTheme, getStoredTheme, type Theme } from "./theme";
import { IconButton } from "./ui";

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(() =>
    typeof document !== "undefined" ? getStoredTheme() : "light",
  );

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  const isDark = theme === "dark";

  return (
    <IconButton
      label={isDark ? "Mode terang" : "Mode gelap"}
      onClick={() => setTheme(isDark ? "light" : "dark")}
    >
      {isDark ? <IconSun /> : <IconMoon />}
    </IconButton>
  );
}

export function AuthThemeCorner({ children }: { children: ReactNode }) {
  return (
    <div className="relative min-h-full">
      <div className="absolute right-4 top-4 z-10">
        <ThemeToggle />
      </div>
      {children}
    </div>
  );
}
