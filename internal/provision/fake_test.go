package provision

import (
	"context"
	"testing"

	"github.com/dianrp/drp-billing/internal/xid"
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
	got := CommentTag("drp-billing", "DLMA-202609-0001", "Budi Santoso")
	want := "drp-billing:DLMA-202609-0001 Budi Santoso"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if CommentTag("Acme ISP", "ABC", "") != "Acme-ISP:ABC" {
		t.Fatal(CommentTag("Acme ISP", "ABC", ""))
	}
	if CommentTag("", "ABC", "") != "drp:ABC" {
		t.Fatal(CommentTag("", "ABC", ""))
	}
	if !IsOwnedComment("drp:KODE Nama", "Acme") || IsOwnedComment("manual", "Acme") {
		t.Fatal("ownership legacy")
	}
	if !IsOwnedComment("Acme:KODE", "Acme") {
		t.Fatal("ownership prefix")
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
