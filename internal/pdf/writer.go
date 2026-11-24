package pdf

import (
	"bytes"
	"fmt"
	"image/jpeg"
)

// imagePDF puts one JPEG per page, edge to edge, on pages of w x h points.
func imagePDF(pages [][]byte, w, h float64) ([]byte, error) {
	var buf bytes.Buffer
	var offsets []int
	obj := func(body func()) {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n", len(offsets))
		body()
		buf.WriteString("\nendobj\n")
	}
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	n := len(pages)
	// 1 catalog, 2 pages, then per page: page, content, image
	obj(func() { buf.WriteString("<< /Type /Catalog /Pages 2 0 R >>") })
	obj(func() {
		buf.WriteString("<< /Type /Pages /Kids [")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&buf, " %d 0 R", 3+i*3)
		}
		fmt.Fprintf(&buf, " ] /Count %d >>", n)
	})
	for i, img := range pages {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(img))
		if err != nil {
			return nil, fmt.Errorf("slide image %d: %w", i, err)
		}
		page, content, xobj := 3+i*3, 4+i*3, 5+i*3
		_ = page
		obj(func() {
			fmt.Fprintf(&buf, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Contents %d 0 R /Resources << /XObject << /S %d 0 R >> >> >>", w, h, content, xobj)
		})
		draw := fmt.Sprintf("q %.2f 0 0 %.2f 0 0 cm /S Do Q", w, h)
		obj(func() { fmt.Fprintf(&buf, "<< /Length %d >>\nstream\n%s\nendstream", len(draw), draw) })
		obj(func() {
			fmt.Fprintf(&buf, "<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", cfg.Width, cfg.Height, len(img))
			buf.Write(img)
			buf.WriteString("\nendstream")
		})
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return buf.Bytes(), nil
}
