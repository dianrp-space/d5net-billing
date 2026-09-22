import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, apiUpload, clearToken } from "./api";
import { compressImageForUpload } from "./imageCompress";
import { IconLogout, IconUser } from "./icons";
import { toastError, toastSuccess } from "./swal";
import { Button, Input, SecretInput } from "./ui";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
}: {
  user?: MeUser | null;
  onLogout: () => void;
}) {
  const qc = useQueryClient();
  const fileRef = useRef<HTMLInputElement>(null);
  const [profileOpen, setProfileOpen] = useState(false);
  const [fullName, setFullName] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [avatarPreview, setAvatarPreview] = useState<string | null>(null);

  useEffect(() => {
    if (!profileOpen) return;
    setFullName(user?.full_name || "");
    setCurrentPassword("");
    setNewPassword("");
    setConfirmPassword("");
    setAvatarPreview(user?.avatar_url || null);
  }, [profileOpen, user?.full_name, user?.avatar_url]);

  async function saveProfile(e: React.FormEvent) {
    e.preventDefault();
    const name = fullName.trim();
    if (!name) {
      void toastError("Nama wajib diisi");
      return;
    }
    const changingPw = newPassword.trim().length > 0 || confirmPassword.trim().length > 0;
    if (changingPw) {
      if (newPassword.trim().length < 8) {
        void toastError("Password baru minimal 8 karakter");
        return;
      }
      if (newPassword !== confirmPassword) {
        void toastError("Konfirmasi password tidak cocok");
        return;
      }
      if (!currentPassword) {
        void toastError("Isi password saat ini untuk mengganti password");
        return;
      }
    }
    setBusy(true);
    try {
      await api("/api/me", {
        method: "PUT",
        body: JSON.stringify({
          full_name: name,
          ...(changingPw
            ? { current_password: currentPassword, new_password: newPassword.trim() }
            : {}),
        }),
      });
      await qc.invalidateQueries({ queryKey: ["me"] });
      void toastSuccess("Profil disimpan");
      setProfileOpen(false);
    } catch (err: unknown) {
      void toastError(err instanceof Error ? err.message : "Gagal menyimpan profil");
    } finally {
      setBusy(false);
    }
  }

  async function onPickAvatar(file: File | undefined) {
    if (!file) return;
    setBusy(true);
    try {
      const compressed = await compressImageForUpload(file);
      const res = await apiUpload<{ url: string }>("/api/me/avatar", compressed);
      setAvatarPreview(res.url);
      await qc.invalidateQueries({ queryKey: ["me"] });
      void toastSuccess("Avatar diperbarui");
    } catch (err: unknown) {
      void toastError(err instanceof Error ? err.message : "Gagal unggah avatar");
    } finally {
      setBusy(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  async function clearAvatar() {
    setBusy(true);
    try {
      await api("/api/me", {
        method: "PUT",
        body: JSON.stringify({
          full_name: (fullName.trim() || user?.full_name || "").trim() || "User",
          clear_avatar: true,
        }),
      });
      setAvatarPreview(null);
      await qc.invalidateQueries({ queryKey: ["me"] });
      void toastSuccess("Foto avatar dihapus");
    } catch (err: unknown) {
      void toastError(err instanceof Error ? err.message : "Gagal menghapus avatar");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <DropdownMenu>
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
              setProfileOpen(true);
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

      <Dialog open={profileOpen} onOpenChange={setProfileOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Profil</DialogTitle>
            <DialogDescription>Ubah nama, password, dan foto avatar Anda.</DialogDescription>
          </DialogHeader>
          <form className="grid gap-4" onSubmit={saveProfile}>
            <div className="flex items-center gap-3">
              <UserAvatar
                name={fullName || user?.full_name}
                email={user?.email}
                avatarUrl={avatarPreview}
                className="app-user-avatar--lg"
              />
              <div className="min-w-0 flex-1 space-y-2">
                <input
                  ref={fileRef}
                  type="file"
                  accept="image/jpeg,image/png,image/webp,image/gif,.jpg,.jpeg,.png,.webp"
                  className="hidden"
                  onChange={(e) => void onPickAvatar(e.target.files?.[0])}
                />
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    disabled={busy}
                    onClick={() => fileRef.current?.click()}
                  >
                    Ganti foto
                  </Button>
                  {avatarPreview ? (
                    <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => void clearAvatar()}>
                      Hapus foto
                    </Button>
                  ) : null}
                </div>
                <p className="text-[11px] text-[var(--muted)]">JPG/PNG/WebP, dikompres otomatis.</p>
              </div>
            </div>

            <label className="grid gap-1.5 text-sm">
              <span className="font-medium">Nama</span>
              <Input
                value={fullName}
                onChange={(e) => setFullName(e.target.value)}
                required
                autoComplete="name"
                disabled={busy}
              />
            </label>

            <label className="grid gap-1.5 text-sm">
              <span className="font-medium text-[var(--muted)]">Email</span>
              <Input value={user?.email || ""} disabled readOnly />
            </label>

            <div className="grid gap-3 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)] p-3">
              <p className="text-xs font-semibold text-[var(--muted)]">Ganti password (opsional)</p>
              <SecretInput
                placeholder="Password saat ini"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                autoComplete="current-password"
                disabled={busy}
              />
              <SecretInput
                placeholder="Password baru (min 8)"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                autoComplete="new-password"
                minLength={8}
                disabled={busy}
              />
              <SecretInput
                placeholder="Ulangi password baru"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                autoComplete="new-password"
                minLength={8}
                disabled={busy}
              />
            </div>

            <DialogFooter>
              <Button type="button" variant="secondary" disabled={busy} onClick={() => setProfileOpen(false)}>
                Batal
              </Button>
              <Button type="submit" disabled={busy}>
                {busy ? "Menyimpan…" : "Simpan"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
