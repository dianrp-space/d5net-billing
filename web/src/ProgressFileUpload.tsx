import { useEffect, useId, useRef, useState } from "react";
import { CheckCircle2, ImagePlus, Loader2, XCircle } from "lucide-react";
import { Progress } from "@/components/ui/progress";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { compressImageForUpload } from "./imageCompress";
import { toastError } from "./swal";

export type ProgressUploadItem = {
  id: string;
  fileName: string;
  previewUrl: string;
  status: "queued" | "compressing" | "uploading" | "done" | "error";
  progress: number;
  error?: string;
  resultUrl?: string;
};

type ProgressFileUploadProps = {
  label?: string;
  hint?: string;
  accept?: string;
  multiple?: boolean;
  maxFiles?: number;
  disabled?: boolean;
  className?: string;
  /** Upload one compressed file; report 0–100 progress. Return public URL. */
  uploadFile: (file: File, onProgress: (pct: number) => void) => Promise<string>;
  /** Fired after each successful file (URL). */
  onFileDone?: (url: string, item: ProgressUploadItem) => void | Promise<void>;
  /** Fired when the whole batch finishes (successes only). */
  onBatchComplete?: (urls: string[]) => void;
};

function statusLabel(s: ProgressUploadItem["status"]) {
  switch (s) {
    case "queued":
      return "Antrian";
    case "compressing":
      return "Kompres…";
    case "uploading":
      return "Mengunggah…";
    case "done":
      return "Selesai";
    case "error":
      return "Gagal";
  }
}

export function ProgressFileUpload({
  label = "Pilih gambar",
  hint,
  accept = "image/jpeg,image/png,image/webp,image/gif,.jpg,.jpeg,.png,.webp",
  multiple = true,
  maxFiles = 12,
  disabled,
  className,
  uploadFile,
  onFileDone,
  onBatchComplete,
}: ProgressFileUploadProps) {
  const inputId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const [items, setItems] = useState<ProgressUploadItem[]>([]);
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const previewsRef = useRef<string[]>([]);
  const uploadFileRef = useRef(uploadFile);
  const onFileDoneRef = useRef(onFileDone);
  const onBatchCompleteRef = useRef(onBatchComplete);
  uploadFileRef.current = uploadFile;
  onFileDoneRef.current = onFileDone;
  onBatchCompleteRef.current = onBatchComplete;

  useEffect(() => {
    return () => {
      for (const u of previewsRef.current) URL.revokeObjectURL(u);
      previewsRef.current = [];
    };
  }, []);

  function patchItem(id: string, patch: Partial<ProgressUploadItem>) {
    setItems((prev) => prev.map((it) => (it.id === id ? { ...it, ...patch } : it)));
  }

  async function runBatch(files: File[]) {
    if (!files.length || busyRef.current) return;
    busyRef.current = true;
    setBusy(true);

    const batch: ProgressUploadItem[] = files.slice(0, maxFiles).map((file, i) => {
      const previewUrl = URL.createObjectURL(file);
      previewsRef.current.push(previewUrl);
      return {
        id: `${Date.now()}-${i}-${file.name}`,
        fileName: file.name,
        previewUrl,
        status: "queued" as const,
        progress: 0,
      };
    });
    // Show queue UI immediately before any await
    setItems(batch);

    const urls: string[] = [];
    for (let i = 0; i < batch.length; i++) {
      const item = batch[i];
      const raw = files[i];
      try {
        patchItem(item.id, { status: "compressing", progress: 8 });
        const compressed = await compressImageForUpload(raw);
        patchItem(item.id, { status: "uploading", progress: 15 });
        const url = await uploadFileRef.current(compressed, (pct) => {
          const mapped = 15 + Math.round((pct / 100) * 80);
          patchItem(item.id, { progress: mapped, status: "uploading" });
        });
        patchItem(item.id, { status: "done", progress: 100, resultUrl: url });
        urls.push(url);
        await onFileDoneRef.current?.(url, { ...item, status: "done", progress: 100, resultUrl: url });
      } catch (e) {
        const message = e instanceof Error ? e.message : "Upload gagal";
        patchItem(item.id, { status: "error", progress: 100, error: message });
      }
    }

    busyRef.current = false;
    setBusy(false);
    if (urls.length) {
      onBatchCompleteRef.current?.(urls);
    } else if (batch.length) {
      void toastError("Semua upload gagal — cek jenis/ukuran gambar");
    }
  }

  const doneCount = items.filter((i) => i.status === "done").length;
  const errorCount = items.filter((i) => i.status === "error").length;
  const overall =
    items.length === 0
      ? 0
      : Math.round(
          items.reduce((s, it) => s + (it.status === "done" || it.status === "error" ? 100 : it.progress), 0) /
            items.length,
        );

  return (
    <div className={cn("grid gap-3", className)}>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled || busy}
          onClick={() => inputRef.current?.click()}
        >
          {busy ? <Loader2 className="size-4 animate-spin" /> : <ImagePlus className="size-4" />}
          {busy ? "Mengunggah…" : label}
        </Button>
        <input
          id={inputId}
          ref={inputRef}
          type="file"
          accept={accept}
          multiple={multiple}
          className="sr-only"
          tabIndex={-1}
          disabled={disabled || busy}
          onChange={(e) => {
            // FileList is live — copy BEFORE clearing input value.
            const files = Array.from(e.target.files ?? []);
            e.target.value = "";
            if (files.length) void runBatch(files);
          }}
        />
        {hint ? <span className="text-xs text-[var(--muted)]">{hint}</span> : null}
      </div>

      {items.length > 0 ? (
        <div className="grid gap-2 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/30 p-3">
          <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
            <span className="font-medium text-[var(--text)]">
              Progress: {doneCount}/{items.length} selesai
              {errorCount ? ` · ${errorCount} gagal` : ""}
            </span>
            <span className="text-[var(--muted)]">{overall}%</span>
          </div>
          <Progress value={overall} className="h-2" />

          <ul className="mt-1 max-h-56 space-y-2 overflow-y-auto">
            {items.map((it) => (
              <li
                key={it.id}
                className="flex items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel)] p-2"
              >
                <img src={it.previewUrl} alt="" className="h-12 w-12 shrink-0 rounded object-cover" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2">
                    <p className="truncate text-xs font-medium text-[var(--text)]">{it.fileName}</p>
                    {it.status === "done" ? (
                      <CheckCircle2 className="size-4 shrink-0 text-[var(--success,var(--accent))]" />
                    ) : it.status === "error" ? (
                      <XCircle className="size-4 shrink-0 text-[var(--danger)]" />
                    ) : (
                      <Loader2 className="size-4 shrink-0 animate-spin text-[var(--muted)]" />
                    )}
                  </div>
                  <p className="text-[10px] text-[var(--muted)]">
                    {it.error || statusLabel(it.status)}
                    {it.status === "uploading" || it.status === "compressing" ? ` · ${it.progress}%` : ""}
                  </p>
                  {it.status !== "done" && it.status !== "error" ? (
                    <Progress value={it.progress} className="mt-1 h-1.5" />
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
