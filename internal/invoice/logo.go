package invoice

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	logoMaxH    = 48.0
	logoMaxW    = 80.0
	logoJPEGMax = 400
)

type pdfJPEG struct {
	Data   []byte
	Width  int
	Height int
}

// LoadLogoJPEG reads a local /uploads path under uploadDir and converts it to
// JPEG suitable for embedding in the invoice PDF. SVG and unreadable files
// return nil (the invoice still renders without a logo).
func LoadLogoJPEG(uploadDir, logoURL string) *pdfJPEG {
	raw, err := readUploadFile(uploadDir, logoURL)
	if err != nil || len(raw) == 0 {
		return nil
	}
	return EncodeLogoJPEG(raw)
}

func readUploadFile(uploadDir, logoURL string) ([]byte, error) {
	u := strings.TrimSpace(logoURL)
	if u == "" || strings.Contains(u, "://") {
		return nil, os.ErrNotExist
	}
	rel := strings.TrimPrefix(u, "/")
	rel = strings.TrimPrefix(rel, "uploads/")
	rel = filepath.Clean(filepath.FromSlash(rel))
	if rel == "." || rel == string(filepath.Separator) || strings.HasPrefix(rel, "..") {
		return nil, os.ErrNotExist
	}
	root, err := filepath.Abs(uploadDir)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(root, rel)
	abs, err := filepath.Abs(full)
	if err != nil {
		return nil, err
	}
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(abs)
}

// EncodeLogoJPEG decodes PNG/JPEG/WebP/GIF, composites onto white, and
// re-encodes as JPEG. Returns nil if the bytes are not a raster image.
func EncodeLogoJPEG(raw []byte) *pdfJPEG {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return nil
	}
	if w > logoJPEGMax || h > logoJPEGMax {
		scale := float64(logoJPEGMax) / float64(w)
		if h > w {
			scale = float64(logoJPEGMax) / float64(h)
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
		draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
		img = dst
		w, h = nw, nh
	} else {
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Over)
		img = dst
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil
	}
	return &pdfJPEG{Data: buf.Bytes(), Width: w, Height: h}
}

func logoDisplaySize(img *pdfJPEG) (w, h float64) {
	if img == nil || img.Width < 1 || img.Height < 1 {
		return 0, 0
	}
	h = logoMaxH
	w = h * float64(img.Width) / float64(img.Height)
	if w > logoMaxW {
		w = logoMaxW
		h = w * float64(img.Height) / float64(img.Width)
	}
	return w, h
}
