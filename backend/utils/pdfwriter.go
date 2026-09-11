package utils

// pdfwriter.go implements a tiny, dependency-free PDF writer used to
// generate the two printable leave forms ("Surat Rekomendasi Izin Cuti" and
// "Formulir Permintaan dan Pemberian Cuti"). It only supports the small
// subset of PDF needed for those two documents (text with the standard
// Helvetica/Helvetica-Bold fonts, lines, rectangles, and one embedded raster
// image for the instansi logo) -- it is intentionally not a general-purpose
// PDF library. Kept dependency-free (stdlib only) because this environment
// cannot fetch new third-party Go modules.

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image/png"
	"sort"
	"strings"
)

// Page sizes in points (72pt = 1 inch).
const (
	PageWidthA4  = 595.28
	PageHeightA4 = 841.89

	// US Legal (8.5in x 14in) -- used for the "Formulir Permintaan dan
	// Pemberian Cuti", which is traditionally printed on legal/folio paper.
	PageWidthLegal  = 612.0
	PageHeightLegal = 1008.0
)

// ---------------------------------------------------------------------------
// Approximate Helvetica glyph metrics -- only accurate enough to make
// reasonable word-wrap/centering decisions in the forms below, not for exact
// typesetting.
// ---------------------------------------------------------------------------

func helveticaWidth(r rune) float64 {
	switch r {
	case ' ', '!', ',', '.', ':', ';', '\'', 'i', 'j', 'l', '|':
		return 222
	case 'f', 't', 'I':
		return 278
	case 'r':
		return 333
	case '-':
		return 333
	case 'm', 'w', 'M', 'W', '@':
		return 833
	}
	switch {
	case r >= '0' && r <= '9':
		return 556
	case r >= 'A' && r <= 'Z':
		return 667
	case r >= 'a' && r <= 'z':
		return 556
	}
	return 556
}

// TextWidth returns the approximate rendered width (in points) of s set in
// Helvetica at the given size.
func TextWidth(s string, size float64) float64 {
	w := 0.0
	for _, r := range s {
		w += helveticaWidth(r)
	}
	return w / 1000 * size
}

// WrapText breaks s into lines that each fit within maxWidth at the given
// font size, breaking on word boundaries.
func WrapText(s string, maxWidth, size float64) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		trial := cur + " " + w
		if TextWidth(trial, size) > maxWidth {
			lines = append(lines, cur)
			cur = w
		} else {
			cur = trial
		}
	}
	lines = append(lines, cur)
	return lines
}

func pdfEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `(`, `\(`)
	s = strings.ReplaceAll(s, `)`, `\)`)
	return s
}

// ---------------------------------------------------------------------------
// Page: a single page's content stream. All coordinates passed to its
// drawing methods are measured from the TOP-LEFT corner (like a normal page
// ruler) -- more natural for laying out a printed form -- and flipped to
// PDF's native bottom-left origin internally.
// ---------------------------------------------------------------------------

// pdfLink adalah satu area klik pada halaman yang membuka URL (annotation
// /Link pada PDF) -- dipakai mis. untuk membuat titik koordinat absen bisa
// langsung diklik dan terbuka di Google Maps.
type pdfLink struct {
	x, yTop, w, h float64
	url           string
}

type PDFPage struct {
	W, H  float64
	buf   bytes.Buffer
	size  float64
	links []pdfLink
}

func NewPDFPage(w, h float64) *PDFPage {
	p := &PDFPage{W: w, H: h}
	p.SetLineWidth(0.75)
	p.SetFont(false, 10)
	return p
}

func (p *PDFPage) SetFont(bold bool, size float64) {
	name := "F1"
	if bold {
		name = "F2"
	}
	p.size = size
	fmt.Fprintf(&p.buf, "/%s %.2f Tf\n", name, size)
}

func (p *PDFPage) SetLineWidth(w float64) {
	fmt.Fprintf(&p.buf, "%.2f w\n", w)
}

// Text draws a single line of text with its baseline at (x, yTop).
func (p *PDFPage) Text(x, yTop float64, s string) {
	if s == "" {
		return
	}
	y := p.H - yTop
	fmt.Fprintf(&p.buf, "BT %.2f %.2f Td (%s) Tj ET\n", x, y, pdfEscape(s))
}

func (p *PDFPage) TextCentered(xCenter, yTop float64, s string) {
	w := TextWidth(s, p.size)
	p.Text(xCenter-w/2, yTop, s)
}

