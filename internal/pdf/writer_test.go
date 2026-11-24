package pdf

import (
	"bytes"
	"image"
	"image/jpeg"
	"strings"
	"testing"
)

func TestImagePDF(t *testing.T) {
	var j bytes.Buffer
	if err := jpeg.Encode(&j, image.NewRGBA(image.Rect(0, 0, 32, 18)), nil); err != nil {
		t.Fatal(err)
	}
	out, err := imagePDF([][]byte{j.Bytes(), j.Bytes()}, 1200, 675)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "%PDF-1.4") || !strings.Contains(s, "/Count 2") || strings.Count(s, "/Subtype /Image") != 2 || !strings.HasSuffix(s, "%%EOF\n") {
		t.Fatalf("bad PDF: %.200q", s)
	}
}
