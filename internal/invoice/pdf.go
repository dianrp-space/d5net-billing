package invoice

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/dianrp/drp-billing/internal/store"
)

// RenderOptions carries tenant-configurable invoice content plus bill-to details
// that are not part of the invoice row itself.
type RenderOptions struct {
	Settings        store.InvoiceSettings
	FallbackCompany string
	CustomerAddress string
	CustomerPhone   string
	CustomerEmail   string
	Logo            *pdfJPEG
}

// A4 page geometry (points).
const (
	pageW    = 595.0
	pageH    = 842.0
	marginX  = 42.0
	fontBody = "F1" // Helvetica
	fontBold = "F2" // Helvetica-Bold
)

// RenderPDF produces an ISP-style A4 invoice document.
func RenderPDF(inv *store.Invoice, items []store.InvoiceItem, opts RenderOptions) []byte {
	s := NormalizeRenderOptions(opts)
	c := &canvas{}

	company := firstNonEmpty(s.Settings.CompanyName, s.FallbackCompany, "INVOICE")
	y := pageH - 54
	textX := marginX
	logoBottom := y
	if s.Logo != nil {
		c.img = s.Logo
		lw, lh := logoDisplaySize(s.Logo)
		logoTop := pageH - 42
		logoBottom = logoTop - lh
		c.drawJPEG(marginX, logoBottom, lw, lh)
		textX = marginX + lw + 10
	}

	// ---- Header: company (left) + INVOICE badge (right) ----
	c.text(textX, y, 20, fontBold, company)
	c.textRight(pageW-marginX, y, 22, fontBold, "INVOICE")
	y -= 16
	rightY := y
	for _, ln := range companyLines(s) {
		c.text(textX, y, 9, fontBody, ln)
		y -= 12
	}
	// Right meta block (invoice no + dates + status).
	c.textRight(pageW-marginX, rightY, 10, fontBody, "No: "+inv.InvoiceNumber)
	rightY -= 13
	if inv.IssuedAt != nil {
		c.textRight(pageW-marginX, rightY, 10, fontBody, "Terbit: "+inv.IssuedAt.Format("02 Jan 2006"))
		rightY -= 13
	}
	c.textRight(pageW-marginX, rightY, 10, fontBody, "Jatuh tempo: "+inv.DueDate.Format("02 Jan 2006"))
	rightY -= 13
	c.textRight(pageW-marginX, rightY, 10, fontBold, "Status: "+strings.ToUpper(invoiceStatusLabel(inv)))

	if rightY-8 < y {
		y = rightY - 8
	}
	if s.Logo != nil && logoBottom-8 < y {
		y = logoBottom - 8
	}
	y -= 6
	c.line(marginX, y, pageW-marginX, y)
	y -= 22

	// ---- Bill to ----
	c.text(marginX, y, 9, fontBold, "DITAGIHKAN KEPADA")
	y -= 14
	c.text(marginX, y, 12, fontBold, firstNonEmpty(inv.CustomerName, "Pelanggan"))
	y -= 13
	for _, ln := range billToLines(inv, s) {
		c.text(marginX, y, 9, fontBody, ln)
		y -= 12
	}
	y -= 12

	// ---- Items table (blue outline, columns clipped to cells) ----
	tableL := marginX
	tableR := pageW - marginX
	const (
		headerH = 20.0
		rowMin  = 18.0
		padX    = 8.0
		cellFs  = 9.0
	)
	xQty := tableL + 228
	xHarga := xQty + 44
	xJumlah := xHarga + 118
	descMaxChars := int((xQty - tableL - padX*2) / (cellFs * 0.62))
	if descMaxChars < 12 {
		descMaxChars = 12
	}

	type tableRow struct {
		desc            []string
		qty, harga, amt string
		h               float64
	}
	var lateFee int64
	rows := make([]tableRow, 0, len(items))
	for _, it := range items {
		lines := splitLines(it.Description, descMaxChars)
		if len(lines) == 0 {
			lines = []string{"—"}
		}
		h := 8 + 12*float64(len(lines))
		if h < rowMin {
			h = rowMin
		}
		rows = append(rows, tableRow{
			desc:  lines,
			qty:   strconv.Itoa(it.Quantity),
			harga: rupiah(it.UnitPrice),
			amt:   rupiah(it.Amount),
			h:     h,
		})
		if isLateFeeItem(it.Description) {
			lateFee += it.Amount
		}
	}
	if len(rows) == 0 {
		rows = append(rows, tableRow{desc: []string{"—"}, h: rowMin})
	}

	tableTop := y
	tableH := headerH
	for _, r := range rows {
		tableH += r.h
	}
	tableBottom := tableTop - tableH

	c.rectFill(tableL, tableTop-headerH, tableR-tableL, headerH)
	hy := tableTop - 13
	c.textFitLeft(tableL+padX, hy, xQty-tableL-padX*2, 8, fontBold, "DESKRIPSI")
	c.textFitRight(xHarga-padX, hy, xHarga-xQty-padX*2, 8, fontBold, "QTY")
	c.textFitRight(xJumlah-padX, hy, xJumlah-xHarga-padX*2, 8, fontBold, "HARGA")
	c.textFitRight(tableR-padX, hy, tableR-xJumlah-padX*2, 8, fontBold, "JUMLAH")

	ry := tableTop - headerH
	for _, r := range rows {
		c.tableRule(tableL, ry, tableR, ry)
		textY := ry - 12
		for i, ln := range r.desc {
			c.textFitLeft(tableL+padX, textY-float64(i)*12, xQty-tableL-padX*2, cellFs, fontBody, ln)
		}
		c.textFitRight(xHarga-padX, textY, xHarga-xQty-padX*2, cellFs, fontBody, r.qty)
		c.textFitRight(xJumlah-padX, textY, xJumlah-xHarga-padX*2, cellFs, fontBody, r.harga)
		c.textFitRight(tableR-padX, textY, tableR-xJumlah-padX*2, cellFs, fontBody, r.amt)
		ry -= r.h
	}
	c.tableRule(xQty, tableBottom, xQty, tableTop)
	c.tableRule(xHarga, tableBottom, xHarga, tableTop)
	c.tableRule(xJumlah, tableBottom, xJumlah, tableTop)
	c.rectStroke(tableL, tableBottom, tableR-tableL, tableH)
	y = tableBottom - 18

	// ---- Totals (right column) ----
	totX := 360.0
	drawTotal := func(label, val string, bold bool) {
		font := fontBody
		if bold {
			font = fontBold
		}
		c.text(totX, y, 10, font, label)
		c.textRight(tableR, y, 10, font, val)
		y -= 15
	}
	drawTotal("Subtotal", rupiah(inv.Subtotal), false)
	drawTotal("Pajak", rupiah(inv.TaxAmount), false)
	if inv.DiscountAmount > 0 {
		drawTotal("Diskon", "-"+rupiah(inv.DiscountAmount), false)
	}
	if lateFee > 0 {
		drawTotal("Denda keterlambatan", rupiah(lateFee), false)
	}
	c.line(totX, y+8, tableR, y+8)
	drawTotal("TOTAL", rupiah(inv.TotalAmount), true)
	if inv.PaidAmount > 0 {
		drawTotal("Terbayar", rupiah(inv.PaidAmount), false)
		bal := inv.TotalAmount - inv.PaidAmount
		if bal < 0 {
			bal = 0
		}
		drawTotal("Sisa tagihan", rupiah(bal), true)
	}

	// ---- Payment instructions + footer (flow after totals, not pinned to A4 bottom) ----
	y -= 10
	if pay := s.Settings.PaymentInstructions; pay != "" {
		c.text(marginX, y, 9, fontBold, "CARA PEMBAYARAN")
		y -= 14
		for _, ln := range splitLines(pay, 90) {
			c.text(marginX, y, 9, fontBody, ln)
			y -= 12
		}
		y -= 6
	}
	if note := s.Settings.FooterNote; note != "" {
		c.line(marginX, y+4, pageW-marginX, y+4)
		y -= 10
		for _, ln := range splitLines(note, 100) {
			c.text(marginX, y, 8.5, fontBody, ln)
			y -= 11
		}
	}

	const bottomPad = 36.0
	mediaBottom := y - bottomPad
	if mediaBottom < 0 {
		mediaBottom = 0
	}
	c.mediaBottom = mediaBottom

	if invoiceIsPaid(inv) {
		c.paidWatermark(pageW/2, (pageH+mediaBottom)/2, pageH-mediaBottom)
	}

	return c.build()
}

