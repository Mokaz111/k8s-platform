package worker

import "testing"

func TestUniqueRestoredName(t *testing.T) {
	taken := map[string]bool{"web-restored-20260830": true}
	got := uniqueRestoredName("web", "20260830", func(n string) bool { return taken[n] })
	if got != "web-restored-20260830-2" {
		t.Fatalf("got %q", got)
	}
	got = uniqueRestoredName("api", "20260830", func(string) bool { return false })
	if got != "api-restored-20260830" {
		t.Fatalf("got %q", got)
	}
}
