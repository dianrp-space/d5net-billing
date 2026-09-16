import { Fragment, useEffect, useRef, useState, type DragEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Globe, UserRound } from "lucide-react";
import { api, apiUpload } from "./api";
import { IconPencil, IconTrash, IconUpload } from "./icons";
import { useAppDialog } from "./confirm";
import { toastError, toastSuccess } from "./swal";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { formatRp, FormDialog, IconButton, Section, SecretInput, Table } from "./ui";
import { EXAMPLE_ICONS, exampleIconDataUrl, exampleIconFile, type ExampleIcon } from "./exampleIcons";
import { DEFAULT_PRIMARY, parseHexColor, setTenantPrimaryColor } from "./theme";
import { cn } from "./lib/utils";
import { usePersistedTab } from "./navPersist";

type Branding = {
  app_name: string;
  logo_url?: string | null;
  favicon_url?: string | null;
  map_pop_icon_url?: string | null;
  map_odp_icon_url?: string | null;
  map_customer_icon_url?: string | null;
};

type BrandingField = "logo" | "favicon" | "map-pop" | "map-odp" | "map-customer";

const CLEAR_FLAG: Record<BrandingField, string> = {
  logo: "clear_logo",
  favicon: "clear_favicon",
  "map-pop": "clear_map_pop_icon",
  "map-odp": "clear_map_odp_icon",
  "map-customer": "clear_map_customer_icon",
};

type TenantBrandingView = {
  overrides: Branding;
  effective: Branding;
  from_owner: {
    app_name: boolean;
    logo_url: boolean;
    favicon_url: boolean;
    map_pop_icon_url: boolean;
    map_odp_icon_url: boolean;
    map_customer_icon_url: boolean;
  };
};

type Role = {
  id: string;
  name: string;
  slug: string;
  permissions: string[];
  is_system: boolean;
};

type TenantUser = {
  user_id: string;
  email: string;
  full_name: string;
  phone?: string | null;
  is_active: boolean;
  role_id: string;
  role_slug: string;
  role_name: string;
};

type PortalUser = {
  id: string;
  customer_code: string;
  full_name: string;
  phone: string;
  portal_enabled: boolean;
  has_password: boolean;
  is_active: boolean;
};

function clampIsolirGrace(n: number | undefined) {
  const v = Math.floor(Number(n) || 0);
  if (!Number.isFinite(v) || v < 0) return 0;
  return Math.min(30, v);
}

function clampCycleStartDay(n: number | undefined) {
  const v = Math.floor(Number(n) || 1);
  if (!Number.isFinite(v) || v < 1) return 1;
  return Math.min(28, v);
}

function clampDueDay(n: number | undefined) {
  const v = Math.floor(Number(n) || 10);
  if (!Number.isFinite(v) || v < 1) return 10;
  return Math.min(28, v);
}

function clampLateFeePercent(n: number | undefined) {
  if (n === undefined || n === null) return 5;
  const v = Number(n);
  if (!Number.isFinite(v) || v < 0) return 0;
  return Math.min(100, v);
}

const PERM_PRESETS: { key: string; label: string; hint: string }[] = [
  { key: "*", label: "Semua akses (*)", hint: "Akses penuh ke semua menu" },
  { key: "dashboard", label: "Dashboard", hint: "Halaman dashboard" },
  { key: "customers", label: "Pelanggan & Lead", hint: "Pelanggan, Diskon, Lead, Reseller, Coverage" },
  { key: "leads", label: "Lead saja", hint: "Lead + Coverage peta jangkauan" },
  { key: "billing", label: "Billing", hint: "Paket, Diskon, Tagihan, Pembayaran, Akunting" },
  { key: "network", label: "Jaringan", hint: "Cluster, Router, IP Pool, ODP, Coverage, Voucher (+ Secrets lewat Pelanggan)" },
  { key: "ops", label: "Operasional", hint: "Menu Tiket (instalasi & support)" },
  { key: "tickets", label: "Tiket saja", hint: "Hanya menu Tiket" },
  { key: "sla-report", label: "Laporan SLA", hint: "Evaluasi SLA & waktu resolve tiket" },
  { key: "settings", label: "Pengaturan", hint: "Umum, Isolir, Cronjob, Notifikasi, Roles, Users, Backup, Integrasi" },
];

