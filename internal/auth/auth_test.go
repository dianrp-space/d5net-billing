package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword("secret123", h)
	if err != nil || !ok {
		t.Fatal("verify failed")
	}
	ok, _ = VerifyPassword("wrong", h)
	if ok {
		t.Fatal("should not match")
	}
}

func TestEncryptor(t *testing.T) {
	e, err := NewEncryptor("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := e.Encrypt("router-pass")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := e.Decrypt(enc)
	if err != nil || plain != "router-pass" {
		t.Fatal(plain, err)
	}
}

func TestJWT(t *testing.T) {
	ts := NewTokenService("secret", time.Hour, 24*time.Hour)
	uid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tid := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	tok, _, err := ts.CreateAccessToken(uid, tid, "a@b.c", "admin")
	if err != nil {
		t.Fatal(err)
	}
	c, err := ts.ParseToken(tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.UserID != uid.String() || c.TenantID != tid.String() {
		t.Fatal(c)
	}
}
