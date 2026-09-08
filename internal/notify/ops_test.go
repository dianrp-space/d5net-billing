package notify

import "testing"

func TestOpsMsg(t *testing.T) {
	got := OpsMsg("isolir", "Budi (C-01)", "User: budi", "", "Paket: 20 Mbps")
	want := "[ISOLIR] Budi (C-01)\nUser: budi\nPaket: 20 Mbps"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestOpsNetworkLines(t *testing.T) {
	lines := []string{"Pelanggan: Budi"}
	lines = append(lines, OpsNetworkLines("Cibinong (CBN)", "CCR-Cibinong")...)
	got := OpsMsg("tiket", "Internet putus", lines...)
	want := "[TIKET] Internet putus\nPelanggan: Budi\nCluster: Cibinong (CBN)\nRouter: CCR-Cibinong"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	empty := OpsNetworkLines("", "  ")
	if empty[0] != "Cluster: —" || empty[1] != "Router: —" {
		t.Fatalf("empty labels = %#v", empty)
	}
}