export function GeneralSettingsPage() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["settings-branding"],
    queryFn: () =>
      api<
        TenantBrandingView & {
          tenant_name: string;
          timezone: string;
          default_tax_percent: number;
          isolir_grace_days?: number;
          billing_cycle_start_day?: number;
          invoice_due_day?: number;
          late_fee_percent?: number;
          primary_color?: string;
          wallet_enabled?: boolean;
          wallet_min_topup?: number;
          about?: string;
          product_description?: string;
          support_email?: string;
          support_phone?: string;
          support_address?: string;
        }
      >("/api/settings/branding"),
  });
  const [tenantName, setTenantName] = useState("");
  const [timezone, setTimezone] = useState("Asia/Jakarta");
  const [taxPercent, setTaxPercent] = useState(0);
  const [isolirGraceDays, setIsolirGraceDays] = useState(0);
  const [cycleStartDay, setCycleStartDay] = useState(1);
  const [dueDay, setDueDay] = useState(10);
  const [lateFeePercent, setLateFeePercent] = useState(5);
  const [primaryColor, setPrimaryColor] = useState(DEFAULT_PRIMARY);
  const [walletEnabled, setWalletEnabled] = useState(false);
  const [walletMinTopup, setWalletMinTopup] = useState(10000);
  const [about, setAbout] = useState("");
  const [productDescription, setProductDescription] = useState("");
  const [supportEmail, setSupportEmail] = useState("");
  const [supportPhone, setSupportPhone] = useState("");
  const [supportAddress, setSupportAddress] = useState("");
  const [err, setErr] = useState("");

  useEffect(() => {
    if (!q.data) return;
    setTenantName(q.data.tenant_name || q.data.overrides.app_name || "");
    setTimezone(q.data.timezone || "Asia/Jakarta");
    setTaxPercent(Number(q.data.default_tax_percent) || 0);
    setIsolirGraceDays(clampIsolirGrace(q.data.isolir_grace_days));
    setCycleStartDay(clampCycleStartDay(q.data.billing_cycle_start_day));
    setDueDay(clampDueDay(q.data.invoice_due_day));
    setLateFeePercent(clampLateFeePercent(q.data.late_fee_percent));
    setPrimaryColor(parseHexColor(q.data.primary_color) || DEFAULT_PRIMARY);
    setWalletEnabled(Boolean(q.data.wallet_enabled));
    setWalletMinTopup(Number(q.data.wallet_min_topup) || 10000);
    setAbout(q.data.about || "");
    setProductDescription(q.data.product_description || "");
    setSupportEmail(q.data.support_email || "");
    setSupportPhone(q.data.support_phone || "");
    setSupportAddress(q.data.support_address || "");
  }, [q.data]);

  function brandingBody(extra?: Record<string, unknown>) {
    return {
      tenant_name: tenantName.trim(),
      timezone: timezone.trim() || "Asia/Jakarta",
      default_tax_percent: Math.max(0, Number(taxPercent) || 0),
      isolir_grace_days: clampIsolirGrace(isolirGraceDays),
      billing_cycle_start_day: clampCycleStartDay(cycleStartDay),
      invoice_due_day: clampDueDay(dueDay),
      late_fee_percent: clampLateFeePercent(lateFeePercent),
      primary_color: parseHexColor(primaryColor) === DEFAULT_PRIMARY ? "" : primaryColor,
      wallet_enabled: walletEnabled,
      wallet_min_topup: Math.max(1000, Math.floor(Number(walletMinTopup) || 10000)),
      about: about.trim(),
      product_description: productDescription.trim(),
      support_email: supportEmail.trim(),
      support_phone: supportPhone.trim(),
      support_address: supportAddress.trim(),
      ...extra,
    };
  }

  function onPrimaryChange(next: string) {
    const parsed = parseHexColor(next) || primaryColor;
    setPrimaryColor(parsed);
    setTenantPrimaryColor(parsed);
  }

  const save = useMutation({
    mutationFn: () =>
      api("/api/settings/branding", {
        method: "PUT",
        body: JSON.stringify(brandingBody()),
      }),
    onSuccess: () => {
      const parsed = parseHexColor(primaryColor);
      setTenantPrimaryColor(parsed && parsed !== DEFAULT_PRIMARY ? parsed : null);
      void qc.invalidateQueries({ queryKey: ["settings-branding"] });
      void qc.invalidateQueries({ queryKey: ["settings-invoice"] });
      void qc.invalidateQueries({ queryKey: ["public-branding"] });
      void qc.invalidateQueries({ queryKey: ["jobs-settings"] });
      void toastSuccess("Pengaturan umum disimpan");
      setErr("");
    },
    onError: (e: Error) => {
      setErr(e.message);
      void toastError(e.message);
    },
  });

  const clearField = useMutation({
    mutationFn: (field: BrandingField) =>
      api("/api/settings/branding", {
        method: "PUT",
        body: JSON.stringify(brandingBody({ [CLEAR_FLAG[field]]: true })),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["settings-branding"] });
      void qc.invalidateQueries({ queryKey: ["settings-invoice"] });
      void qc.invalidateQueries({ queryKey: ["public-branding"] });
      void toastSuccess("Mengikuti bawaan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  async function onUpload(kind: BrandingField, file: File | undefined) {
    if (!file) return;
    try {
      await apiUpload<{ url: string }>(`/api/settings/branding/${kind}`, file);
      void qc.invalidateQueries({ queryKey: ["settings-branding"] });
      void qc.invalidateQueries({ queryKey: ["settings-invoice"] });
      void qc.invalidateQueries({ queryKey: ["public-branding"] });
      void toastSuccess(
        kind === "logo" ? "Logo diunggah" : kind === "favicon" ? "Favicon diunggah" : "Icon peta diunggah",
      );
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Upload gagal");
    }
  }

  const view = q.data;
  const eff = view?.effective;
  const from = view?.from_owner;

  const TIMEZONES = [
    "Asia/Jakarta",
    "Asia/Makassar",
    "Asia/Jayapura",
    "Asia/Singapore",
    "UTC",
  ];

  return (
    <Section title="Umum">
      {q.isLoading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <div className="grid gap-6">
          <div className="panel-card grid gap-4 p-4 sm:grid-cols-2">
            <label className="grid gap-1 text-sm">
              <span className="font-medium">Nama provider</span>
              <input
                className="input"
                value={tenantName}
                onChange={(e) => setTenantName(e.target.value)}
                placeholder="Nama perusahaan / ISP"
                required
              />
              <span className="text-xs text-[var(--muted)]">Ditampilkan di sidebar, login, dan dokumen.</span>
            </label>

            <label className="grid gap-1 text-sm">
              <span className="font-medium">Timezone</span>
              <select className="input" value={timezone} onChange={(e) => setTimezone(e.target.value)}>
                {TIMEZONES.map((tz) => (
                  <option key={tz} value={tz}>
                    {tz}
                  </option>
                ))}
              </select>
              <span className="text-xs text-[var(--muted)]">
                Dipakai untuk jadwal worker / referensi waktu lokal (server tetap memakai zona proses).
              </span>
            </label>

            <label className="grid gap-1 text-sm">
              <span className="font-medium">Pajak default (%)</span>
              <input
                className="input"
                type="number"
                min={0}
                max={100}
                step={0.01}
                value={taxPercent}
                onChange={(e) => setTaxPercent(Number(e.target.value))}
              />
              <span className="text-xs text-[var(--muted)]">
                Diterapkan ke semua tagihan & ganti paket (bukan per paket).
              </span>
            </label>

            <label className="grid gap-1 text-sm">
              <span className="font-medium">Masa tenggang isolir (hari)</span>
              <input
                className="input"
                type="number"
                min={0}
                max={30}
                step={1}
                value={isolirGraceDays}
                onChange={(e) => setIsolirGraceDays(clampIsolirGrace(Number(e.target.value)))}
              />
              <span className="text-xs text-[var(--muted)]">
                Hari setelah jatuh tempo invoice sebelum auto-isolir. 0 = isolir pada tanggal jatuh tempo. Maks. 30.
                Bisa diubah juga di Cronjob.
              </span>
            </label>

            <label className="grid gap-1 text-sm">
              <span className="font-medium">Awal siklus tagihan (tanggal)</span>
              <input
                className="input"
                type="number"
                min={1}
                max={28}
                step={1}
                value={cycleStartDay}
                onChange={(e) => setCycleStartDay(clampCycleStartDay(Number(e.target.value)))}
              />
              <span className="text-xs text-[var(--muted)]">
                Hari kalender (1–28) yang jadi jangkar prorata. Aktivasi baru default ke tanggal ini (bisa diubah
                manual per secret). Contoh: 1 = awal bulan.
              </span>
            </label>

            <label className="grid gap-1 text-sm">
              <span className="font-medium">Tanggal jatuh tempo invoice (tanggal)</span>
              <input
                className="input"
                type="number"
                min={1}
                max={28}
                step={1}
                value={dueDay}
                onChange={(e) => setDueDay(clampDueDay(Number(e.target.value)))}
              />
              <span className="text-xs text-[var(--muted)]">
                Tanggal kalender (1–28) jatuh tempo semua invoice. Paket / harga per cluster bisa override. Contoh: 10 =
                jatuh tempo tiap tanggal 10.
              </span>
            </label>

            <label className="grid gap-1 text-sm">
              <span className="font-medium">Denda keterlambatan (%)</span>
              <input
                className="input"
                type="number"
                min={0}
                max={100}
                step={0.5}
                value={lateFeePercent}
                onChange={(e) => setLateFeePercent(clampLateFeePercent(Number(e.target.value)))}
              />
              <span className="text-xs text-[var(--muted)]">
                Persen dari total tunggakan, otomatis ditambahkan sebagai item saat tagihan baru terbit bila pelanggan
                punya tunggakan. 0 = nonaktif. Berlaku untuk tagihan berikutnya, bukan yang sudah terbit.
              </span>
            </label>

            <div className="grid gap-2 rounded-[var(--radius-lg)] border border-[var(--border)] p-3 text-sm">
              <label className="flex flex-wrap items-center justify-between gap-3">
                <span className="flex items-center gap-2 font-medium">
                  <input
                    type="checkbox"
                    checked={walletEnabled}
                    onChange={(e) => setWalletEnabled(e.target.checked)}
                  />
                  Aktifkan saldo &amp; auto-pay
                </span>
                <span className="flex items-center gap-2">
                  <span className="text-[var(--muted)]">Minimal topup (Rp)</span>
                  <input
                    className="input w-32"
                    type="number"
                    min={1000}
                    step={1000}
                    value={walletMinTopup}
                    onChange={(e) => setWalletMinTopup(Math.max(0, Number(e.target.value) || 0))}
                  />
                </span>
              </label>
              <span className="text-xs text-[var(--muted)]">
                Pelanggan bisa topup saldo dari portal. Tagihan langganan otomatis dibayar dari saldo saat terbit bila
                cukup; bila saldo kurang dikirim notifikasi WhatsApp. Tagihan manual dibayar oleh pelanggan (pilih saldo
                atau payment gateway).
              </span>
            </div>

            <div className="grid gap-2">
              <span className="text-sm font-medium">Warna primary</span>
              <div className="flex flex-wrap items-center gap-2">
                <input
                  type="color"
                  className="h-9 w-12 cursor-pointer rounded-md border border-[var(--border)] bg-[var(--panel)] p-0.5"
                  value={parseHexColor(primaryColor) || DEFAULT_PRIMARY}
                  onChange={(e) => onPrimaryChange(e.target.value)}
                  title="Pilih warna tombol"
                  aria-label="Pilih warna primary"
                />
                <input
                  className="input w-32 font-mono uppercase"
                  value={primaryColor}
                  onChange={(e) => onPrimaryChange(e.target.value)}
                  spellCheck={false}
                  aria-label="Kode hex warna primary"
                />
                <button
                  type="button"
                  className="btn-ghost"
                  onClick={() => {
                    setPrimaryColor(DEFAULT_PRIMARY);
                    setTenantPrimaryColor(null);
                  }}
                >
                  Default
                </button>
                <button type="button" className="btn">
                  Contoh tombol
                </button>
              </div>
              <span className="text-xs text-[var(--muted)]">
                Dipakai untuk tombol, tautan, dan aksen UI (termasuk portal pelanggan).
              </span>
            </div>

            <div className="flex flex-wrap items-center gap-2 sm:col-span-2">
              <button type="button" className="btn" disabled={save.isPending} onClick={() => save.mutate()}>
                {save.isPending ? "Menyimpan..." : "Simpan"}
              </button>
              {err ? <p className="text-sm text-[var(--danger)]">{err}</p> : null}
            </div>
          </div>

          <div className="grid gap-3">
            <p className="text-sm font-medium">Identitas visual</p>
            <div className="grid gap-3 sm:grid-cols-2">
              <BrandAssetField
                label="Logo"
                hint="PNG atau SVG transparan. Tampil di sidebar, login, dan dokumen."
                url={eff?.logo_url}
                fromOwner={Boolean(from?.logo_url)}
                onFile={(f) => void onUpload("logo", f)}
                onClear={() => clearField.mutate("logo")}
              />
              <BrandAssetField
                label="Favicon"
                hint="Ikon tab browser. ICO, PNG, atau SVG."
                url={eff?.favicon_url}
                fromOwner={Boolean(from?.favicon_url)}
                onFile={(f) => void onUpload("favicon", f)}
                onClear={() => clearField.mutate("favicon")}
              />
            </div>
          </div>

          <div className="grid gap-3">
            <p className="text-sm font-medium">Ikon peta FTTH</p>
            <div className="grid gap-3 sm:grid-cols-3">
              <BrandAssetField
                label="POP"
                hint="Marker menara / cluster di peta. Kosongkan untuk icon bawaan."
                url={eff?.map_pop_icon_url}
                fromOwner={Boolean(from?.map_pop_icon_url)}
                examples={EXAMPLE_ICONS.filter((e) => e.kind === "pop")}
                onFile={(f) => void onUpload("map-pop", f)}
                onClear={() => clearField.mutate("map-pop")}
              />
              <BrandAssetField
                label="ODP"
                hint="Marker kotak distribusi di peta. Kosongkan untuk icon bawaan."
                url={eff?.map_odp_icon_url}
                fromOwner={Boolean(from?.map_odp_icon_url)}
                examples={EXAMPLE_ICONS.filter((e) => e.kind === "odp")}
                onFile={(f) => void onUpload("map-odp", f)}
                onClear={() => clearField.mutate("map-odp")}
              />
              <BrandAssetField
                label="Pelanggan"
                hint="Marker rumah pelanggan di peta. Kosongkan untuk icon bawaan."
                url={eff?.map_customer_icon_url}
                fromOwner={Boolean(from?.map_customer_icon_url)}
                examples={EXAMPLE_ICONS.filter((e) => e.kind === "customer")}
                onFile={(f) => void onUpload("map-customer", f)}
                onClear={() => clearField.mutate("map-customer")}
              />
            </div>
          </div>

          <div className="grid gap-3">
            <div>
              <p className="text-sm font-medium">Profil publik (website)</p>
              <p className="text-xs text-[var(--muted)]">
                Tampil di halaman depan untuk pengunjung — juga dipakai untuk verifikasi payment gateway. Daftar
                produk &amp; harga diambil otomatis dari menu Paket (yang ditandai tampil di portal).
              </p>
            </div>
            <div className="panel-card grid gap-4 p-4 sm:grid-cols-2">
              <label className="grid gap-1 text-sm sm:col-span-2">
                <span className="font-medium">Deskripsi usaha</span>
                <textarea
                  className="input min-h-[90px]"
                  value={about}
                  onChange={(e) => setAbout(e.target.value)}
                  placeholder="Contoh: PT Delima Net adalah penyedia jasa layanan internet (ISP) untuk rumah dan bisnis di wilayah ..."
                />
              </label>
              <label className="grid gap-1 text-sm sm:col-span-2">
                <span className="font-medium">Deskripsi produk / layanan</span>
                <textarea
                  className="input min-h-[90px]"
                  value={productDescription}
                  onChange={(e) => setProductDescription(e.target.value)}
                  placeholder="Contoh: Layanan internet unlimited bulanan. Daftar paket dan harga (Rupiah) tampil otomatis dari menu Paket."
                />
              </label>
              <label className="grid gap-1 text-sm">
                <span className="font-medium">Email dukungan</span>
                <input
                  className="input"
                  type="email"
                  value={supportEmail}
                  onChange={(e) => setSupportEmail(e.target.value)}
                  placeholder="support@example.com"
                />
              </label>
              <label className="grid gap-1 text-sm">
                <span className="font-medium">Nomor telepon</span>
                <input
                  className="input"
                  value={supportPhone}
                  onChange={(e) => setSupportPhone(e.target.value)}
                  placeholder="021-1234567 atau 0812-3456-7890"
                />
              </label>
              <label className="grid gap-1 text-sm sm:col-span-2">
                <span className="font-medium">Alamat usaha</span>
                <textarea
                  className="input min-h-[70px]"
                  value={supportAddress}
                  onChange={(e) => setSupportAddress(e.target.value)}
                  placeholder="Alamat lengkap kantor / usaha"
                />
              </label>
              <div className="sm:col-span-2">
                <button type="button" className="btn" disabled={save.isPending} onClick={() => save.mutate()}>
                  {save.isPending ? "Menyimpan..." : "Simpan profil publik"}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </Section>
  );
}

type InvoiceSettingsData = {
  company_name: string;
  address: string;
  phone: string;
  email: string;
  website: string;
  tax_id: string;
  payment_instructions: string;
  footer_note: string;
};

const EMPTY_INVOICE_SETTINGS: InvoiceSettingsData = {
  company_name: "",
  address: "",
  phone: "",
  email: "",
  website: "",
  tax_id: "",
  payment_instructions: "",
  footer_note: "",
};

export function InvoiceSettingsPage() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["settings-invoice"],
    queryFn: () =>
      api<{ settings: InvoiceSettingsData; default_company: string; logo_url?: string | null; tenant_slug?: string }>(
        "/api/settings/invoice",
      ),
  });
  const [form, setForm] = useState<InvoiceSettingsData>(EMPTY_INVOICE_SETTINGS);
  const [err, setErr] = useState("");

  useEffect(() => {
    if (!q.data) return;
    setForm({ ...EMPTY_INVOICE_SETTINGS, ...q.data.settings });
  }, [q.data]);

  const set = (k: keyof InvoiceSettingsData) => (e: { target: { value: string } }) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  const save = useMutation({
    mutationFn: () =>
      api("/api/settings/invoice", { method: "PUT", body: JSON.stringify(form) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["settings-invoice"] });
      void toastSuccess("Format invoice disimpan");
      setErr("");
    },
    onError: (e: Error) => {
      setErr(e.message);
      void toastError(e.message);
    },
  });

  const defaultCompany = q.data?.default_company || "";
  const logoUrl = q.data?.logo_url || "";

  return (
    <Section title="Format Invoice">
      {q.isLoading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <div className="grid gap-6">
          <p className="text-sm text-[var(--muted)]">
            Data ini tampil di dokumen invoice PDF (header perusahaan, cara pembayaran, dan catatan kaki). Bisa
            diunduh admin dari menu Tagihan maupun pelanggan dari portal.
          </p>

          <div className="grid gap-6 xl:grid-cols-2 xl:items-start">
          <div className="panel-card grid gap-4 p-4 sm:grid-cols-2">
            <label className="grid gap-1 text-sm sm:col-span-2">
              <span className="font-medium">Nama perusahaan</span>
              <input
                className="input"
                value={form.company_name}
                onChange={set("company_name")}
                placeholder={defaultCompany || "Nama ISP / perusahaan"}
              />
              <span className="text-xs text-[var(--muted)]">
                Kosongkan untuk memakai nama provider{defaultCompany ? ` (${defaultCompany})` : ""}. Logo
                memakai unggahan di Pengaturan → Umum.
              </span>
            </label>

            <label className="grid gap-1 text-sm sm:col-span-2">
              <span className="font-medium">Alamat</span>
              <textarea
                className="input min-h-[70px]"
                value={form.address}
                onChange={set("address")}
                placeholder="Alamat kantor (boleh beberapa baris)"
              />
            </label>

            <label className="grid gap-1 text-sm">
              <span className="font-medium">Telepon</span>
              <input className="input" value={form.phone} onChange={set("phone")} placeholder="0812xxxx" />
            </label>

            <label className="grid gap-1 text-sm">
              <span className="font-medium">Email</span>
              <input className="input" value={form.email} onChange={set("email")} placeholder="billing@isp.id" />
            </label>

            <label className="grid gap-1 text-sm">
              <span className="font-medium">Website</span>
              <input className="input" value={form.website} onChange={set("website")} placeholder="www.isp.id" />
            </label>

            <label className="grid gap-1 text-sm">
              <span className="font-medium">NPWP / Tax ID</span>
              <input className="input" value={form.tax_id} onChange={set("tax_id")} placeholder="00.000.000.0-000.000" />
            </label>

            <label className="grid gap-1 text-sm sm:col-span-2">
              <span className="font-medium">Cara pembayaran</span>
              <textarea
                className="input min-h-[80px]"
                value={form.payment_instructions}
                onChange={set("payment_instructions")}
                placeholder={"Transfer ke:\nBCA 1234567890 a.n. PT ISP\nQRIS / VA lewat portal"}
              />
              <span className="text-xs text-[var(--muted)]">Instruksi transfer / rekening. Tampil di bagian bawah invoice.</span>
            </label>

            <label className="grid gap-1 text-sm sm:col-span-2">
              <span className="font-medium">Catatan kaki</span>
              <textarea
                className="input min-h-[60px]"
                value={form.footer_note}
                onChange={set("footer_note")}
                placeholder="Terima kasih telah berlangganan. Tagihan ini sah tanpa tanda tangan."
              />
            </label>

            <div className="flex flex-wrap items-center gap-2 sm:col-span-2">
              <button type="button" className="btn" disabled={save.isPending} onClick={() => save.mutate()}>
                {save.isPending ? "Menyimpan..." : "Simpan"}
              </button>
              {err ? <p className="text-sm text-[var(--danger)]">{err}</p> : null}
            </div>
          </div>

          <div className="grid gap-2 xl:sticky xl:top-4">
            <p className="text-sm font-medium">Pratinjau invoice</p>
            <InvoiceFormPreview
              form={form}
              defaultCompany={defaultCompany}
              logoUrl={logoUrl}
              tenantSlug={q.data?.tenant_slug}
            />
            <p className="text-xs text-[var(--muted)]">
              Contoh dengan data dummy. Watermark LUNAS hanya muncul otomatis pada invoice yang sudah dibayar.
            </p>
          </div>
          </div>
        </div>
      )}
    </Section>
  );
}

