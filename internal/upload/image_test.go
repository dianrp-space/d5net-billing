package upload

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveImageAsWebP_ConvertsPNG(t *testing.T) {
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "logo.png"), []byte("x"), 0o644)

	name, err := SaveImageAsWebP(dir, "logo", bytes.NewReader(buf.Bytes()), "photo.png", int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if name != "logo.webp" {
		t.Fatalf("got %q", name)
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if !isWebP(data) {
		t.Fatal("not webp")
	}
	if _, err := os.Stat(filepath.Join(dir, "logo.png")); !os.IsNotExist(err) {
		t.Fatal("old png should be removed")
	}
}

func TestSaveImageAsWebP_ReencodesWebP(t *testing.T) {
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.NRGBA{G: 200, A: 255})
		}
	}
	// Round-trip via PNG then Save — ensures decode path works for re-encode.
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	name, err := SaveImageAsWebP(dir, "favicon", bytes.NewReader(pngBuf.Bytes()), "icon.png", int64(pngBuf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if !isWebP(data) {
		t.Fatal("expected webp output")
	}
	// Feed webp back — must re-encode successfully (not store junk verbatim).
	name2, err := SaveImageAsWebP(dir, "favicon2", bytes.NewReader(data), "icon.webp", int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, name2))
	if err != nil {
		t.Fatal(err)
	}
	if !isWebP(out) {
		t.Fatal("re-encoded webp invalid")
	}
}

func TestSaveImageAsWebP_ResizesLarge(t *testing.T) {
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 3200, 2000))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	name, err := SaveImageAsWebP(dir, "big", bytes.NewReader(buf.Bytes()), "big.jpg", int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width > MaxEdge || cfg.Height > MaxEdge {
		t.Fatalf("expected max edge %d, got %dx%d", MaxEdge, cfg.Width, cfg.Height)
	}
}

func TestSaveImageAsWebP_KeepsSVG(t *testing.T) {
	dir := t.TempDir()
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"></svg>`)
	name, err := SaveImageAsWebP(dir, "logo", bytes.NewReader(svg), "mark.svg", int64(len(svg)))
	if err != nil {
		t.Fatal(err)
	}
	if name != "logo.svg" {
		t.Fatalf("got %q", name)
	}
}

func TestSaveImageAsWebP_RejectsHEIC(t *testing.T) {
	dir := t.TempDir()
	_, err := SaveImageAsWebP(dir, "x", bytes.NewReader([]byte("fake")), "photo.heic", 4)
	if err == nil {
		t.Fatal("expected heic error")
	}
}
