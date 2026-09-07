/** Client-side image compress → WebP before upload (saves bandwidth; server also re-encodes). */

const MAX_EDGE = 1600;
const QUALITY = 0.82;

function isRasterImage(file: File): boolean {
  const t = (file.type || "").toLowerCase();
  if (t.startsWith("image/") && t !== "image/svg+xml") return true;
  const name = file.name.toLowerCase();
  return /\.(jpe?g|png|gif|webp|bmp)$/i.test(name);
}

export async function compressImageForUpload(file: File): Promise<File> {
  if (!isRasterImage(file)) return file;
  // Already small webp under ~400KB — skip canvas work
  if (file.type === "image/webp" && file.size < 400_000) return file;

  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file);
  } catch {
    return file;
  }

  try {
    let { width, height } = bitmap;
    if (width < 1 || height < 1) return file;
    const scale = Math.min(1, MAX_EDGE / Math.max(width, height));
    width = Math.max(1, Math.round(width * scale));
    height = Math.max(1, Math.round(height * scale));

    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext("2d");
    if (!ctx) return file;
    ctx.drawImage(bitmap, 0, 0, width, height);

    const blob: Blob | null = await new Promise((resolve) => {
      canvas.toBlob((b) => resolve(b), "image/webp", QUALITY);
    });
    if (!blob || blob.size === 0) return file;
    // Keep original if somehow larger after compress
    if (blob.size >= file.size && file.type === "image/webp") return file;

    const base = file.name.replace(/\.[^.]+$/, "") || "photo";
    return new File([blob], `${base}.webp`, { type: "image/webp", lastModified: Date.now() });
  } finally {
    bitmap.close();
  }
}

export async function compressImagesForUpload(files: FileList | File[]): Promise<File[]> {
  const list = Array.from(files);
  const out: File[] = [];
  for (const f of list) {
    out.push(await compressImageForUpload(f));
  }
  return out;
}
