package upload

import (
	"bytes"
	"image"
	"image/color"
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
	// leftover old file should be cleaned
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

func TestSaveImageAsWebP_KeepsWebP(t *testing.T) {
	dir := t.TempDir()
	// minimal valid-looking webp header + junk body is enough for isWebP path (we don't re-validate encode)
	raw := []byte("RIFF....WEBP")
	copy(raw[4:8], []byte{8, 0, 0, 0})
	name, err := SaveImageAsWebP(dir, "favicon", bytes.NewReader(raw), "icon.webp", int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if name != "favicon.webp" {
		t.Fatalf("got %q", name)
	}
	got, _ := os.ReadFile(filepath.Join(dir, name))
	if !bytes.Equal(got, raw) {
		t.Fatal("webp should be stored verbatim")
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
