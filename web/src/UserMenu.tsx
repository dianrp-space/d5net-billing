import { useState } from "react";
import { api, clearToken } from "./api";
import { IconLogout, IconUser } from "./icons";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export type MeUser = {
  user_id: string;
  email: string;
  full_name: string;
  avatar_url?: string | null;
  role_name?: string;
  role_slug?: string;
};

export function UserAvatar({
  name,
  email,
  avatarUrl,
  className = "",
}: {
  name?: string;
  email?: string;
  avatarUrl?: string | null;
  className?: string;
}) {
  const initial = ((name || email || "U").trim()[0] || "U").toUpperCase();
  if (avatarUrl) {
    return (
      <img
        src={avatarUrl}
        alt=""
        className={`app-user-avatar app-user-avatar--img ${className}`.trim()}
      />
    );
  }
  return <div className={`app-user-avatar ${className}`.trim()}>{initial}</div>;
}

export function UserMenu({
  user,
  onLogout,
  onOpenProfile,
}: {
  user?: MeUser | null;
  onLogout: () => void;
  onOpenProfile: () => void;
}) {
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <>
      <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            className="app-header-user-trigger"
            title="Menu akun"
            aria-label="Menu akun"
          >
            <UserAvatar
              name={user?.full_name}
              email={user?.email}
              avatarUrl={user?.avatar_url}
              className="app-user-avatar--header"
            />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuLabel>
            <div className="flex flex-col gap-0.5 font-normal">
              <span className="truncate text-sm font-semibold text-[var(--text)]">
                {user?.full_name || "User"}
              </span>
              <span className="truncate text-[11px] text-[var(--muted)]">{user?.email || "—"}</span>
              {(user?.role_name || user?.role_slug) && (
                <span className="truncate text-[11px] text-[var(--muted)]">
                  {user.role_name || user.role_slug}
                </span>
              )}
            </div>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onSelect={() => {
              setMenuOpen(false);
              onOpenProfile();
            }}
          >
            <IconUser className="size-4 opacity-80" />
            Profil
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            danger
            onSelect={() => {
              // Revoke refresh cookie di server (best-effort), lalu buang token lokal.
              void api("/api/auth/logout", { method: "POST" }, { skipAuthRefresh: true })
                .catch(() => undefined)
                .finally(() => {
                  clearToken();
                  onLogout();
                });
            }}
          >
            <IconLogout className="size-4 opacity-80" />
            Keluar
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  );
}
