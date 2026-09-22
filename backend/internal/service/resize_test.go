package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestResizeAspect(t *testing.T) {
	// Кадр 1920x1080 — как у .135 и .5
	src := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for x := 0; x < 1920; x++ {
		for y := 0; y < 1080; y++ {
			src.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, nil); err != nil {
		t.Fatal(err)
	}
	out, err := resizeJPEG(buf.Bytes(), 480)
	if err != nil {
		t.Fatalf("resizeJPEG: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	b := img.Bounds()
	t.Logf("получено %dx%d (%d байт)", b.Dx(), b.Dy(), len(out))
	if b.Dx() != 480 {
		t.Errorf("ширина %d, ожидалось 480", b.Dx())
	}
	if b.Dy() != 270 {
		t.Errorf("высота %d, ожидалось 270", b.Dy())
	}
}
