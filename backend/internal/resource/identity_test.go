package resource

import "testing"

func TestParseYAMLIdentityAndMatch(t *testing.T) {
	raw := []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: prod
`)
	id, err := ParseYAMLIdentity(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := id.MatchGVK("apps/v1", "Deployment"); err != nil {
		t.Fatal(err)
	}
	if err := id.MatchNamespacedName("prod", "web"); err != nil {
		t.Fatal(err)
	}
	if err := id.MatchNamespacedName("dev", "web"); err == nil {
		t.Fatal("expected namespace mismatch")
	}
}

func TestNormalizeNamespace(t *testing.T) {
	if NormalizeNamespace("_") != "" || NormalizeNamespace("dev") != "dev" {
		t.Fatal("normalize failed")
	}
}