func (p *PDFPage) TextRight(xRight, yTop float64, s string) {
	w := TextWidth(s, p.size)
	p.Text(xRight-w, yTop, s)
}

// MultilineText wraps s to fit maxWidth and draws each line, returning the
// yTop position just below the last line drawn.
func (p *PDFPage) MultilineText(x, yTop, maxWidth, lineHeight float64, s string) float64 {
	y := yTop
	for _, line := range WrapText(s, maxWidth, p.size) {
		p.Text(x, y, line)
		y += lineHeight
	}
	return y
}

func (p *PDFPage) Line(x1, y1Top, x2, y2Top float64) {
	fmt.Fprintf(&p.buf, "%.2f %.2f m %.2f %.2f l S\n", x1, p.H-y1Top, x2, p.H-y2Top)
}

// Rect strokes an (unfilled) rectangle whose top-left corner is (x, yTop).
func (p *PDFPage) Rect(x, yTop, w, h float64) {
	fmt.Fprintf(&p.buf, "%.2f %.2f %.2f %.2f re S\n", x, p.H-yTop-h, w, h)
}

// Cross draws a small "X" mark (used to tick a checkbox) inside a box whose
// top-left corner is (x, yTop) and side length is size.
func (p *PDFPage) Cross(x, yTop, size float64) {
	p.Line(x, yTop, x+size, yTop+size)
	p.Line(x, yTop+size, x+size, yTop)
}

// Checkmark draws a small "✓" tick mark (used to tick a checkbox on the
// printed forms, which use a checkmark rather than an X) inside a box whose
// top-left corner is (x, yTop) and side length is size.
func (p *PDFPage) Checkmark(x, yTop, size float64) {
	p.Line(x+size*0.12, yTop+size*0.55, x+size*0.4, yTop+size*0.85)
	p.Line(x+size*0.4, yTop+size*0.85, x+size*0.92, yTop+size*0.12)
}

// Link marks a rectangular area (top-left corner at (x, yTop)) as clickable:
// membuka url saat diklik di pembaca PDF. Tidak menggambar apa pun -- teks/
// garis bawahnya digambar sendiri oleh pemanggil.
func (p *PDFPage) Link(x, yTop, w, h float64, url string) {
	if url == "" || w <= 0 || h <= 0 {
		return
	}
	p.links = append(p.links, pdfLink{x: x, yTop: yTop, w: w, h: h, url: url})
}

// Image draws a previously registered (via PDFDoc.RegisterImage) image by
// name, top-left corner at (x, yTop).
func (p *PDFPage) Image(name string, x, yTop, w, h float64) {
	y := p.H - yTop - h
	fmt.Fprintf(&p.buf, "q %.2f 0 0 %.2f %.2f %.2f cm /%s Do Q\n", w, h, x, y, name)
}

// ---------------------------------------------------------------------------
// Doc: the whole PDF file (one or more pages + shared resources).
// ---------------------------------------------------------------------------

type pdfImage struct {
	w, h int
	data []byte // zlib/FlateDecode-compressed raw 8-bit RGB pixel data
}

type PDFDoc struct {
	pages []*PDFPage
	imgs  map[string]*pdfImage
}

func NewPDFDoc() *PDFDoc {
	return &PDFDoc{imgs: map[string]*pdfImage{}}
}

func (d *PDFDoc) AddPage(p *PDFPage) {
	d.pages = append(d.pages, p)
}

// RegisterImage decodes a PNG (flattening any transparency onto a white
// background, since these forms are meant to be printed on white paper) and
// stores it under name for later use with PDFPage.Image.
func (d *PDFDoc) RegisterImage(name string, pngBytes []byte) error {
	pix, w, h, err := decodePNGFlattenWhite(pngBytes)
	if err != nil {
		return err
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(pix); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	d.imgs[name] = &pdfImage{w: w, h: h, data: zbuf.Bytes()}
	return nil
}

func decodePNGFlattenWhite(data []byte) (pix []byte, w, h int, err error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	bounds := img.Bounds()
	w, h = bounds.Dx(), bounds.Dy()
	pix = make([]byte, 0, w*h*3)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// image/color.Color.RGBA() returns alpha-premultiplied 16-bit
			// components; compositing onto opaque white simplifies to
			// premultiplied + (0xffff - alpha), then downscaled to 8-bit.
			r, g, b, a := img.At(x, y).RGBA()
			outR := r + (0xffff - a)
			outG := g + (0xffff - a)
			outB := b + (0xffff - a)
			pix = append(pix, byte(outR>>8), byte(outG>>8), byte(outB>>8))
		}
	}
	return pix, w, h, nil
}

