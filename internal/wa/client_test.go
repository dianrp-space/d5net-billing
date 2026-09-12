package wa

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendImageMultipart(t *testing.T) {
	var gotPhone, gotCaption, gotFile, gotName, gotDevice string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDevice = r.Header.Get("X-Device-Id")
		if !strings.HasSuffix(r.URL.Path, "/send/image") {
			t.Errorf("path = %s", r.URL.Path)
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(p)
			switch p.FormName() {
			case "phone":
				gotPhone = string(body)
			case "caption":
				gotCaption = string(body)
			case "image":
				gotFile = string(body)
				gotName = p.FileName()
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":200}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, DeviceID: "org_2"})
	if err := c.SendImage(t.Context(), "081234567890", "bayar", []byte("PNGDATA")); err != nil {
		t.Fatal(err)
	}
	if gotPhone != "6281234567890" || gotCaption != "bayar" || gotFile != "PNGDATA" || gotName != "qris.png" || gotDevice != "org_2" {
		t.Fatalf("phone=%q caption=%q file=%q name=%q device=%q", gotPhone, gotCaption, gotFile, gotName, gotDevice)
	}
}

func TestSendFileMultipart(t *testing.T) {
	var gotName, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/send/file") {
			t.Errorf("path = %s", r.URL.Path)
		}
		ct := r.Header.Get("Content-Type")
		_, params, err := mime.ParseMediaType(ct)
		if err != nil {
			t.Fatal(err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.FormName() == "file" {
				gotName = p.FileName()
				gotCT = p.Header.Get("Content-Type")
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	if err := c.SendFile(t.Context(), "62812", "tagihan", "INV-1.pdf", "application/pdf", []byte("%PDF-1.4")); err != nil {
		t.Fatal(err)
	}
	if gotName != "INV-1.pdf" || gotCT != "application/pdf" {
		t.Fatalf("name=%q ct=%q", gotName, gotCT)
	}
}

func TestSendPresence(t *testing.T) {
	var gotPath, gotPhone, gotPresence string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body struct {
			Phone    string `json:"phone"`
			Presence string `json:"presence"`
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		gotPhone, gotPresence = body.Phone, body.Presence
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":200}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	if err := c.SendPresence(t.Context(), "081234567890", PresenceComposing); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/send/presence" || gotPhone != "6281234567890" || gotPresence != "composing" {
		t.Fatalf("path=%q phone=%q presence=%q", gotPath, gotPhone, gotPresence)
	}
}
