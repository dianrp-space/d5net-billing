import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, ImageIcon, Users } from "lucide-react";
import { api, apiUpload, clearPlatformToken } from "./api";
import { applyBrandingMeta } from "./branding";
import { BackupRestorePage } from "./BackupRestorePage";
import { IconPencil, IconShield, IconTrash, IconUser } from "./icons";
import { useAppDialog } from "./confirm";
import { toastError, toastSuccess } from "./swal";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { FormDialog, IconButton, IconLink, Section, SecretInput, Table } from "./ui";
import { ThemeToggle } from "./ThemeToggle";

type Tenant = {
  id: string;
  slug: string;
  name: string;
  email?: string | null;
  phone?: string | null;
  is_active: boolean;
  created_at?: string;
};

type Branding = {
  app_name: string;
  logo_url?: string | null;
  favicon_url?: string | null;
};

const emptyCreate = {
  slug: "",
  name: "",
  email: "",
  password: "",
  fullName: "Admin",
};

type PlatformPage = "tenants" | "branding" | "backup";

export function PlatformApp({ onLogout }: { onLogout: () => void }) {
  const [page, setPage] = useState<PlatformPage>("tenants");
  const branding = useQuery({
    queryKey: ["platform-branding"],
    queryFn: () => api<Branding>("/api/platform/branding", {}, { platform: true }),
  });

  useEffect(() => {
    const b = branding.data;
    applyBrandingMeta({
      appName: b?.app_name || "drp-billing",
      faviconUrl: b?.favicon_url,
      titleSuffix: page === "branding" ? "Branding" : page === "backup" ? "Backup" : "Platform",
    });
  }, [branding.data, page]);

  const appName = branding.data?.app_name || "drp-billing";
  const pageTitle =
    page === "branding" ? "Branding" : page === "backup" ? "Backup / Restore" : "Tenant";

  return (
    <div className="mx-auto max-w-5xl p-6">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          {branding.data?.logo_url ? (
            <img src={branding.data.logo_url} alt="" className="h-10 w-10 rounded-lg object-contain" />
          ) : null}
          <div>
            <div className="text-sm text-[var(--muted)]">{appName}</div>
            <h1 className="text-2xl font-semibold">Platform · {pageTitle}</h1>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <ThemeToggle />
          <button
            type="button"
            className="btn-ghost"
            onClick={() => {
              clearPlatformToken();
              onLogout();
            }}
          >
            Keluar
          </button>
        </div>
      </div>

      <Tabs
        value={page}
        onValueChange={(v: string) => setPage(v as PlatformPage)}
        className="space-y-0"
      >
        <TabsList aria-label="Menu platform" className="mb-6">
          <TabsTrigger value="tenants">
            <Users />
            Tenant
          </TabsTrigger>
          <TabsTrigger value="branding">
            <ImageIcon />
            Branding
          </TabsTrigger>
          <TabsTrigger value="backup">
            <Download />
            Backup / Restore
          </TabsTrigger>
        </TabsList>
        <TabsContent value="tenants" className="mt-0">
          <PlatformTenants />
        </TabsContent>
        <TabsContent value="branding" className="mt-0">
          <PlatformBranding />
        </TabsContent>
        <TabsContent value="backup" className="mt-0">
          <BackupRestorePage platform />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function PlatformBranding() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["platform-branding"],
    queryFn: () => api<Branding>("/api/platform/branding", {}, { platform: true }),
  });
  const [appName, setAppName] = useState("");
  const [err, setErr] = useState("");

  useEffect(() => {
    if (q.data) setAppName(q.data.app_name || "drp-billing");
  }, [q.data]);

  const save = useMutation({
    mutationFn: () =>
      api<Branding>(
        "/api/platform/branding",
        { method: "PUT", body: JSON.stringify({ app_name: appName.trim() || "drp-billing" }) },
        { platform: true },
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["platform-branding"] });
      void toastSuccess("Branding platform disimpan");
      setErr("");
    },
    onError: (e: Error) => {
      setErr(e.message);
      void toastError(e.message);
    },
  });

  const clearField = useMutation({
    mutationFn: (field: "logo" | "favicon") =>
      api<Branding>(
        "/api/platform/branding",
        {
          method: "PUT",
          body: JSON.stringify({
            app_name: appName.trim() || "drp-billing",
            clear_logo: field === "logo",
            clear_favicon: field === "favicon",
          }),
        },
        { platform: true },
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["platform-branding"] });
      void toastSuccess("Dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  async function onUpload(kind: "logo" | "favicon", file: File | undefined) {
    if (!file) return;
    try {
      await apiUpload<{ url: string }>(`/api/platform/branding/${kind}`, file, { platform: true });
      void qc.invalidateQueries({ queryKey: ["platform-branding"] });
      void toastSuccess(kind === "logo" ? "Logo diunggah" : "Favicon diunggah");
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Upload gagal");
    }
  }

  const b = q.data;

  return (
    <Section title="Branding owner (fallback tenant)">
      {q.isLoading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <div className="grid max-w-xl gap-4">
          <label className="grid gap-1 text-sm">
            <span className="font-medium">Nama aplikasi</span>
            <input className="input" value={appName} onChange={(e) => setAppName(e.target.value)} />
            <span className="text-xs text-[var(--muted)]">
              Dipakai sebagai fallback jika tenant belum mengisi, dan sebagai prefix komentar RouterOS.
            </span>
          </label>
          <div className="rounded-xl border border-[var(--border)] p-3">
            <div className="mb-2 flex items-center justify-between">
              <span className="text-sm font-medium">Logo</span>
              {b?.logo_url ? (
                <button type="button" className="btn-ghost text-xs" onClick={() => clearField.mutate("logo")}>
                  Hapus
                </button>
              ) : null}
            </div>
            <div className="flex flex-wrap items-center gap-3">
              {b?.logo_url ? (
                <img src={b.logo_url} alt="" className="h-12 w-12 rounded-lg object-contain" />
              ) : (
                <div className="flex h-12 w-12 items-center justify-center rounded-lg border border-dashed text-xs text-[var(--muted)]">
                  —
                </div>
              )}
              <input
                type="file"
                accept="image/*,.ico,.svg"
                className="text-sm"
                onChange={(e) => {
                  void onUpload("logo", e.target.files?.[0]);
                  e.target.value = "";
                }}
              />
            </div>
          </div>
          <div className="rounded-xl border border-[var(--border)] p-3">
            <div className="mb-2 flex items-center justify-between">
              <span className="text-sm font-medium">Favicon</span>
              {b?.favicon_url ? (
                <button type="button" className="btn-ghost text-xs" onClick={() => clearField.mutate("favicon")}>
                  Hapus
                </button>
              ) : null}
            </div>
            <div className="flex flex-wrap items-center gap-3">
              {b?.favicon_url ? (
                <img src={b.favicon_url} alt="" className="h-12 w-12 rounded-lg object-contain" />
              ) : (
                <div className="flex h-12 w-12 items-center justify-center rounded-lg border border-dashed text-xs text-[var(--muted)]">
                  —
                </div>
              )}
              <input
                type="file"
                accept="image/*,.ico,.svg"
                className="text-sm"
                onChange={(e) => {
                  void onUpload("favicon", e.target.files?.[0]);
                  e.target.value = "";
                }}
              />
            </div>
          </div>
          <button type="button" className="btn w-fit" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? "Menyimpan..." : "Simpan"}
          </button>
          {err && <p className="text-sm text-[var(--danger)]">{err}</p>}
        </div>
      )}
    </Section>
  );
}

