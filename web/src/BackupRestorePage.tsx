import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, getPlatformToken, getToken } from "./api";
import { useAppDialog } from "./confirm";
import { IconDownload, IconTrash, IconUpload } from "./icons";
import { toastError, toastSuccess } from "./swal";
import { IconButton, Section, Table } from "./ui";

type BackupFile = {
  name: string;
  size: number;
  created_at: string;
  scope: string;
};

function formatBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

async function downloadBackup(url: string, filename: string, platform: boolean) {
  const token = platform ? getPlatformToken() : getToken();
  const res = await fetch(url, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    credentials: "include",
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  const blob = await res.blob();
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  a.click();
  URL.revokeObjectURL(a.href);
}

async function uploadRestore(url: string, file: File, platform: boolean) {
  const token = platform ? getPlatformToken() : getToken();
  const body = new FormData();
  body.append("file", file);
  body.append("confirm", "true");
  const res = await fetch(url, {
    method: "POST",
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body,
    credentials: "include",
  });
  if (!res.ok) {
    const text = await res.text();
    try {
      const j = JSON.parse(text) as { error?: string };
      throw new Error(j.error || text);
    } catch (e) {
      if (e instanceof SyntaxError) throw new Error(text || res.statusText);
      throw e;
    }
  }
}

export function BackupRestorePage({ platform = false }: { platform?: boolean }) {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);

  const base = platform ? "/api/platform/db-backups" : "/api/db-backups";
  const qKey = platform ? ["platform-db-backups"] : ["tenant-db-backups"];

  const list = useQuery({
    queryKey: qKey,
    queryFn: () => api<BackupFile[]>(base, {}, { platform }),
  });

  const create = useMutation({
    mutationFn: () => api<BackupFile>(base, { method: "POST", body: "{}" }, { platform }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qKey });
      void toastSuccess("Backup dibuat");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const remove = useMutation({
    mutationFn: (name: string) => api(`${base}/${encodeURIComponent(name)}`, { method: "DELETE" }, { platform }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qKey });
      void toastSuccess("Backup dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const restoreNamed = useMutation({
    mutationFn: (name: string) =>
      api(`${base}/restore`, {
        method: "POST",
        body: JSON.stringify({ name, confirm: true }),
      }, { platform }),
    onSuccess: () => void toastSuccess("Restore selesai"),
    onError: (e: Error) => void toastError(e.message),
  });

  const rows = (Array.isArray(list.data) ? list.data : []).map((f) => [
    f.name,
    formatBytes(f.size),
    f.created_at ? new Date(f.created_at).toLocaleString("id-ID") : "—",
    <div key={f.name} className="flex items-center gap-1">
      <IconButton
        label="Unduh"
        onClick={() => {
          void downloadBackup(`${base}/${encodeURIComponent(f.name)}/download`, f.name, platform).catch((e: Error) =>
            toastError(e.message),
          );
        }}
      >
        <IconDownload />
      </IconButton>
      <IconButton
        label="Restore"
        onClick={async () => {
          const ok = await confirm({
            title: "Restore database?",
            description: platform
              ? `Ini akan menimpa SELURUH database dari ${f.name}. Tidak bisa dibatalkan.`
              : `Ini akan menimpa data tenant dari ${f.name}. Tidak bisa dibatalkan.`,
            confirmLabel: "Restore",
            danger: true,
          });
          if (!ok) return;
          restoreNamed.mutate(f.name);
        }}
      >
        <IconUpload />
      </IconButton>
      <IconButton
        label="Hapus"
        danger
        onClick={async () => {
          const ok = await confirm({
            title: "Hapus backup?",
            description: f.name,
            confirmLabel: "Hapus",
            danger: true,
          });
          if (!ok) return;
          remove.mutate(f.name);
        }}
      >
        <IconTrash />
      </IconButton>
    </div>,
  ]);

  return (
    <Section title={platform ? "Backup / Restore Database" : "Backup / Restore Data Tenant"}>
      <p className="mb-4 text-sm text-[var(--muted)]">
        {platform
          ? "Backup penuh via pg_dump (.sql.gz). Restore memakai psql dan menimpa seluruh database."
          : "Export JSON data tenant (tabel ber-tenant_id). Restore menimpa data tenant ini saja."}
      </p>
      <div className="mb-4 flex flex-wrap gap-2">
        <button type="button" className="btn" disabled={create.isPending} onClick={() => create.mutate()}>
          {create.isPending ? "Membuat..." : "Buat backup"}
        </button>
        <button
          type="button"
          className="btn-ghost"
          disabled={busy}
          onClick={() => fileRef.current?.click()}
        >
          Restore dari file…
        </button>
        <input
          ref={fileRef}
          type="file"
          className="hidden"
          accept={platform ? ".sql,.gz,.sql.gz" : ".json,.gz,.json.gz"}
          onChange={async (e) => {
            const file = e.target.files?.[0];
            e.target.value = "";
            if (!file) return;
            const ok = await confirm({
              title: "Restore dari file?",
              description: platform
                ? `File ${file.name} akan menimpa seluruh database.`
                : `File ${file.name} akan menimpa data tenant.`,
              confirmLabel: "Restore",
              danger: true,
            });
            if (!ok) return;
            setBusy(true);
            try {
              await uploadRestore(`${base}/restore-upload`, file, platform);
              void qc.invalidateQueries({ queryKey: qKey });
              void toastSuccess("Restore selesai");
            } catch (err) {
              void toastError(err instanceof Error ? err.message : "Restore gagal");
            } finally {
              setBusy(false);
            }
          }}
        />
      </div>
      {list.isLoading ? (
        <p className="text-[var(--muted)]">Memuat...</p>
      ) : (
        <Table columns={["File", "Ukuran", "Dibuat", "Aksi"]} rows={rows.length ? rows : [["Belum ada backup", "", "", ""]]} />
      )}
    </Section>
  );
}