// NormalizeRenderOptions trims option strings.
func NormalizeRenderOptions(o RenderOptions) RenderOptions {
	o.Settings = store.NormalizeInvoiceSettings(o.Settings)
	o.FallbackCompany = strings.TrimSpace(o.FallbackCompany)
	o.CustomerAddress = strings.TrimSpace(o.CustomerAddress)
	o.CustomerPhone = strings.TrimSpace(o.CustomerPhone)
	o.CustomerEmail = strings.TrimSpace(o.CustomerEmail)
	return o
}

func companyLines(s RenderOptions) []string {
	var out []string
	for _, ln := range splitMultiline(s.Settings.Address) {
		out = append(out, ln)
	}
	var contact []string
	if s.Settings.Phone != "" {
		contact = append(contact, "Telp: "+s.Settings.Phone)
	}
	if s.Settings.Email != "" {
		contact = append(contact, s.Settings.Email)
	}
	if len(contact) > 0 {
		out = append(out, strings.Join(contact, "  ·  "))
	}
	if s.Settings.Website != "" {
		out = append(out, s.Settings.Website)
	}
	if s.Settings.TaxID != "" {
		out = append(out, "NPWP: "+s.Settings.TaxID)
	}
	return out
}

func billToLines(inv *store.Invoice, s RenderOptions) []string {
	var out []string
	if inv.CustomerCode != "" {
		out = append(out, "Kode: "+inv.CustomerCode)
	}
	for _, ln := range splitMultiline(s.CustomerAddress) {
		out = append(out, ln)
	}
	var contact []string
	if s.CustomerPhone != "" {
		contact = append(contact, "Telp: "+s.CustomerPhone)
	}
	if s.CustomerEmail != "" {
		contact = append(contact, s.CustomerEmail)
	}
	if len(contact) > 0 {
		out = append(out, strings.Join(contact, "  ·  "))
	}
	return out
}

