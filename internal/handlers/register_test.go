package handlers

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

// Regression: struct anonim dengan field bernama sama (mis. "Items")
// membuat huma panic "duplicate name" saat registrasi route.
func TestRegisterAllNoDuplicateSchema(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("test", "0.0.0"))
	RegisterAll(api, &Deps{})
}
