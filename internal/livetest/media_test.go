package livetest

import (
	"bytes"
	"fmt"
	"image/png"
	"testing"
)

func TestPNG16Decodes(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(png16))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() < 16 || b.Dy() < 16 {
		t.Fatalf("got %dx%d", b.Dx(), b.Dy())
	}
}

func TestMiniPDFIsOpenable(t *testing.T) {
	raw := miniPDF()
	if !bytes.HasPrefix(raw, []byte("%PDF-")) {
		t.Fatal("missing %PDF- header")
	}
	if !bytes.Contains(raw, []byte("%%EOF")) {
		t.Fatal("missing PDF EOF trailer")
	}
	i := bytes.LastIndex(raw, []byte("startxref"))
	if i < 0 {
		t.Fatal("missing startxref")
	}
	var off int
	if _, err := fmt.Sscanf(string(bytes.TrimSpace(raw[i+len("startxref"):])), "%d", &off); err != nil {
		t.Fatalf("startxref: %v", err)
	}
	if off < 0 || off >= len(raw) || !bytes.HasPrefix(raw[off:], []byte("xref")) {
		t.Fatalf("startxref %d does not point at xref (len %d)", off, len(raw))
	}
}
