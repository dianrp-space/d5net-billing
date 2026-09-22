package config

import (
	"strings"
	"testing"
)

func TestValidateSecrets(t *testing.T) {
	good := &Config{
		JWTSecret:     "9f2c7a1b4e6d8f0a3c5e7b9d1f3a5c7e9f2c7a1b4e6d8f0a3c5e7b9d1f3a5c7e",
		EncryptionKey: "a1b2c3d4e5f60718293a4b5c6d7e8f90",
	}
	if err := good.validateSecrets(); err != nil {
		t.Fatalf("valid secrets rejected: %v", err)
	}

	cases := map[string]Config{
		"default jwt":   {JWTSecret: "change-me-to-random-64-char-string-in-production", EncryptionKey: strings.Repeat("x", 32)},
		"short jwt":     {JWTSecret: "pendek", EncryptionKey: strings.Repeat("x", 32)},
		"default crypt": {JWTSecret: strings.Repeat("y", 40), EncryptionKey: "01234567890123456789012345678901"},
		"short crypt":   {JWTSecret: strings.Repeat("y", 40), EncryptionKey: "terlalu-pendek"},
	}
	for name, cfg := range cases {
		if err := cfg.validateSecrets(); err == nil {
			t.Fatalf("%s: weak secret accepted", name)
		}
	}
}
