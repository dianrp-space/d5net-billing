package invoice

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/dianrp-space/d5net-billing/internal/store"
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
	// AdminFee: MDR yang dibebankan ke pelanggan saat bayar online (0 = sembunyikan).
	// Ditampilkan terpisah agar TOTAL tagihan ISP tetap jelas, tapi nominal bayar
	// di gateway sama dengan yang dilihat customer.
	AdminFee int64
}

// A4 page geometry (points).
const (
	pageW    = 595.0
	pageH    = 842.0
	marginX  = 40.0
	fontBody = "F1" // Helvetica
	fontBold = "F2" // Helvetica-Bold
)

const (
	colInk    = "0.14 0.14 0.12"
	colMuted  = "0.42 0.42 0.37"
	colOlive  = "0.353 0.353 0.251"
	colLine   = "0.82 0.81 0.78"
	colFill   = "0.969 0.965 0.949"
	colHead   = "0.353 0.353 0.251"
	colWhite  = "1 1 1"
	colStamp  = "0.12 0.25 0.69"
	colDanger = "0.55 0.20 0.16"
)

// RenderPDF produces an ISP-style A4 invoice document.
func RenderPDF(inv *store.Invoice, items []store.InvoiceItem, opts RenderOptions) []byte {
	s := NormalizeRenderOptions(opts)
	c := &canvas{}

	company := firstNonEmpty(s.Settings.CompanyName, s.FallbackCompany, "INVOICE")
	tableL := marginX
	tableR := pageW - marginX
	contentW := tableR - tableL

	const (
		metaW   = 248.0
		metaGap = 16.0
		labelW  = 74.0
		headerH = 22.0
		rowMin  = 20.0
		padX    = 8.0
		cellFs  = 9.0
		totalsW = 230.0
	)
	metaX := tableR - metaW
	leftW := metaX - tableL - metaGap
	if leftW < 180 {
		leftW = 180
	}

	// Top accent bar.
	c.rectFill(0, pageH-6, pageW, 6, colOlive)

	y := pageH - 28
	textX := tableL
	logoBottom := y
	if s.Logo != nil {
		c.img = s.Logo
		lw, lh := logoDisplaySize(s.Logo)
		logoTop := pageH - 22
		logoBottom = logoTop - lh
		c.drawJPEG(tableL, logoBottom, lw, lh)
		textX = tableL + lw + 12
		leftW -= lw + 12
		if leftW < 140 {
			leftW = 140
		}
	}

	nameMax := leftW
	if textX+nameMax > metaX-8 {
		nameMax = metaX - 8 - textX
	}
	nameLines := wrapToWidth(company, 15, nameMax, true)
	if len(nameLines) == 0 {
		nameLines = []string{company}
	}
	c.text(textX, y, 15, fontBold, nameLines[0], colInk)
	for i, ln := range nameLines[1:] {
		c.text(textX, y-14-float64(i)*13, 11, fontBold, ln, colInk)
	}
	y = y - 14 - float64(max(0, len(nameLines)-1))*13

	addrY := y
	for _, ln := range companyLines(s) {
		c.textFitLeft(textX, addrY, nameMax, 8.5, fontBody, ln, colMuted)
		addrY -= 11
	}

	// Right meta: title + aligned label/value rows (not a pile of right-aligned strings).
	rightY := pageH - 28
	c.textRight(tableR, rightY, 16, fontBold, "INVOICE", colOlive)
	rightY -= 10
	c.line(metaX, rightY, tableR, rightY, colOlive, 1.2)
	rightY -= 16
	valX := metaX + labelW + 6
	valW := tableR - valX
	drawMeta := func(label, value string, emphasis bool) {
		c.textFitLeft(metaX, rightY, labelW, 8.5, fontBody, label, colMuted)
		font := fontBody
		color := colInk
		if emphasis {
			font = fontBold
			if invoiceIsPaid(inv) {
				color = colOlive
			} else if strings.EqualFold(invoiceStatusLabel(inv), "Terlambat") {
				color = colDanger
			}
		}
		valLines := wrapToWidth(value, 8.5, valW, emphasis)
		if len(valLines) == 0 {
			valLines = []string{value}
		}
		for i, ln := range valLines {
			c.textFitLeft(valX, rightY-float64(i)*11, valW, 8.5, font, ln, color)
		}
		rightY -= 13 + float64(len(valLines)-1)*11
	}
	drawMeta("Nomor", inv.InvoiceNumber, false)
	if inv.IssuedAt != nil {
		drawMeta("Terbit", inv.IssuedAt.Format("02 Jan 2006"), false)
	}
	drawMeta("Jatuh tempo", inv.DueDate.Format("02 Jan 2006"), false)
	drawMeta("Status", strings.ToUpper(invoiceStatusLabel(inv)), true)

	y = minFloat(addrY, rightY, logoBottom) - 14
	c.line(tableL, y, tableR, y, colLine, 0.7)
	y -= 20

	// Bill to
	c.text(tableL, y, 8, fontBold, "DITAGIHKAN KEPADA", colOlive)
	y -= 14
	c.text(tableL, y, 12, fontBold, firstNonEmpty(inv.CustomerName, "Pelanggan"), colInk)
	y -= 13
	for _, ln := range billToLines(inv, s) {
		c.text(tableL, y, 9, fontBody, ln, colMuted)
		y -= 12
	}
	y -= 14

	// Items table
	xQty := tableL + 258
	xHarga := xQty + 48
	xJumlah := xHarga + 104
	descW := xQty - tableL - padX*2

	type tableRow struct {
		desc            []string
		qty, harga, amt string
		h               float64
	}
	var lateFee int64
	rows := make([]tableRow, 0, len(items))
	for _, it := range items {
		lines := wrapToWidth(firstNonEmpty(it.Description, "—"), cellFs, descW, false)
		if len(lines) == 0 {
			lines = []string{"—"}
		}
		h := 10 + 12*float64(len(lines))
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

	c.rectFill(tableL, tableTop-headerH, contentW, headerH, colHead)
	hy := tableTop - 14
	c.textFitLeft(tableL+padX, hy, xQty-tableL-padX*2, 8, fontBold, "Deskripsi", colWhite)
	c.textFitRight(xHarga-padX, hy, xHarga-xQty-padX*2, 8, fontBold, "Qty", colWhite)
	c.textFitRight(xJumlah-padX, hy, xJumlah-xHarga-padX*2, 8, fontBold, "Harga", colWhite)
	c.textFitRight(tableR-padX, hy, tableR-xJumlah-padX*2, 8, fontBold, "Jumlah", colWhite)

	ry := tableTop - headerH
	for i, r := range rows {
		if i%2 == 1 {
			c.rectFill(tableL, ry-r.h, contentW, r.h, colFill)
		}
		textY := ry - 13
		for j, ln := range r.desc {
			c.textFitLeft(tableL+padX, textY-float64(j)*12, descW, cellFs, fontBody, ln, colInk)
		}
		c.textFitRight(xHarga-padX, textY, xHarga-xQty-padX*2, cellFs, fontBody, r.qty, colInk)
		c.textFitRight(xJumlah-padX, textY, xJumlah-xHarga-padX*2, cellFs, fontBody, r.harga, colInk)
		c.textFitRight(tableR-padX, textY, tableR-xJumlah-padX*2, cellFs, fontBody, r.amt, colInk)
		ry -= r.h
	}
	c.line(tableL, tableTop, tableR, tableTop, colOlive, 0.8)
	c.line(tableL, tableBottom, tableR, tableBottom, colLine, 0.7)
	c.line(xQty, tableBottom, xQty, tableTop, colLine, 0.5)
	c.line(xHarga, tableBottom, xHarga, tableTop, colLine, 0.5)
	c.line(xJumlah, tableBottom, xJumlah, tableTop, colLine, 0.5)
	y = tableBottom - 20

	// Totals
	totL := tableR - totalsW
	drawTotal := func(label, val string, bold bool) {
		font := fontBody
		color := colMuted
		if bold {
			font = fontBold
			color = colInk
		}
		c.text(totL, y, 9.5, font, label, color)
		c.textRight(tableR, y, 9.5, font, val, color)
		y -= 14
	}
	drawTotal("Subtotal", rupiah(inv.Subtotal), false)
	drawTotal("Pajak", rupiah(inv.TaxAmount), false)
	if inv.DiscountAmount > 0 {
		drawTotal("Diskon", "-"+rupiah(inv.DiscountAmount), false)
	}
	if lateFee > 0 {
		drawTotal("Denda keterlambatan", rupiah(lateFee), false)
	}
	c.line(totL, y+6, tableR, y+6, colOlive, 1)
	y -= 4
	drawTotal("TOTAL TAGIHAN", rupiah(inv.TotalAmount), true)
	if s.AdminFee > 0 {
		drawTotal("Biaya admin pembayaran online", rupiah(s.AdminFee), false)
		c.line(totL, y+6, tableR, y+6, colOlive, 0.8)
		y -= 4
		payOnline := inv.TotalAmount + s.AdminFee
		remaining := inv.TotalAmount - inv.PaidAmount
		if remaining < 0 {
			remaining = 0
		}
		if inv.PaidAmount > 0 && remaining > 0 {
			payOnline = remaining + s.AdminFee
			drawTotal("TOTAL BAYAR ONLINE (sisa)", rupiah(payOnline), true)
		} else {
			drawTotal("TOTAL BAYAR ONLINE", rupiah(payOnline), true)
		}
	}
	if inv.PaidAmount > 0 {
		drawTotal("Terbayar (pelunasan tagihan)", rupiah(inv.PaidAmount), false)
		bal := inv.TotalAmount - inv.PaidAmount
		if bal < 0 {
			bal = 0
		}
		drawTotal("Sisa tagihan", rupiah(bal), true)
	}
	if s.AdminFee > 0 {
		y -= 4
		note := "*Biaya admin hanya berlaku untuk pembayaran online (payment gateway) dan ditanggung pelanggan."
		for _, ln := range wrapToWidth(note, 8, contentW, false) {
			c.text(tableL, y, 8, fontBody, ln, colMuted)
			y -= 10
		}
	}

	// Payment + footer
	y -= 8
	if pay := s.Settings.PaymentInstructions; pay != "" {
		c.text(tableL, y, 8, fontBold, "CARA PEMBAYARAN", colOlive)
		y -= 13
		for _, ln := range wrapToWidth(pay, 9, contentW, false) {
			c.text(tableL, y, 9, fontBody, ln, colInk)
			y -= 12
		}
		y -= 6
	}
	if note := s.Settings.FooterNote; note != "" {
		c.line(tableL, y+4, tableR, y+4, colLine, 0.6)
		y -= 12
		for _, ln := range wrapToWidth(note, 8.5, contentW, false) {
			c.text(tableL, y, 8.5, fontBody, ln, colMuted)
			y -= 11
		}
	}

	const bottomPad = 32.0
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
		contact = append(contact, s.Settings.Phone)
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
		out = append(out, inv.CustomerCode)
	}
	for _, ln := range splitMultiline(s.CustomerAddress) {
		out = append(out, ln)
	}
	var contact []string
	if s.CustomerPhone != "" {
		contact = append(contact, s.CustomerPhone)
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

func minFloat(vals ...float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---- minimal PDF canvas (Helvetica) ----

type canvas struct {
	buf         bytes.Buffer
	img         *pdfJPEG
	watermark   bool
	mediaBottom float64
}

func glyphWidth(r rune, bold bool) float64 {
	w := 0.55
	switch {
	case r == ' ':
		w = 0.28
	case r == 'I' || r == 'i' || r == 'l' || r == 'j' || r == 'f' || r == 't' || r == '.' || r == ',':
		w = 0.28
	case r == 'm' || r == 'M' || r == 'W' || r == 'w':
		w = 0.83
	case r >= '0' && r <= '9':
		w = 0.56
	case r >= 'A' && r <= 'Z':
		w = 0.67
	case r >= 'a' && r <= 'z':
		w = 0.52
	}
	if bold {
		w *= 1.06
	}
	return w
}

func textWidth(s string, size float64, bold bool) float64 {
	var w float64
	for _, r := range s {
		w += glyphWidth(r, bold) * size
	}
	return w
}

func wrapToWidth(s string, size, maxW float64, bold bool) []string {
	s = strings.TrimSpace(s)
	if s == "" || maxW <= 0 {
		if s == "" {
			return nil
		}
		return []string{s}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		cur := ""
		for _, w := range words {
			next := w
			if cur != "" {
				next = cur + " " + w
			}
			if textWidth(next, size, bold) <= maxW {
				cur = next
				continue
			}
			if cur != "" {
				out = append(out, cur)
			}
			if textWidth(w, size, bold) <= maxW {
				cur = w
			} else {
				out = append(out, breakLongToken(w, size, maxW, bold)...)
				cur = ""
			}
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return out
}

func breakLongToken(s string, size, maxW float64, bold bool) []string {
	r := []rune(s)
	if len(r) == 0 {
		return nil
	}
	var out []string
	var cur []rune
	for _, ch := range r {
		next := append(append([]rune{}, cur...), ch)
		if textWidth(string(next), size, bold) <= maxW {
			cur = next
			continue
		}
		if len(cur) > 0 {
			out = append(out, string(cur))
		}
		cur = []rune{ch}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func fitTextSized(s string, size, maxW float64, bold bool) string {
	if maxW <= 0 || textWidth(s, size, bold) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 1 && textWidth(string(r)+"…", size, bold) > maxW {
		r = r[:len(r)-1]
	}
	if len(r) == 0 {
		return ""
	}
	return string(r) + "…"
}

func (c *canvas) text(x, y, size float64, font, s, color string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	if color == "" {
		color = colInk
	}
	// Isolate each text run: fill-only, no leftover stroke (avoids faux-bold / doubled glyphs).
	fmt.Fprintf(&c.buf, "q 0 Tr %s rg %s RG 0 w BT /%s %.1f Tf %.1f %.1f Td %s ET Q\n", color, color, font, size, x, y, pdfEscape(s))
}

func (c *canvas) textRight(xRight, y, size float64, font, s, color string) {
	c.text(xRight-textWidth(s, size, font == fontBold), y, size, font, s, color)
}

func (c *canvas) textFitLeft(x, y, maxW, size float64, font, s, color string) {
	c.text(x, y, size, font, fitTextSized(s, size, maxW, font == fontBold), color)
}

func (c *canvas) textFitRight(xRight, y, maxW, size float64, font, s, color string) {
	c.textRight(xRight, y, size, font, fitTextSized(s, size, maxW, font == fontBold), color)
}

func (c *canvas) line(x1, y1, x2, y2 float64, color string, width float64) {
	if color == "" {
		color = colLine
	}
	if width <= 0 {
		width = 0.6
	}
	fmt.Fprintf(&c.buf, "q %.2f w %s RG %.1f %.1f m %.1f %.1f l S Q\n", width, color, x1, y1, x2, y2)
}

func (c *canvas) rectFill(x, y, w, h float64, color string) {
	fmt.Fprintf(&c.buf, "q %s rg %.1f %.1f %.1f %.1f re f Q\n", color, x, y, w, h)
}

func (c *canvas) drawJPEG(x, y, w, h float64) {
	if c.img == nil || w <= 0 || h <= 0 {
		return
	}
	fmt.Fprintf(&c.buf, "q %.2f 0 0 %.2f %.2f %.2f cm /Im1 Do Q\n", w, h, x, y)
}

func (c *canvas) paidWatermark(cx, cy, contentH float64) {
	c.watermark = true
	size := 64.0
	if contentH > 0 && contentH < 520 {
		size = contentH * 0.11
		if size < 44 {
			size = 44
		}
		if size > 68 {
			size = 68
		}
	}
	const (
		cos = 0.81915204 // 35°
		sin = 0.57357644
	)
	off := size * 2.05
	fmt.Fprintf(&c.buf,
		"q /GS1 gs 0 Tr %s rg 1 0 0 1 %.1f %.1f cm %.5f %.5f %.5f %.5f 0 0 cm BT /F2 %.1f Tf %.1f 0 Td (LUNAS) Tj ET Q\n",
		colStamp, cx, cy, cos, sin, -sin, cos, size, -off,
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
		nil,
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
		objects = append(objects, []byte("<< /Type /ExtGState /ca 0.14 /CA 0.14 >>"))
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
				switch r {
				case '·':
					b.WriteString(" - ")
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
