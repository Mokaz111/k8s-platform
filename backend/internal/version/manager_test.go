package version

import (
	"strings"
	"testing"
)

func TestDiffVersionsRejectsSameSeq(t *testing.T) {
	m := &Manager{}
	_, _, err := m.DiffVersions("c", "ns", "apps/v1", "Deployment", "web", 2, 2)
	if err == nil {
		t.Fatal("expected error when seqA == seqB")
	}
}

func TestDiffVersionsRejectsBothCurrent(t *testing.T) {
	m := &Manager{}
	_, _, err := m.DiffVersions("c", "ns", "apps/v1", "Deployment", "web", CurrentVersionSeq, CurrentVersionSeq)
	if err == nil {
		t.Fatal("expected error when both sides are current")
	}
}

func TestDiffVersionsRejectsNegativeSeq(t *testing.T) {
	m := &Manager{}
	_, _, err := m.DiffVersions("c", "ns", "apps/v1", "Deployment", "web", -1, 1)
	if err == nil {
		t.Fatal("expected error for negative seq")
	}
}

func TestStripManagedFieldsYAML(t *testing.T) {
	raw := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  managedFields:\n    - manager: kubectl\nspec:\n  replicas: 1\n"
	out := stripManagedFieldsYAML(raw)
	if strings.Contains(out, "managedFields") {
		t.Fatalf("managedFields should be stripped, got:\n%s", out)
	}
	if !strings.Contains(out, "name: web") {
		t.Fatalf("expected remaining metadata, got:\n%s", out)
	}
}
