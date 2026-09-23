import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiUpload } from "./api";
import { compressImageForUpload } from "./imageCompress";
import { toastError, toastSuccess } from "./swal";
import { Button, Input, SecretInput, Section } from "./ui";
import { UserAvatar, type MeUser } from "./UserMenu";

export function ProfilePage() {
  const qc = useQueryClient();
  const fileRef = useRef<HTMLInputElement>(null);
  const meQ = useQuery<MeUser>({ queryKey: ["me"], queryFn: () => api<MeUser>("/api/me") });
  const user = meQ.data;

  const [fullName, setFullName] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [avatarPreview, setAvatarPreview] = useState<string | null>(null);
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    if (!meQ.data || initialized) return;
    setFullName(meQ.data.full_name || "");
    setAvatarPreview(meQ.data.avatar_url || null);
    setInitialized(true);
  }, [meQ.data, initialized]);

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
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      await qc.invalidateQueries({ queryKey: ["me"] });
      void toastSuccess("Profil disimpan");
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
    <Section title="Profil">
      <p className="mb-4 text-sm text-[var(--muted)]">Ubah nama, password, dan foto avatar Anda.</p>
      {meQ.isLoading ? (
        <p className="text-sm text-[var(--muted)]">Memuat profil…</p>
      ) : meQ.isError ? (
        <p className="text-sm text-[var(--danger)]">Gagal memuat profil. Coba refresh halaman.</p>
      ) : (
        <form className="grid max-w-xl gap-4" onSubmit={saveProfile}>
          <div className="flex items-center gap-4">
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

          {(user?.role_name || user?.role_slug) && (
            <label className="grid gap-1.5 text-sm">
              <span className="font-medium text-[var(--muted)]">Role</span>
              <Input value={user.role_name || user.role_slug || ""} disabled readOnly />
            </label>
          )}

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

          <div>
            <Button type="submit" disabled={busy}>
              {busy ? "Menyimpan…" : "Simpan"}
            </Button>
          </div>
        </form>
      )}
    </Section>
  );
}
