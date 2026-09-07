import { useCallback, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, X } from "lucide-react";
import { api, apiUpload } from "../api";
import { ProgressFileUpload } from "../ProgressFileUpload";
import { IconTrash } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import { Button, IconButton, Label, Section } from "../ui";

const DOC_KINDS: { id: string; label: string }[] = [
  { id: "ktp", label: "KTP / identitas" },
  { id: "rumah", label: "Rumah / lokasi" },
  { id: "odp", label: "ODP" },
  { id: "psb", label: "Proses PSB" },
  { id: "other", label: "Lainnya" },
];

function docKindLabel(kind: string) {
  return DOC_KINDS.find((k) => k.id === kind)?.label || kind;
}

type CustomerDoc = {
  id: string;
  kind: string;
  url: string;
  caption?: string;
  source?: string;
  uploader_name?: string;
  created_at: string;
};

type CustomerInfo = {
  id: string;
  full_name: string;
  customer_code: string;
  phone?: string;
};

function ImageLightbox({
  docs,
  index,
  onClose,
  onIndexChange,
}: {
  docs: CustomerDoc[];
  index: number;
  onClose: () => void;
  onIndexChange: (i: number) => void;
}) {
  const doc = docs[index];
  const hasPrev = index > 0;
  const hasNext = index < docs.length - 1;

  const goPrev = useCallback(() => {
    if (hasPrev) onIndexChange(index - 1);
  }, [hasPrev, index, onIndexChange]);

  const goNext = useCallback(() => {
    if (hasNext) onIndexChange(index + 1);
  }, [hasNext, index, onIndexChange]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key === "ArrowLeft") goPrev();
      if (e.key === "ArrowRight") goNext();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, goPrev, goNext]);

  if (!doc) return null;

  return (
    <div
      className="fixed inset-0 z-[80] flex flex-col bg-black/85"
      role="dialog"
      aria-modal="true"
      aria-label="Lihat gambar"
      onClick={onClose}
    >
      <div className="flex items-center justify-between gap-3 px-4 py-3 text-white" onClick={(e) => e.stopPropagation()}>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">
            {docKindLabel(doc.kind)}
            {doc.source === "lead_convert" ? " · dari lead" : ""}
          </p>
          <p className="text-xs text-white/70">
            {index + 1} / {docs.length}
            {doc.uploader_name ? ` · ${doc.uploader_name}` : ""}
          </p>
        </div>
        <IconButton label="Tutup" className="text-white hover:bg-white/10" onClick={onClose}>
          <X className="size-5" />
        </IconButton>
      </div>

      <div className="relative flex min-h-0 flex-1 items-center justify-center px-12 py-4" onClick={(e) => e.stopPropagation()}>
        {hasPrev ? (
          <button
            type="button"
            className="absolute left-2 top-1/2 z-10 -translate-y-1/2 rounded-full bg-white/15 p-2 text-white hover:bg-white/25"
            title="Sebelumnya"
            aria-label="Sebelumnya"
            onClick={goPrev}
          >
            <ChevronLeft className="size-6" />
          </button>
        ) : null}
        <img
          src={doc.url}
          alt={docKindLabel(doc.kind)}
          className="max-h-full max-w-full rounded-md object-contain shadow-lg"
        />
        {hasNext ? (
          <button
            type="button"
            className="absolute right-2 top-1/2 z-10 -translate-y-1/2 rounded-full bg-white/15 p-2 text-white hover:bg-white/25"
            title="Berikutnya"
            aria-label="Berikutnya"
            onClick={goNext}
          >
            <ChevronRight className="size-6" />
          </button>
        ) : null}
      </div>
    </div>
  );
}