function PlatformTenants() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [form, setForm] = useState(emptyCreate);
  const [createOpen, setCreateOpen] = useState(false);
  const [edit, setEdit] = useState<Tenant | null>(null);
  const [err, setErr] = useState("");
  const [editErr, setEditErr] = useState("");

  const tenants = useQuery({
    queryKey: ["platform-tenants"],
    queryFn: () => api<{ data: Tenant[] }>("/api/platform/tenants", {}, { platform: true }),
  });

  const refresh = () => qc.invalidateQueries({ queryKey: ["platform-tenants"] });

  const create = useMutation({
    mutationFn: () =>
      api(
        "/api/platform/tenants",
        {
          method: "POST",
          body: JSON.stringify({
            slug: form.slug.trim().toLowerCase(),
            name: form.name.trim(),
            email: form.email.trim(),
            password: form.password,
            full_name: form.fullName.trim(),
          }),
        },
        { platform: true },
      ),
    onSuccess: () => {
      setForm(emptyCreate);
      setErr("");
      setCreateOpen(false);
      refresh();
      void toastSuccess("Tenant ditambahkan");
    },
    onError: (e: Error) => {
      setErr(e.message);
      void toastError(e.message);
    },
  });

  const update = useMutation({
    mutationFn: (t: Tenant) =>
      api(
        `/api/platform/tenants/${t.id}`,
        {
          method: "PUT",
          body: JSON.stringify({
            name: t.name.trim(),
            email: t.email?.trim() || null,
            phone: t.phone?.trim() || null,
            is_active: t.is_active,
          }),
        },
        { platform: true },
      ),
    onSuccess: () => {
      setEdit(null);
      setEditErr("");
      refresh();
      void toastSuccess("Tenant diperbarui");
    },
    onError: (e: Error) => {
      setEditErr(e.message);
      void toastError(e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/platform/tenants/${id}`, { method: "DELETE" }, { platform: true }),
    onSuccess: () => {
      if (edit?.id === remove.variables) setEdit(null);
      refresh();
      void toastSuccess("Tenant dihapus");
    },
    onError: (e: Error) => {
      setEditErr(e.message);
      void toastError(e.message);
    },
  });

  const rows = (tenants.data?.data || []).map((t) => [
    t.slug,
    t.name,
    t.email || "—",
    t.phone || "—",
    t.is_active ? "aktif" : "nonaktif",
    <span key={t.id} className="flex flex-wrap items-center gap-1.5">
      <IconButton
        label="Edit tenant"
        onClick={() => {
          setEdit({ ...t });
          setEditErr("");
        }}
      >
        <IconPencil />
      </IconButton>
      <IconLink label="Login admin" href={`/admin/${t.slug}/login`}>
        <IconShield />
      </IconLink>
      <IconLink label="Portal pelanggan" href={`/client/${t.slug}/login`}>
        <IconUser />
      </IconLink>
      <IconButton
        label="Hapus tenant"
        danger
        disabled={remove.isPending}
        onClick={async () => {
          const ok = await confirm({
            title: "Hapus tenant",
            description: `Hapus tenant "${t.slug}" beserta semua datanya? Tindakan ini tidak bisa dibatalkan.`,
            confirmLabel: "Hapus permanen",
          });
          if (!ok) return;
          remove.mutate(t.id);
        }}
      >
        <IconTrash />
      </IconButton>
    </span>,
  ]);

  return (
    <>
      <Section
        title="Daftar tenant"
        actions={
          <button
            type="button"
            className="btn"
            onClick={() => {
              setErr("");
              setForm(emptyCreate);
              setCreateOpen(true);
            }}
          >
            + Tambah
          </button>
        }
      >
        {tenants.isLoading ? (
          <p className="text-[var(--muted)]">Memuat...</p>
        ) : (
          <Table columns={["Slug", "Nama", "Email", "Telepon", "Status", "Aksi"]} rows={rows} />
        )}
      </Section>

      <FormDialog
        open={createOpen}
        wide
        title="Buat tenant"
        onClose={() => {
          setCreateOpen(false);
          setErr("");
        }}
      >
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <input
            className="input"
            placeholder="slug (contoh: demo)"
            value={form.slug}
            onChange={(e) => setForm({ ...form, slug: e.target.value })}
            required
            pattern="[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?"
            title="huruf kecil, angka, hyphen"
          />
          <input
            className="input"
            placeholder="Nama ISP"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            required
          />
          <input
            className="input"
            type="email"
            placeholder="Email admin"
            value={form.email}
            onChange={(e) => setForm({ ...form, email: e.target.value })}
            required
          />
          <input
            className="input"
            placeholder="Nama lengkap admin"
            value={form.fullName}
            onChange={(e) => setForm({ ...form, fullName: e.target.value })}
            required
          />
          <SecretInput
            className="sm:col-span-2"
            placeholder="Password admin (min 8)"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            required
            minLength={8}
            autoComplete="new-password"
          />
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <button className="btn" disabled={create.isPending}>
              {create.isPending ? "Membuat..." : "Simpan"}
            </button>
            <button type="button" className="btn-ghost" onClick={() => setCreateOpen(false)}>
              Batal
            </button>
          </div>
          {err && <p className="text-sm text-[var(--danger)] sm:col-span-2">{err}</p>}
        </form>
      </FormDialog>

      <FormDialog open={Boolean(edit)} wide title={edit ? `Edit tenant · ${edit.slug}` : "Edit tenant"} onClose={() => setEdit(null)}>
        {edit && (
          <form
            className="grid gap-3 sm:grid-cols-2"
            onSubmit={(e) => {
              e.preventDefault();
              update.mutate(edit);
            }}
          >
            <input className="input bg-[var(--panel-muted)] text-[var(--muted)]" value={edit.slug} disabled title="Slug tidak bisa diubah" />
            <input
              className="input"
              placeholder="Nama ISP"
              value={edit.name}
              onChange={(e) => setEdit({ ...edit, name: e.target.value })}
              required
            />
            <input
              className="input"
              type="email"
              placeholder="Email kontak"
              value={edit.email || ""}
              onChange={(e) => setEdit({ ...edit, email: e.target.value })}
            />
            <input
              className="input"
              placeholder="Telepon"
              value={edit.phone || ""}
              onChange={(e) => setEdit({ ...edit, phone: e.target.value })}
            />
            <label className="flex items-center gap-2 text-sm text-[var(--text-body)] sm:col-span-2">
              <input
                type="checkbox"
                checked={edit.is_active}
                onChange={(e) => setEdit({ ...edit, is_active: e.target.checked })}
              />
              Tenant aktif
            </label>
            <div className="flex flex-wrap gap-2 sm:col-span-2">
              <button className="btn" disabled={update.isPending}>
                {update.isPending ? "Menyimpan..." : "Simpan"}
              </button>
              <button type="button" className="btn-ghost" onClick={() => setEdit(null)}>
                Batal
              </button>
            </div>
            {editErr && <p className="text-sm text-[var(--danger)] sm:col-span-2">{editErr}</p>}
          </form>
        )}
      </FormDialog>
    </>
  );
}
