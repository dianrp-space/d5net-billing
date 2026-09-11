package provision

import (
	"context"
	"testing"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

type fakeProv struct {
	applied int
}

func (f *fakeProv) Apply(ctx context.Context, s *ServiceSpec) error      { f.applied++; return nil }
func (f *fakeProv) Suspend(ctx context.Context, s *ServiceSpec) error    { return nil }
func (f *fakeProv) Resume(ctx context.Context, s *ServiceSpec) error     { return nil }
func (f *fakeProv) Remove(ctx context.Context, s *ServiceSpec) error     { return nil }
func (f *fakeProv) Disconnect(ctx context.Context, s *ServiceSpec) error { return nil }
func (f *fakeProv) ActiveSessions(ctx context.Context, routerID xid.ID) ([]Session, error) {
	return []Session{{Username: "user1"}}, nil
}
func (f *fakeProv) TestConnection(ctx context.Context, routerID xid.ID) (string, error) {
	return "ok", nil
}
func (f *fakeProv) BackupConfig(ctx context.Context, routerID xid.ID) (string, error) {
	return "# backup", nil
}
func (f *fakeProv) Capabilities() Caps { return Caps{PPPoE: true} }

func TestCommentTag(t *testing.T) {
	got := CommentTag("D5Net", "DLMA-202609-0001", "Budi Santoso")
	want := "D5Net:DLMA-202609-0001 Budi Santoso"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if CommentTag("Acme ISP", "ABC", "") != "Acme-ISP:ABC" {
		t.Fatal(CommentTag("Acme ISP", "ABC", ""))
	}
	if CommentTag("", "ABC", "") != "d5n:ABC" {
		t.Fatal(CommentTag("", "ABC", ""))
	}
	if !IsOwnedComment("drp:KODE Nama", "Acme") || IsOwnedComment("manual", "Acme") {
		t.Fatal("ownership legacy")
	}
	if !IsOwnedComment("Acme:KODE", "Acme") {
		t.Fatal("ownership prefix")
	}
}

func TestProfileComment(t *testing.T) {
	if got := ProfileComment("Acme ISP", 150000); got != "Acme-ISP Rp150.000" {
		t.Fatalf("got %q", got)
	}
	if got := FormatRpDots(1500000); got != "1.500.000" {
		t.Fatalf("got %q", got)
	}
	if got := ProfileComment("Acme", 0); got != "Acme" {
		t.Fatalf("got %q", got)
	}
}

func TestFakeProvisionerApply(t *testing.T) {
	f := &fakeProv{}
	if err := f.Apply(context.Background(), &ServiceSpec{Username: "a"}); err != nil {
		t.Fatal(err)
	}
	if f.applied != 1 {
		t.Fatal("apply")
	}
}

func TestSecretLooksIsolir(t *testing.T) {
	if !SecretLooksIsolir("isolir", "ISOLIR Acme:KODE", "isolir", "10Mbps") {
		t.Fatal("comment prefix")
	}
	if !SecretLooksIsolir("isolir", "Acme:KODE", "isolir", "10Mbps") {
		t.Fatal("profile isolir")
	}
	if SecretLooksIsolir("10Mbps", "Acme:KODE", "isolir", "10Mbps") {
		t.Fatal("already on plan")
	}
	if SecretLooksIsolir("isolir", "Acme:KODE", "isolir", "isolir") {
		t.Fatal("plan profile same name as isolir, no comment")
	}
	if !SecretLooksIsolir("isolir", "ISOLIR Acme:KODE", "isolir", "isolir") {
		t.Fatal("same name still isolir via comment")
	}
}

func TestIsolirComment(t *testing.T) {
	got := WithIsolirComment("Acme:KODE Nama")
	if got != "ISOLIR Acme:KODE Nama" {
		t.Fatal(got)
	}
	if WithIsolirComment(got) != got {
		t.Fatal("not idempotent")
	}
	if WithoutIsolirComment(got) != "Acme:KODE Nama" {
		t.Fatal(WithoutIsolirComment(got))
	}
	if WithoutIsolirComment("Acme:KODE") != "Acme:KODE" {
		t.Fatal("should leave non-isolir")
	}
}