const PREVIEW_ITEMS = [
  { desc: "Paket Internet 30 Mbps — Sep 2026", qty: 1, price: 150000, amount: 150000 },
  { desc: "Biaya instalasi (sekali)", qty: 1, price: 50000, amount: 50000 },
];
const PREVIEW_SUBTOTAL = 200000;
const PREVIEW_TAX = 22000;
const PREVIEW_TOTAL = 222000;

function previewLines(value: string): string[] {
  return String(value || "")
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
}

function InvoiceFormPreview({
  form,
  defaultCompany,
  logoUrl,
  tenantSlug,
}: {
  form: InvoiceSettingsData;
  defaultCompany: string;
  logoUrl?: string;
  tenantSlug?: string;
}) {
  const [paidPreview, setPaidPreview] = useState(false);
  const company = form.company_name.trim() || defaultCompany || "Nama Perusahaan";
  const addressLines = previewLines(form.address);
  const contact = [form.phone.trim() && `Telp: ${form.phone.trim()}`, form.email.trim()].filter(Boolean);
  const payLines = previewLines(form.payment_instructions);
  const footLines = previewLines(form.footer_note);

  // Mirror store.FormatInvoiceNumber: INV-<kode pelanggan>-<mmyyyy><6 char>.
  const now = new Date();
  const period = `${String(now.getMonth() + 1).padStart(2, "0")}${now.getFullYear()}`;
  const invoiceNo = `INV-D5N-2026090001-${period}A3F9K2`;
  const fmtDate = (d: Date) => d.toLocaleDateString("id-ID", { day: "2-digit", month: "short", year: "numeric" });
  const issuedAt = fmtDate(new Date(now.getFullYear(), now.getMonth(), 1));
  const dueAt = fmtDate(new Date(now.getFullYear(), now.getMonth(), 8));

  return (
    <div className="grid gap-2">
      <label className="flex w-fit items-center gap-2 text-xs text-[var(--muted)]">
        <input
          type="checkbox"
          checked={paidPreview}
          onChange={(e) => setPaidPreview(e.target.checked)}
        />
        Pratinjau status lunas (watermark)
      </label>
    <div className="overflow-hidden rounded-lg border border-[var(--border)] bg-white text-[13px] leading-relaxed text-slate-800 shadow-sm">
      <div className="h-1.5 bg-[#5a5a40]" aria-hidden />
      <div className="relative mx-auto max-w-[560px] p-6">
        {paidPreview ? (
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center overflow-hidden" aria-hidden>
            <span
              className="select-none text-[96px] font-bold tracking-[0.28em] text-blue-800/15"
              style={{ transform: "rotate(-35deg)" }}
            >
              LUNAS
            </span>
          </div>
        ) : null}
        {/* Header */}
        <div className="relative flex items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            {logoUrl ? (
              <img
                src={logoUrl}
                alt=""
                className="h-12 w-12 shrink-0 rounded-md object-contain"
              />
            ) : null}
            <div className="min-w-0">
              <p className="text-lg font-bold text-slate-900">{company}</p>
              {addressLines.map((l, i) => (
                <p key={`addr-${i}`} className="text-[11px] text-slate-500">
                  {l}
                </p>
              ))}
              {contact.length ? <p className="text-[11px] text-slate-500">{contact.join("  ·  ")}</p> : null}
              {form.website.trim() ? <p className="text-[11px] text-slate-500">{form.website.trim()}</p> : null}
              {form.tax_id.trim() ? <p className="text-[11px] text-slate-500">NPWP: {form.tax_id.trim()}</p> : null}
            </div>
          </div>
          <div className="shrink-0">
            <p className="text-right text-xl font-bold tracking-wide text-[#5a5a40]">INVOICE</p>
            <div className="my-1 border-t border-[#5a5a40]" aria-hidden />
            <div className="grid w-[248px] grid-cols-[74px_1fr] gap-x-1.5 gap-y-0.5 text-[11px]">
              {(
                [
                  ["Nomor", invoiceNo, "text-slate-800"],
                  ["Terbit", issuedAt, "text-slate-800"],
                  ["Jatuh tempo", dueAt, "text-slate-800"],
                  [
                    "Status",
                    paidPreview ? "LUNAS" : "BELUM DIBAYAR",
                    `font-semibold ${paidPreview ? "text-[#5a5a40]" : "text-slate-700"}`,
                  ],
                ] as [string, string, string][]
              ).map(([label, value, cls]) => (
                <Fragment key={label}>
                  <span className="text-slate-500">{label}</span>
                  <span className={`break-words ${cls}`}>{value}</span>
                </Fragment>
              ))}
            </div>
          </div>
        </div>

        <div className="my-4 border-t border-slate-200" />

        {/* Bill to */}
        <p className="text-[10px] font-semibold tracking-wide text-[#5a5a40]">DITAGIHKAN KEPADA</p>
        <p className="mt-1 font-semibold text-slate-900">Budi Santoso</p>
        <p className="text-[11px] text-slate-500">D5N-2026090001</p>
        <p className="text-[11px] text-slate-500">Jl. Kenanga No. 5</p>
        <p className="text-[11px] text-slate-500">0812-3456-7890 · budi@mail.id</p>

        {/* Items */}
        <table className="mt-4 w-full border-collapse text-[12px]" style={{ border: "1px solid #d8d5cc" }}>
          <thead>
            <tr className="bg-[#5a5a40] text-[11px] text-white">
              <th className="border border-[#d8d5cc] px-2 py-1.5 text-left font-semibold">Deskripsi</th>
              <th className="w-12 border border-[#d8d5cc] px-2 py-1.5 text-right font-semibold">Qty</th>
              <th className="w-24 border border-[#d8d5cc] px-2 py-1.5 text-right font-semibold">Harga</th>
              <th className="w-28 border border-[#d8d5cc] px-2 py-1.5 text-right font-semibold">Jumlah</th>
            </tr>
          </thead>
          <tbody>
            {PREVIEW_ITEMS.map((it, i) => (
              <tr key={i} className={i % 2 === 1 ? "bg-slate-50" : undefined}>
                <td className="border border-[#d8d5cc] px-2 py-1.5">{it.desc}</td>
                <td className="border border-[#d8d5cc] px-2 py-1.5 text-right">{it.qty}</td>
                <td className="border border-[#d8d5cc] px-2 py-1.5 text-right">{formatRp(it.price)}</td>
                <td className="border border-[#d8d5cc] px-2 py-1.5 text-right">{formatRp(it.amount)}</td>
              </tr>
            ))}
          </tbody>
        </table>

        {/* Totals */}
        <div className="mt-3 ml-auto w-full max-w-[240px] text-[12px]">
          <div className="flex justify-between py-0.5 text-slate-600">
            <span>Subtotal</span>
            <span>{formatRp(PREVIEW_SUBTOTAL)}</span>
          </div>
          <div className="flex justify-between py-0.5 text-slate-600">
            <span>Pajak</span>
            <span>{formatRp(PREVIEW_TAX)}</span>
          </div>
          <div className="mt-1 flex justify-between border-t border-[#5a5a40] py-1 font-bold text-slate-900">
            <span>TOTAL</span>
            <span>{formatRp(PREVIEW_TOTAL)}</span>
          </div>
          {paidPreview ? (
            <>
              <div className="flex justify-between py-0.5 text-slate-600">
                <span>Terbayar</span>
                <span>{formatRp(PREVIEW_TOTAL)}</span>
              </div>
              <div className="flex justify-between py-0.5 font-bold text-slate-900">
                <span>Sisa tagihan</span>
                <span>{formatRp(0)}</span>
              </div>
            </>
          ) : null}
        </div>

        {/* Payment instructions */}
        {payLines.length ? (
          <div className="mt-5">
            <p className="text-[10px] font-semibold tracking-wide text-[#5a5a40]">CARA PEMBAYARAN</p>
            {payLines.map((l, i) => (
              <p key={`pay-${i}`} className="text-[11px] text-slate-600">
                {l}
              </p>
            ))}
          </div>
        ) : null}

        {/* Footer */}
        {footLines.length ? (
          <div className="mt-4 border-t border-slate-200 pt-2">
            {footLines.map((l, i) => (
              <p key={`foot-${i}`} className="text-[10px] text-slate-500">
                {l}
              </p>
            ))}
          </div>
        ) : null}
      </div>
    </div>
    </div>
  );
}