export function CustomerGalleryPage({
  customerId,
  onBack,
}: {
  customerId: string;
  onBack: () => void;
}) {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [docKind, setDocKind] = useState("psb");
  const [viewerIndex, setViewerIndex] = useState<number | null>(null);

  const custQ = useQuery({
    queryKey: ["customer", customerId],
    queryFn: () => api<CustomerInfo>(`/api/customers/${customerId}`),
  });

  const docsQ = useQuery({
    queryKey: ["customer-documents", customerId],
    queryFn: () => api<CustomerDoc[]>(`/api/customers/${customerId}/documents`),
  });
  const docs = Array.isArray(docsQ.data) ? docsQ.data : [];

  const removeDoc = useMutation({
    mutationFn: (docId: string) => api(`/api/customers/${customerId}/documents/${docId}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["customer-documents", customerId] });
      void toastSuccess("Dokumen dihapus");
      setViewerIndex(null);
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const titleName = custQ.data?.full_name || "Pelanggan";
  const code = custQ.data?.customer_code;

  return (
    <>
      <Section
        title="Dokumentasi / Galeri"
        actions={
          <Button type="button" variant="outline" size="sm" onClick={onBack}>
            ← Kembali ke pelanggan
          </Button>
        }
      >
        <p className="mb-4 text-sm text-[var(--muted)]">
          {custQ.isLoading ? (
            "Memuat…"
          ) : (
            <>
              <span className="font-medium text-[var(--text)]">{titleName}</span>
              {code ? ` · ${code}` : ""}
              {custQ.data?.phone ? ` · ${custQ.data.phone}` : ""}
            </>
          )}
        </p>

        <div className="mb-6 grid gap-3 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel)] p-4">
          <div className="min-w-[160px] max-w-xs">
            <Label className="mb-1 block text-xs">Jenis dokumen</Label>
            <select className="input" value={docKind} onChange={(e) => setDocKind(e.target.value)}>
              {DOC_KINDS.map((k) => (
                <option key={k.id} value={k.id}>
                  {k.label}
                </option>
              ))}
            </select>
          </div>
          <ProgressFileUpload
            label="Unggah gambar"
            hint="JPG/PNG/WebP · dikompres otomatis · bisa banyak"
            uploadFile={async (file, onProgress) => {
              const up = await apiUpload<{ url: string }>(
                `/api/customers/${customerId}/documents/photos`,
                file,
                { onProgress },
              );
              await api(`/api/customers/${customerId}/documents`, {
                method: "POST",
                body: JSON.stringify({ kind: docKind, url: up.url }),
              });
              return up.url;
            }}
            onBatchComplete={(urls) => {
              void qc.invalidateQueries({ queryKey: ["customer-documents", customerId] });
              void toastSuccess(urls.length > 1 ? `${urls.length} dokumen diunggah` : "Dokumen diunggah");
            }}
          />
        </div>

        {docsQ.isLoading ? (
          <p className="text-sm text-[var(--muted)]">Memuat galeri…</p>
        ) : docs.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">Belum ada dokumen. Unggah di atas, atau dari convert lead.</p>
        ) : (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
            {docs.map((d, i) => (
              <div key={d.id} className="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel)]">
                <button
                  type="button"
                  className="block w-full text-left"
                  title="Lihat gambar"
                  aria-label={`Lihat ${docKindLabel(d.kind)}`}
                  onClick={() => setViewerIndex(i)}
                >
                  <img src={d.url} alt="" className="h-40 w-full object-cover transition hover:opacity-90" />
                </button>
                <div className="flex items-center justify-between gap-1 px-2 py-1.5">
                  <div className="min-w-0">
                    <p className="truncate text-xs font-medium text-[var(--text)]">{docKindLabel(d.kind)}</p>
                    <p className="truncate text-[10px] text-[var(--muted)]">
                      {d.source === "lead_convert" ? "dari lead" : d.uploader_name || "upload"}
                    </p>
                  </div>
                  <IconButton
                    label="Hapus dokumen"
                    danger
                    onClick={() => {
                      void confirm({
                        title: "Hapus dokumen?",
                        description: docKindLabel(d.kind),
                        confirmLabel: "Hapus",
                        danger: true,
                      }).then((ok) => {
                        if (ok) removeDoc.mutate(d.id);
                      });
                    }}
                  >
                    <IconTrash />
                  </IconButton>
                </div>
              </div>
            ))}
          </div>
        )}
      </Section>

      {viewerIndex != null && docs[viewerIndex] ? (
        <ImageLightbox
          docs={docs}
          index={viewerIndex}
          onClose={() => setViewerIndex(null)}
          onIndexChange={setViewerIndex}
        />
      ) : null}
    </>
  );
}
