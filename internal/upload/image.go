package upload

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/HugoSmits86/nativewebp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// MaxBytes is the maximum accepted upload size before compression (phone photos).
const MaxBytes = 15 << 20 // 15 MiB

// MaxEdge is the longest side kept after resize (saves storage/bandwidth).
const MaxEdge = 1600

// MaxMapIconEdge is the longest side kept after resize for map icons.
// Map pins render at ~32px, so a 128px edge keeps them crisp on retina while tiny on disk.
const MaxMapIconEdge = 128

var rasterExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".ico": true,
}

// SaveImageAsWebP writes an uploaded image under dir as {kind}.webp.
// Inputs are resized (max MaxEdge) and always re-encoded to WebP for smaller files.
// SVG/HEIC are rejected. Older sibling files with other extensions are removed.
func SaveImageAsWebP(dir, kind string, src io.Reader, filename string, declaredSize int64) (publicName string, err error) {
	return saveImageAsWebPWithEdge(dir, kind, src, filename, declaredSize, MaxEdge)
}

// SaveMapIconAsWebP is like SaveImageAsWebP but resizes to MaxMapIconEdge so the
// uploaded image is small enough to be used as a compact map pin icon.
func SaveMapIconAsWebP(dir, kind string, src io.Reader, filename string, declaredSize int64) (publicName string, err error) {
	return saveImageAsWebPWithEdge(dir, kind, src, filename, declaredSize, MaxMapIconEdge)
}

func saveImageAsWebPWithEdge(dir, kind string, src io.Reader, filename string, declaredSize int64, maxEdge int) (publicName string, err error) {
	if declaredSize > MaxBytes {
		return "", fmt.Errorf("file terlalu besar (max 15MB)")
	}
	data, err := io.ReadAll(io.LimitReader(src, MaxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > MaxBytes {
		return "", fmt.Errorf("file terlalu besar (max 15MB)")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("file kosong")
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".heic" || ext == ".heif" {
		return "", fmt.Errorf("format HEIC/HEIF tidak didukung — simpan sebagai JPG/PNG dulu")
	}
	// SVG ditolak: file SVG boleh membawa <script>/event-handler/onload yang
	// dieksekusi saat dibuka langsung, alias Stored XSS satu origin dengan
	// aplikasi (token di localStorage ikut terancam). Pakai PNG/JPG/WebP yang
	// selalu di-decode + re-encode di bawah sehingga byte aktif tidak lolos.
	// P0-5 audit keamanan 2026-09-22.
	if ext == ".svg" || isSVG(data) {
		return "", fmt.Errorf("format SVG tidak didukung demi keamanan — pakai PNG/JPG/WebP")
	}

	if !rasterExts[ext] && !isWebP(data) {
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			return "", fmt.Errorf("tipe file tidak didukung (pakai JPG/PNG/WebP)")
		}
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("gagal baca gambar: %w", err)
	}
	img = resizeMax(img, maxEdge)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("gagal buat folder upload")
	}

	outName := kind + ".webp"
	outPath := filepath.Join(dir, outName)
	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	opts := &nativewebp.Options{CompressionLevel: nativewebp.BestCompression}
	if err := nativewebp.Encode(f, img, opts); err != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("gagal convert ke webp: %w", err)
	}
	cleanupSiblings(dir, kind, ".webp")
	return outName, nil
}

func resizeMax(img image.Image, maxEdge int) image.Image {
	if maxEdge <= 0 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || (w <= maxEdge && h <= maxEdge) {
		return img
	}
	scale := float64(maxEdge) / float64(w)
	if h > w {
		scale = float64(maxEdge) / float64(h)
	}
	nw := int(float64(w)*scale + 0.5)
	nh := int(float64(h)*scale + 0.5)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

func isWebP(data []byte) bool {
	return len(data) >= 12 &&
		string(data[0:4]) == "RIFF" &&
		string(data[8:12]) == "WEBP"
}

func isSVG(data []byte) bool {
	s := strings.TrimSpace(string(data))
	if strings.HasPrefix(s, "<?xml") {
		return strings.Contains(strings.ToLower(s[:min(512, len(s))]), "<svg")
	}
	return strings.HasPrefix(strings.ToLower(s), "<svg")
}

func cleanupSiblings(dir, kind, keepExt string) {
	for _, e := range []string{".png", ".jpg", ".jpeg", ".gif", ".ico", ".webp", ".svg"} {
		if e == keepExt {
			continue
		}
		_ = os.Remove(filepath.Join(dir, kind+e))
	}
}
