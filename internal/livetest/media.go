package livetest

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

func pngSolid(n int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	c := color.RGBA{R: 0, G: 80, B: 200, A: 255}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// 16×16: several Providers reject 1×1 (minimum 8–10px).
var png16 = pngSolid(16)

func miniPDF() []byte {
	content := "BT /F1 12 Tf 50 50 Td (ok) Tj ET"
	objs := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n",
		fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content)+1, content),
		"5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, obj := range objs {
		offs[i] = buf.Len()
		buf.WriteString(obj)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offs {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return buf.Bytes()
}

func wavSilence() []byte {
	const dataSize = 32
	buf := make([]byte, 44+dataSize)
	copy(buf[0:], []byte("RIFF"))
	binary.LittleEndian.PutUint32(buf[4:], uint32(36+dataSize))
	copy(buf[8:], []byte("WAVEfmt "))
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)
	binary.LittleEndian.PutUint16(buf[22:], 1)
	binary.LittleEndian.PutUint32(buf[24:], 8000)
	binary.LittleEndian.PutUint32(buf[28:], 16000)
	binary.LittleEndian.PutUint16(buf[32:], 2)
	binary.LittleEndian.PutUint16(buf[34:], 16)
	copy(buf[36:], []byte("data"))
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataSize))
	return buf
}

func miniMP4() []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70,
		0x69, 0x73, 0x6f, 0x6d, 0x00, 0x00, 0x00, 0x01,
		0x69, 0x73, 0x6f, 0x6d, 0x6d, 0x70, 0x34, 0x31,
		0x00, 0x00, 0x00, 0x08, 0x6d, 0x64, 0x61, 0x74,
	}
}