function BrandAssetField({
  label,
  hint,
  url,
  fromOwner,
  onFile,
  onClear,
  examples,
}: {
  label: string;
  hint?: string;
  url?: string | null;
  fromOwner: boolean;
  onFile: (f: File | undefined) => void;
  onClear: () => void;
  examples?: ExampleIcon[];
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);

  function take(file?: File) {
    if (file) onFile(file);
  }

  function onDrop(e: DragEvent<HTMLButtonElement>) {
    e.preventDefault();
    setOver(false);
    take(e.dataTransfer.files?.[0]);
  }

  return (
    <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] p-3">
      <div className="mb-2 flex items-center justify-between gap-1">
        <p className="truncate text-sm font-semibold text-[var(--text)]" title={hint}>
          {label}
        </p>
        {fromOwner ? (
          <span className="shrink-0 text-[10px] text-[var(--muted)]">owner</span>
        ) : url ? (
          <IconButton label="Hapus override" danger onClick={onClear}>
            <IconTrash />
          </IconButton>
        ) : null}
      </div>

      <div className="flex items-center gap-2">
        <div className="flex size-11 shrink-0 items-center justify-center overflow-hidden rounded-md border border-[var(--border)] bg-[var(--panel-muted)]">
          {url ? (
            <img src={url} alt="" className="h-full w-full object-contain p-0.5" />
          ) : (
            <span className="text-[9px] text-[var(--muted)]">—</span>
          )}
        </div>
        <button
          type="button"
          title={hint ? `${hint} Klik atau seret file.` : `Unggah ${label}`}
          aria-label={`Unggah ${label}`}
          className={cn(
            "flex min-w-0 flex-1 items-center justify-center gap-1.5 rounded-md border border-dashed px-2 py-2 text-xs font-semibold transition-colors",
            over
              ? "border-[var(--accent)] bg-[color-mix(in_srgb,var(--accent)_10%,transparent)] text-[var(--accent)]"
              : "border-[var(--border-strong)] bg-[var(--panel-muted)]/60 text-[var(--text)] hover:border-[var(--accent)]",
          )}
          onClick={() => inputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setOver(true);
          }}
          onDragLeave={() => setOver(false)}
          onDrop={onDrop}
        >
          <IconUpload />
          Unggah
        </button>
        <input
          ref={inputRef}
          type="file"
          accept="image/*,.ico,.svg"
          className="sr-only"
          tabIndex={-1}
          onChange={(e) => {
            take(e.target.files?.[0]);
            e.target.value = "";
          }}
        />
      </div>

      {examples && examples.length > 0 ? (
        <div className="mt-2 flex flex-wrap gap-1">
          {examples.map((ex) => (
            <button
              key={ex.key}
              type="button"
              title={`Pakai contoh ${ex.label}`}
              aria-label={`Pakai contoh ${ex.label}`}
              className="rounded-md border border-[var(--border)] bg-[var(--panel-muted)]/50 p-1 hover:border-[var(--accent)]"
              onClick={() => onFile(exampleIconFile(ex))}
            >
              <img src={exampleIconDataUrl(ex)} alt="" className="size-6 object-contain" />
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

export function RolesSettingsPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const roles = useQuery({
    queryKey: ["settings-roles"],
    queryFn: () => api<Role[]>("/api/settings/roles"),
  });
  const [open, setOpen] = useState(false);
  const [edit, setEdit] = useState<Role | null>(null);
  const [form, setForm] = useState({ name: "", slug: "", permissions: ["dashboard"] as string[] });
  const [err, setErr] = useState("");

  const refresh = () => void qc.invalidateQueries({ queryKey: ["settings-roles"] });

  const create = useMutation({
    mutationFn: () =>
      api("/api/settings/roles", {
        method: "POST",
        body: JSON.stringify({
          name: form.name.trim(),
          slug: form.slug.trim().toLowerCase(),
          permissions: form.permissions,
        }),
      }),
    onSuccess: () => {
      setOpen(false);
      setErr("");
      refresh();
      void toastSuccess("Role dibuat");
    },
    onError: (e: Error) => {
      setErr(e.message);
      void toastError(e.message);
    },
  });

  const update = useMutation({
    mutationFn: (r: Role) =>
      api(`/api/settings/roles/${r.id}`, {
        method: "PUT",
        body: JSON.stringify({
          name: r.name.trim(),
          slug: r.slug.trim().toLowerCase(),
          permissions: r.permissions,
        }),
      }),
    onSuccess: () => {
      setEdit(null);
      refresh();
      void toastSuccess("Role diperbarui");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/settings/roles/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      refresh();
      void toastSuccess("Role dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const list = Array.isArray(roles.data) ? roles.data : [];
  const rows = list.map((r) => [
    r.name,
    r.slug,
    r.permissions.join(", ") || "—",
    r.is_system ? "sistem" : "custom",
    <span key={r.id} className="flex gap-1">
      <IconButton
        label="Edit role"
        onClick={() => {
          setEdit({ ...r, permissions: [...(r.permissions || [])] });
        }}
      >
        <IconPencil />
      </IconButton>
      {!r.is_system && (
        <IconButton
          label="Hapus role"
          danger
          onClick={async () => {
            const ok = await confirm({
              title: "Hapus role",
              description: `Hapus role "${r.name}"?`,
              confirmLabel: "Hapus",
            });
            if (ok) remove.mutate(r.id);
          }}
        >
          <IconTrash />
        </IconButton>
      )}
    </span>,
  ]);

  return (
    <>
      <Section
        title="Roles"
        actions={
          <button
            type="button"
            className="btn"
            onClick={() => {
              setForm({ name: "", slug: "", permissions: ["dashboard"] });
              setErr("");
              setOpen(true);
            }}
          >
            + Role
          </button>
        }
      >
        {roles.isLoading ? (
          <p className="text-[var(--muted)]">Memuat...</p>
        ) : (
          <Table columns={["Nama", "Slug", "Permissions", "Tipe", "Aksi"]} rows={rows} />
        )}
      </Section>

      <FormDialog open={open} title="Buat role" onClose={() => setOpen(false)}>
        <RoleForm
          form={form}
          setForm={setForm}
          err={err}
          busy={create.isPending}
          onSubmit={() => create.mutate()}
          onCancel={() => setOpen(false)}
        />
      </FormDialog>

      <FormDialog open={Boolean(edit)} title="Edit role" onClose={() => setEdit(null)}>
        {edit && (
          <RoleForm
            form={{ name: edit.name, slug: edit.slug, permissions: edit.permissions || [] }}
            setForm={(next) =>
              setEdit({
                ...edit,
                name: next.name,
                slug: next.slug,
                permissions: next.permissions,
              })
            }
            err=""
            busy={update.isPending}
            slugLocked={edit.is_system}
            onSubmit={() => update.mutate(edit)}
            onCancel={() => setEdit(null)}
          />
        )}
      </FormDialog>
    </>
  );
}

function RoleForm({
  form,
  setForm,
  err,
  busy,
  slugLocked,
  onSubmit,
  onCancel,
}: {
  form: { name: string; slug: string; permissions: string[] };
  setForm: (f: { name: string; slug: string; permissions: string[] }) => void;
  err: string;
  busy: boolean;
  slugLocked?: boolean;
  onSubmit: () => void;
  onCancel: () => void;
}) {
  function toggle(p: string) {
    const set = new Set(form.permissions);
    if (set.has(p)) set.delete(p);
    else set.add(p);
    setForm({ ...form, permissions: [...set] });
  }
  return (
    <form
      className="grid gap-2.5"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit();
      }}
    >
      <div className="grid gap-2 sm:grid-cols-2">
        <input
          className="input"
          placeholder="Nama role"
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
          required
        />
        <input
          className="input"
          placeholder="slug"
          value={form.slug}
          onChange={(e) => setForm({ ...form, slug: e.target.value })}
          required
          disabled={slugLocked}
          pattern="[a-z][a-z0-9_-]{1,62}"
          title="huruf kecil, angka, _ atau -"
        />
      </div>
      <div>
        <p className="mb-1.5 text-xs text-[var(--muted)]">Izin menu</p>
        <div className="max-h-56 space-y-0.5 overflow-y-auto rounded-md border border-[var(--border)] p-1.5">
          {PERM_PRESETS.map((p) => (
            <label
              key={p.key}
              className="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-[var(--panel-muted)]/60"
              title={p.hint}
            >
              <input
                type="checkbox"
                className="shrink-0"
                checked={form.permissions.includes(p.key)}
                onChange={() => toggle(p.key)}
              />
              <span className="min-w-0 truncate font-medium">{p.label}</span>
              <code className="ml-auto shrink-0 text-[10px] text-[var(--muted)]">{p.key}</code>
            </label>
          ))}
        </div>
      </div>
      <div className="flex gap-2 pt-0.5">
        <button className="btn" disabled={busy}>
          {busy ? "Menyimpan..." : "Simpan"}
        </button>
        <button type="button" className="btn-ghost" onClick={onCancel}>
          Batal
        </button>
      </div>
      {err && <p className="text-sm text-[var(--danger)]">{err}</p>}
    </form>
  );
}

export function UsersSettingsPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [tab, setTab] = usePersistedTab("users", "staff", ["staff", "portal"] as const);
  const roles = useQuery({
    queryKey: ["settings-roles"],
    queryFn: () => api<Role[]>("/api/settings/roles"),
  });
  const users = useQuery({
    queryKey: ["settings-users"],
    queryFn: () => api<TenantUser[]>("/api/settings/users"),
  });
  const portal = useQuery({
    queryKey: ["settings-portal-users"],
    queryFn: () => api<PortalUser[]>("/api/settings/portal-users"),
  });

  const [createOpen, setCreateOpen] = useState(false);
  const [editUser, setEditUser] = useState<TenantUser | null>(null);
  const [editPortal, setEditPortal] = useState<PortalUser | null>(null);
  const [form, setForm] = useState({
    email: "",
    password: "",
    full_name: "",
    phone: "",
    role_id: "",
  });
  const [editForm, setEditForm] = useState({
    full_name: "",
    phone: "",
    role_id: "",
    is_active: true,
    password: "",
  });
  const [portalForm, setPortalForm] = useState({ portal_enabled: false, password: "" });
  const [err, setErr] = useState("");

  const roleList = Array.isArray(roles.data) ? roles.data : [];
  const userList = Array.isArray(users.data) ? users.data : [];
  const portalList = Array.isArray(portal.data) ? portal.data : [];

  const refreshUsers = () => {
    void qc.invalidateQueries({ queryKey: ["settings-users"] });
    void qc.invalidateQueries({ queryKey: ["settings-portal-users"] });
  };

  const createUser = useMutation({
    mutationFn: () =>
      api("/api/settings/users", {
        method: "POST",
        body: JSON.stringify({
          email: form.email.trim(),
          password: form.password,
          full_name: form.full_name.trim(),
          phone: form.phone.trim() || null,
          role_id: form.role_id,
        }),
      }),
    onSuccess: () => {
      setCreateOpen(false);
      setErr("");
      refreshUsers();
      void toastSuccess("User staf ditambahkan");
    },
    onError: (e: Error) => {
      setErr(e.message);
      void toastError(e.message);
    },
  });

  const updateUser = useMutation({
    mutationFn: () =>
      api(`/api/settings/users/${editUser!.user_id}`, {
        method: "PUT",
        body: JSON.stringify({
          full_name: editForm.full_name.trim(),
          phone: editForm.phone.trim() || null,
          role_id: editForm.role_id,
          is_active: editForm.is_active,
          password: editForm.password || undefined,
        }),
      }),
    onSuccess: () => {
      setEditUser(null);
      refreshUsers();
      void toastSuccess("User diperbarui");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const removeUser = useMutation({
    mutationFn: (id: string) => api(`/api/settings/users/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      refreshUsers();
      void toastSuccess("User dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const updatePortal = useMutation({
    mutationFn: () =>
      api(`/api/settings/portal-users/${editPortal!.id}`, {
        method: "PUT",
        body: JSON.stringify({
          portal_enabled: portalForm.portal_enabled,
          password: portalForm.password || undefined,
        }),
      }),
    onSuccess: () => {
      setEditPortal(null);
      refreshUsers();
      void toastSuccess("Portal pelanggan diperbarui");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const staffRows = userList.map((u) => [
    u.full_name,
    u.email,
    u.role_name,
    u.is_active ? "aktif" : "nonaktif",
    <span key={u.user_id} className="flex gap-1">
      <IconButton
        label="Edit user"
        onClick={() => {
          setEditUser(u);
          setEditForm({
            full_name: u.full_name,
            phone: u.phone || "",
            role_id: u.role_id,
            is_active: u.is_active,
            password: "",
          });
        }}
      >
        <IconPencil />
      </IconButton>
      <IconButton
        label="Hapus user"
        danger
        onClick={async () => {
          const ok = await confirm({
            title: "Hapus user",
            description: `Hapus ${u.email} dari aplikasi ini?`,
            confirmLabel: "Hapus",
          });
          if (ok) removeUser.mutate(u.user_id);
        }}
      >
        <IconTrash />
      </IconButton>
    </span>,
  ]);

  const portalRows = portalList.map((p) => [
    p.customer_code,
    p.full_name,
    p.phone,
    p.portal_enabled ? "ya" : "tidak",
    p.has_password ? "ya" : "belum",
    <IconButton
      key={p.id}
      label="Edit portal"
      onClick={() => {
        setEditPortal(p);
        setPortalForm({ portal_enabled: p.portal_enabled, password: "" });
      }}
    >
      <IconPencil />
    </IconButton>,
  ]);

  return (
    <div className="space-y-6">
      <Tabs value={tab} onValueChange={setTab} className="space-y-0">
        <TabsList aria-label="Users">
          <TabsTrigger value="staff">
            <UserRound />
            User staf
          </TabsTrigger>
          <TabsTrigger value="portal">
            <Globe />
            Portal pelanggan
          </TabsTrigger>
        </TabsList>
        <TabsContent value="staff">
          <Section
            title="User staf"
            actions={
              <button
                type="button"
                className="btn"
                onClick={() => {
                  setForm({
                    email: "",
                    password: "",
                    full_name: "",
                    phone: "",
                    role_id: roleList[0]?.id || "",
                  });
                  setErr("");
                  setCreateOpen(true);
                }}
              >
                + User
              </button>
            }
          >
            {users.isLoading ? (
              <p className="text-[var(--muted)]">Memuat...</p>
            ) : (
              <Table columns={["Nama", "Email", "Role", "Status", "Aksi"]} rows={staffRows} />
            )}
          </Section>
        </TabsContent>
        <TabsContent value="portal">
          <Section title="Portal pelanggan">
            <p className="mb-3 text-sm text-[var(--muted)]">
              Password default portal = nomor HP pelanggan. Kosongkan field password saat edit untuk
              mempertahankan password yang ada (atau mengisi otomatis dari HP bila belum punya password).
            </p>
            {portal.isLoading ? (
              <p className="text-[var(--muted)]">Memuat...</p>
            ) : (
              <Table
                columns={["Kode", "Nama", "Telepon", "Portal", "Password", "Aksi"]}
                rows={portalRows}
              />
            )}
          </Section>
        </TabsContent>
      </Tabs>

      <FormDialog open={createOpen} title="Tambah user staf" onClose={() => setCreateOpen(false)} wide>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            createUser.mutate();
          }}
        >
          <input
            className="input"
            placeholder="Nama lengkap"
            value={form.full_name}
            onChange={(e) => setForm({ ...form, full_name: e.target.value })}
            required
          />
          <input
            className="input"
            type="email"
            placeholder="Email"
            value={form.email}
            onChange={(e) => setForm({ ...form, email: e.target.value })}
            required
          />
          <SecretInput
            placeholder="Password (min 8)"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            required
            minLength={8}
            autoComplete="new-password"
          />
          <input
            className="input"
            placeholder="Telepon"
            value={form.phone}
            onChange={(e) => setForm({ ...form, phone: e.target.value })}
          />
          <select
            className="input sm:col-span-2"
            value={form.role_id}
            onChange={(e) => setForm({ ...form, role_id: e.target.value })}
            required
          >
            <option value="">Pilih role</option>
            {roleList.map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
          <div className="flex gap-2 sm:col-span-2">
            <button className="btn" disabled={createUser.isPending}>
              {createUser.isPending ? "Menyimpan..." : "Buat"}
            </button>
            <button type="button" className="btn-ghost" onClick={() => setCreateOpen(false)}>
              Batal
            </button>
          </div>
          {err && <p className="text-sm text-[var(--danger)] sm:col-span-2">{err}</p>}
        </form>
      </FormDialog>

      <FormDialog open={Boolean(editUser)} title="Edit user staf" onClose={() => setEditUser(null)} wide>
        {editUser && (
          <form
            className="grid gap-3 sm:grid-cols-2"
            onSubmit={(e) => {
              e.preventDefault();
              updateUser.mutate();
            }}
          >
            <input className="input bg-[var(--panel-muted)] text-[var(--muted)]" value={editUser.email} disabled />
            <input
              className="input"
              placeholder="Nama"
              value={editForm.full_name}
              onChange={(e) => setEditForm({ ...editForm, full_name: e.target.value })}
              required
            />
            <input
              className="input"
              placeholder="Telepon"
              value={editForm.phone}
              onChange={(e) => setEditForm({ ...editForm, phone: e.target.value })}
            />
            <select
              className="input"
              value={editForm.role_id}
              onChange={(e) => setEditForm({ ...editForm, role_id: e.target.value })}
              required
            >
              {roleList.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </select>
            <SecretInput
              className="sm:col-span-2"
              placeholder="Password baru (opsional)"
              value={editForm.password}
              onChange={(e) => setEditForm({ ...editForm, password: e.target.value })}
              autoComplete="new-password"
            />
            <label className="flex items-center gap-2 text-sm sm:col-span-2">
              <input
                type="checkbox"
                checked={editForm.is_active}
                onChange={(e) => setEditForm({ ...editForm, is_active: e.target.checked })}
              />
              Aktif
            </label>
            <div className="flex gap-2 sm:col-span-2">
              <button className="btn" disabled={updateUser.isPending}>
                Simpan
              </button>
              <button type="button" className="btn-ghost" onClick={() => setEditUser(null)}>
                Batal
              </button>
            </div>
          </form>
        )}
      </FormDialog>

      <FormDialog
        open={Boolean(editPortal)}
        title={editPortal ? `Portal · ${editPortal.full_name}` : "Portal"}
        onClose={() => setEditPortal(null)}
      >
        {editPortal && (
          <form
            className="grid gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              updatePortal.mutate();
            }}
          >
            <p className="text-sm text-[var(--muted)]">
              {editPortal.customer_code} · {editPortal.phone}
            </p>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={portalForm.portal_enabled}
                onChange={(e) => setPortalForm({ ...portalForm, portal_enabled: e.target.checked })}
              />
              Aktifkan login portal
            </label>
            <SecretInput
              placeholder="Password baru (opsional; default = nomor HP)"
              value={portalForm.password}
              onChange={(e) => setPortalForm({ ...portalForm, password: e.target.value })}
              autoComplete="new-password"
            />
            <div className="flex gap-2">
              <button className="btn" disabled={updatePortal.isPending}>
                Simpan
              </button>
              <button type="button" className="btn-ghost" onClick={() => setEditPortal(null)}>
                Batal
              </button>
            </div>
          </form>
        )}
      </FormDialog>
    </div>
  );
}
