package invoice

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/dianrp/drp-billing/internal/store"
)

// RenderPDF produces a minimal PDF (single page text). Sufficient for thermal/A4 print.
func RenderPDF(inv *store.Invoice, items []store.InvoiceItem) []byte {
	var body bytes.Buffer
	fmt.Fprintf(&body, "INVOICE %s\n", inv.InvoiceNumber)
	fmt.Fprintf(&body, "Pelanggan: %s\n", inv.CustomerName)
	fmt.Fprintf(&body, "Status: %s\n", inv.Status)
	fmt.Fprintf(&body, "Jatuh tempo: %s\n", inv.DueDate.Format("2006-01-02"))
	if inv.IssuedAt != nil {
		fmt.Fprintf(&body, "Terbit: %s\n", inv.IssuedAt.Format("2006-01-02"))
	}
	body.WriteString("\nItem:\n")
	var lateFee int64
	for _, it := range items {
		fmt.Fprintf(&body, "- %s  qty=%d  @%d  = %d\n", it.Description, it.Quantity, it.UnitPrice, it.Amount)
		if isLateFeeItem(it.Description) {
			lateFee += it.Amount
		}
	}
	body.WriteString("\n")
	fmt.Fprintf(&body, "Subtotal: %d\n", inv.Subtotal)
	if inv.TaxAmount > 0 {
		fmt.Fprintf(&body, "Pajak: %d\n", inv.TaxAmount)
	}
	if inv.DiscountAmount > 0 {
		fmt.Fprintf(&body, "Diskon: -%d\n", inv.DiscountAmount)
	}
	if lateFee > 0 {
		fmt.Fprintf(&body, "Denda keterlambatan: %d\n", lateFee)
	}
	fmt.Fprintf(&body, "Total: %d\n", inv.TotalAmount)
	if inv.PaidAmount > 0 {
		fmt.Fprintf(&body, "Terbayar: %d\n", inv.PaidAmount)
		fmt.Fprintf(&body, "Sisa: %d\n", inv.TotalAmount-inv.PaidAmount)
	}
	content := body.String()
	stream := "BT /F1 10 Tf 12 TL 50 800 Td " + pdfEscape(content) + " ET"
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>",
	}
	offsets := make([]int, len(objects)+1)
	offsets[0] = pdf.Len()
	for i, obj := range objects {
		offsets[i+1] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&pdf, "trailer << /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return pdf.Bytes()
}

func isLateFeeItem(desc string) bool {
	d := strings.ToLower(desc)
	return strings.Contains(d, "denda") || strings.Contains(d, "late") || strings.Contains(d, "keterlambatan")
}

func pdfEscape(s string) string {
	var b bytes.Buffer
	b.WriteByte('(')
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(") Tj T* (")
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			if r < 32 || r > 126 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte(')')
	b.WriteString(" Tj")
	return b.String()
}