func invoiceStatusLabel(inv *store.Invoice) string {
	switch strings.ToLower(inv.Status) {
	case "paid":
		return "Lunas"
	case "partial":
		return "Sebagian"
	case "overdue":
		return "Terlambat"
	case "void", "cancelled", "canceled":
		return "Batal"
	case "issued":
		return "Belum dibayar"
	default:
		if inv.Status == "" {
			return "Belum dibayar"
		}
		return inv.Status
	}
}

func invoiceIsPaid(inv *store.Invoice) bool {
	if inv == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(inv.Status), "paid") {
		return true
	}
	return inv.TotalAmount > 0 && inv.PaidAmount >= inv.TotalAmount
}

func isLateFeeItem(desc string) bool {
	d := strings.ToLower(desc)
	return strings.Contains(d, "denda") || strings.Contains(d, "late") || strings.Contains(d, "keterlambatan")
}

// ---- number / string helpers ----

func rupiah(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	out := "Rp " + b.String()
	if neg {
		out = "-" + out
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func splitMultiline(s string) []string {
	var out []string
	for _, raw := range strings.Split(s, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		out = append(out, splitLines(raw, 90)...)
	}
	return out
}

// splitLines wraps text to at most n characters per line on word boundaries.
func splitLines(s string, n int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		cur := ""
		for _, w := range words {
			if cur == "" {
				cur = w
			} else if len(cur)+1+len(w) <= n {
				cur += " " + w
			} else {
				out = append(out, cur)
				cur = w
			}
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return out
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// ---- minimal PDF canvas (Helvetica) ----

type canvas struct {
	buf         bytes.Buffer
	img         *pdfJPEG
	watermark   bool
	mediaBottom float64
}

// approxWidth estimates string width for right-alignment (Helvetica ~0.5em avg).
func approxWidth(s string, size float64) float64 {
	// Helvetica is closer to 0.6em; 0.5 under-estimated and let text overlap column rules.
	return float64(len([]rune(s))) * size * 0.62
}

func (c *canvas) text(x, y, size float64, font, s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	fmt.Fprintf(&c.buf, "BT /%s %.1f Tf %.1f %.1f Td %s ET\n", font, size, x, y, pdfEscape(s))
}

func (c *canvas) textRight(xRight, y, size float64, font, s string) {
	c.text(xRight-approxWidth(s, size), y, size, font, s)
}

func fitText(s string, size, maxW float64) string {
	if maxW <= 0 || approxWidth(s, size) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && approxWidth(string(r), size) > maxW {
		r = r[:len(r)-1]
	}
	return string(r)
}

func (c *canvas) textFitLeft(x, y, maxW, size float64, font, s string) {
	c.text(x, y, size, font, fitText(s, size, maxW))
}

func (c *canvas) textFitRight(xRight, y, maxW, size float64, font, s string) {
	c.textRight(xRight, y, size, font, fitText(s, size, maxW))
}

const tableBlue = "0.32 0.48 0.72"

func (c *canvas) line(x1, y1, x2, y2 float64) {
	fmt.Fprintf(&c.buf, "0.75 w 0.72 0.72 0.72 RG %.1f %.1f m %.1f %.1f l S\n", x1, y1, x2, y2)
}

func (c *canvas) tableRule(x1, y1, x2, y2 float64) {
	fmt.Fprintf(&c.buf, "0.8 w %s RG %.1f %.1f m %.1f %.1f l S 0 0 0 rg\n", tableBlue, x1, y1, x2, y2)
}

func (c *canvas) rectFill(x, y, w, h float64) {
	fmt.Fprintf(&c.buf, "0.93 0.93 0.90 rg %.1f %.1f %.1f %.1f re f 0 0 0 rg\n", x, y, w, h)
}

func (c *canvas) rectStroke(x, y, w, h float64) {
	fmt.Fprintf(&c.buf, "1 w %s RG %.1f %.1f %.1f %.1f re S 0 0 0 rg\n", tableBlue, x, y, w, h)
}

func (c *canvas) drawJPEG(x, y, w, h float64) {
	if c.img == nil || w <= 0 || h <= 0 {
		return
	}
	fmt.Fprintf(&c.buf, "q %.2f 0 0 %.2f %.2f %.2f cm /Im1 Do Q\n", w, h, x, y)
}

// paidWatermark draws a diagonal "LUNAS" stamp across the invoice body (not the empty A4).
func (c *canvas) paidWatermark(cx, cy, contentH float64) {
	c.watermark = true
	size := 72.0
	if contentH > 0 && contentH < 520 {
		size = contentH * 0.16
		if size < 42 {
			size = 42
		}
		if size > 72 {
			size = 72
		}
	}
	const (
		cos = 0.81915204 // 35°
		sin = 0.57357644
	)
	// Offset roughly half the word width so it sits on the content, not the page void.
	off := size * 2.15
	fmt.Fprintf(&c.buf,
		"q /GS1 gs 0.70 0.22 0.18 rg 1 0 0 1 %.1f %.1f cm %.5f %.5f %.5f %.5f 0 0 cm BT /F2 %.1f Tf %.1f -20 Td (LUNAS) Tj ET Q\n",
		cx, cy, cos, sin, -sin, cos, size, -off,
	)
}

func jpegXObject(img *pdfJPEG) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", img.Width, img.Height, len(img.Data))
	b.Write(img.Data)
	b.WriteString("\nendstream")
	return b.Bytes()
}

func (c *canvas) build() []byte {
	stream := c.buf.String()
	fontRes := "/Font << /F1 5 0 R /F2 6 0 R >>"
	xobj := ""
	gs := ""
	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		nil, // page filled below
		[]byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream)),
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>"),
	}
	next := 7
	if c.img != nil && len(c.img.Data) > 0 {
		xobj = fmt.Sprintf(" /XObject << /Im1 %d 0 R >>", next)
		objects = append(objects, jpegXObject(c.img))
		next++
	}
	if c.watermark {
		gs = fmt.Sprintf(" /ExtGState << /GS1 %d 0 R >>", next)
		objects = append(objects, []byte("<< /Type /ExtGState /ca 0.12 /CA 0.12 >>"))
	}
	resources := fmt.Sprintf("<< %s%s%s >>", fontRes, xobj, gs)
	mb := c.mediaBottom
	if mb < 0 {
		mb = 0
	}
	if mb > pageH-200 {
		mb = pageH - 200
	}
	objects[2] = []byte(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 %.1f 595 842] /CropBox [0 %.1f 595 842] /Contents 4 0 R /Resources %s >>", mb, mb, resources))

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	offsets[0] = pdf.Len()
	for i, obj := range objects {
		offsets[i+1] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n", i+1)
		pdf.Write(obj)
		pdf.WriteString("\nendobj\n")
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&pdf, "trailer << /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return pdf.Bytes()
}

func pdfEscape(s string) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, r := range s {
		switch r {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			if r < 32 || r > 126 {
				// Best-effort transliteration for common non-ASCII glyphs.
				switch r {
				case '·':
					b.WriteString("- ")
				case '…':
					b.WriteString("...")
				default:
					b.WriteByte(' ')
				}
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte(')')
	b.WriteString(" Tj")
	return b.String()
}
