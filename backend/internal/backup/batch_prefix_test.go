package backup

import "testing"

func TestBatchObjectPrefix(t *testing.T) {
	got := BatchObjectPrefix(12, `prod/20260831/12-00005_Deployment_web.yaml`)
	want := "prod/20260831/12-"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if BatchObjectPrefix(12, want) != want {
		t.Fatal("already-prefix should stay")
	}
	if !IsNamespaceBatch("namespace_batch") || !IsNamespaceBatch("namespace") {
		t.Fatal("IsNamespaceBatch should accept both type names")
	}
	if IsNamespaceBatch("single") || IsNamespaceBatch("restore") {
		t.Fatal("IsNamespaceBatch should reject non-batch types")
	}
}
