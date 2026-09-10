package store

import (
	"testing"
	"time"
)

func TestCustomerFillServiceStatus(t *testing.T) {
	c := Customer{IsActive: true}
	c.FillServiceStatus()
	if c.ServiceStatus != "active" {
		t.Fatalf("active = %q", c.ServiceStatus)
	}
	c.IsActive = false
	c.FillServiceStatus()
	if c.ServiceStatus != "inactive" {
		t.Fatalf("inactive = %q", c.ServiceStatus)
	}
	now := time.Now()
	c.DismantledAt = &now
	c.FillServiceStatus()
	if c.ServiceStatus != "dismantled" {
		t.Fatalf("dismantled = %q", c.ServiceStatus)
	}
	if !c.IsDismantled() {
		t.Fatal("expected IsDismantled")
	}
}

func TestCustomerFillServiceStatusFromSubscriptions(t *testing.T) {
	cases := []struct {
		name      string
		subStatus string
		want      string
	}{
		{"suspended becomes isolir", "isolir", "isolir"},
		{"overdue stays overdue", "overdue", "overdue"},
		{"active stays active", "active", "active"},
		{"no subscription defaults active", "", "active"},
	}
	for _, tc := range cases {
		c := Customer{IsActive: true, SubStatus: tc.subStatus}
		c.FillServiceStatus()
		if c.ServiceStatus != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, c.ServiceStatus, tc.want)
		}
	}
	// Customer-level inactive wins over subscription state.
	c := Customer{IsActive: false, SubStatus: "isolir"}
	c.FillServiceStatus()
	if c.ServiceStatus != "inactive" {
		t.Fatalf("inactive customer = %q", c.ServiceStatus)
	}
}

func TestFormatSiteLabel(t *testing.T) {
	if got := formatSiteLabel("Cibinong", "CBN"); got != "Cibinong (CBN)" {
		t.Fatalf("got %q", got)
	}
	if got := formatSiteLabel("Cibinong", "cibinong"); got != "Cibinong" {
		t.Fatalf("same name/code = %q", got)
	}
	if got := formatSiteLabel("", "CBN"); got != "CBN" {
		t.Fatalf("code only = %q", got)
	}
	if got := formatSiteLabel("  ", ""); got != "" {
		t.Fatalf("blank = %q", got)
	}
}