func pdfRef(id int) string { return fmt.Sprintf("%d 0 R", id) }

// Output serializes the whole document (all pages + shared resources) into a
// valid PDF byte stream.
func (d *PDFDoc) Output() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	nextID := 1
	alloc := func() int { id := nextID; nextID++; return id }

	catalogID := alloc()
	pagesID := alloc()
	fontRegID := alloc()
	fontBoldID := alloc()

	imageNames := make([]string, 0, len(d.imgs))
	for name := range d.imgs {
		imageNames = append(imageNames, name)
	}
	sort.Strings(imageNames)
	imageIDs := map[string]int{}
	for _, name := range imageNames {
		imageIDs[name] = alloc()
	}

	type pageIDs struct {
		page, content int
		annots        []int // satu objek annotation per link pada halaman itu
	}
	pIDs := make([]pageIDs, len(d.pages))
	for i, page := range d.pages {
		ids := pageIDs{page: alloc(), content: alloc()}
		for range page.links {
			ids.annots = append(ids.annots, alloc())
		}
		pIDs[i] = ids
	}

	offsets := make([]int, nextID)

	writeObj := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}
	writeStreamObj := func(id int, dict string, data []byte) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nstream\n", id, dict)
		buf.Write(data)
		buf.WriteString("\nendstream\nendobj\n")
	}

	var res bytes.Buffer
	fmt.Fprintf(&res, "<< /Font << /F1 %s /F2 %s >>", pdfRef(fontRegID), pdfRef(fontBoldID))
	if len(imageNames) > 0 {
		res.WriteString(" /XObject <<")
		for _, name := range imageNames {
			fmt.Fprintf(&res, " /%s %s", name, pdfRef(imageIDs[name]))
		}
		res.WriteString(" >>")
	}
	res.WriteString(" >>")

	var kids bytes.Buffer
	kids.WriteString("[")
	for i, ids := range pIDs {
		if i > 0 {
			kids.WriteString(" ")
		}
		kids.WriteString(pdfRef(ids.page))
	}
	kids.WriteString("]")

	writeObj(catalogID, fmt.Sprintf("<< /Type /Catalog /Pages %s >>", pdfRef(pagesID)))
	writeObj(pagesID, fmt.Sprintf("<< /Type /Pages /Kids %s /Count %d /Resources %s >>", kids.String(), len(d.pages), res.String()))
	writeObj(fontRegID, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	writeObj(fontBoldID, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")

	for _, name := range imageNames {
		img := d.imgs[name]
		dict := fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>", img.w, img.h, len(img.data))
		writeStreamObj(imageIDs[name], dict, img.data)
	}

	for i, page := range d.pages {
		ids := pIDs[i]
		content := page.buf.Bytes()

		// daftar area klik (kalau ada) dilampirkan ke halaman lewat /Annots
		annots := ""
		if len(ids.annots) > 0 {
			var ab bytes.Buffer
			ab.WriteString(" /Annots [")
			for j, id := range ids.annots {
				if j > 0 {
					ab.WriteString(" ")
				}
				ab.WriteString(pdfRef(id))
			}
			ab.WriteString("]")
			annots = ab.String()
		}

		writeObj(ids.page, fmt.Sprintf("<< /Type /Page /Parent %s /MediaBox [0 0 %.2f %.2f] /Contents %s%s >>", pdfRef(pagesID), page.W, page.H, pdfRef(ids.content), annots))
		writeStreamObj(ids.content, fmt.Sprintf("<< /Length %d >>", len(content)), content)

		for j, lk := range page.links {
			// koordinat annotation memakai titik asal kiri-BAWAH seperti PDF
			// aslinya, jadi yTop dibalik dulu (lihat komentar pada PDFPage).
			y2 := page.H - lk.yTop
			y1 := y2 - lk.h
			writeObj(ids.annots[j], fmt.Sprintf(
				"<< /Type /Annot /Subtype /Link /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0] /F 4 /A << /S /URI /URI (%s) >> >>",
				lk.x, y1, lk.x+lk.w, y2, pdfEscape(lk.url)))
		}
	}

	xrefStart := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", nextID)
	for id := 1; id < nextID; id++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[id])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %s >>\nstartxref\n%d\n%%%%EOF", nextID, pdfRef(catalogID), xrefStart)

	return buf.Bytes(), nil
}
