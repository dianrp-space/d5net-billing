package invoice

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

func samplePDF(t *testing.T) string {
	t.Helper()
	issued := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	inv := &store.Invoice{
		ID:            xid.New(),
		InvoiceNumber: "INV-202609-0001",
		CustomerName:  "Budi (Santoso), Jr.",
		Status:        "issued",
		Subtotal:      105000,
		TotalAmount:   105000,
		DueDate:       time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		IssuedAt:      &issued,
	}
	items := []store.InvoiceItem{
		{Description: "Internet 20 Mbps", Quantity: 1, UnitPrice: 100000, Amount: 100000},
		{Description: "Denda keterlambatan", Quantity: 1, UnitPrice: 5000, Amount: 5000},
	}
	return string(RenderPDF(inv, items))
}

func TestRenderPDFStreamLengthMatches(t *testing.T) {
	pdf := samplePDF(t)
	const hdr = "<< /Length "
	i := strings.Index(pdf, hdr)
	if i < 0 {
		t.Fatal("content stream object not found")
	}
	j := strings.Index(pdf, ">>\nstream\n")
	if j < 0 || j < i {
		t.Fatal("stream start marker not found")
	}
	declared, err := strconv.Atoi(strings.TrimSpace(pdf[i+len(hdr) : j]))
	if err != nil {
		t.Fatalf("invalid /Length value: %v", err)
	}
	k := strings.Index(pdf, "\nendstream")
	if k < 0 {
		t.Fatal("endstream marker not found")
	}
	stream := pdf[j+len(">>\nstream\n") : k]
	if len(stream) != declared {
		t.Fatalf("declared /Length = %d, actual stream bytes = %d", declared, len(stream))
	}
}

func TestRenderPDFSetsLeading(t *testing.T) {
	pdf := samplePDF(t)
	if !strings.Contains(pdf, "BT /F1 10 Tf 12 TL 50 800 Td ") {
		t.Fatal("text leading not set — lines would overlap without TL")
	}
	if strings.Count(pdf, "T*") < 2 {
		t.Fatal("expected line breaks via T*")
	}
}

func TestRenderPDFXrefOffsetsValid(t *testing.T) {
	pdf := samplePDF(t)
	x := strings.Index(pdf, "xref\n0 6\n0000000000 65535 f")
	if x < 0 {
		t.Fatal("xref table not found")
	}
	sx := strings.LastIndex(pdf, "startxref\n")
	if sx < 0 {
		t.Fatal("startxref not found")
	}
	start, err := strconv.Atoi(strings.TrimSpace(strings.SplitN(pdf[sx+len("startxref\n"):], "\n", 2)[0]))
	if err != nil {
		t.Fatalf("invalid startxref: %v", err)
	}
	if start != x {
		t.Fatalf("startxref = %d, want %d", start, x)
	}
	lines := strings.Split(pdf[x:], "\n")
	if lines[1] != "0 6" {
		t.Fatalf("unexpected xref size line: %q", lines[1])
	}
	for i, line := range lines[3:8] {
		off, err := strconv.Atoi(strings.TrimSpace(strings.SplitN(line, " ", 2)[0]))
		if err != nil {
			t.Fatalf("xref entry %d invalid: %v", i+1, err)
		}
		want := strconv.Itoa(i+1) + " 0 obj"
		if !strings.HasPrefix(pdf[off:], want) {
			t.Fatalf("xref offset for object %d points to %q, want prefix %q", i+1, pdf[off:off+16], want)
		}
	}
}

func TestRenderPDFEscapesSpecialChars(t *testing.T) {
	pdf := samplePDF(t)
	if strings.Contains(pdf, "(Budi (Santoso), Jr.)") {
		t.Fatal("unescaped parentheses leaked into PDF text operators")
	}
	if !strings.Contains(pdf, `Budi \(Santoso\), Jr.`) {
		t.Fatal("expected escaped parentheses in stream")
	}
}
