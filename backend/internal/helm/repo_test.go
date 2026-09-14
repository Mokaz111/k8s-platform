package helm

import "testing"

func TestValidateRepoName(t *testing.T) {
	ok := []string{"bitnami", "stable", "my-repo", "repo_1", "A.b-c"}
	for _, n := range ok {
		if err := ValidateRepoName(n); err != nil {
			t.Fatalf("name %q should be valid: %v", n, err)
		}
	}
	bad := []string{"", " has space", "../etc", "repo;rm", "-leading", "a/b", "x y"}
	for _, n := range bad {
		if err := ValidateRepoName(n); err == nil {
			t.Fatalf("name %q should be invalid", n)
		}
	}
}

func TestValidateRepoURL(t *testing.T) {
	if err := ValidateRepoURL("https://charts.bitnami.com/bitnami"); err != nil {
		t.Fatalf("https url should be valid: %v", err)
	}
	if err := ValidateRepoURL("http://127.0.0.1:8080/charts"); err != nil {
		t.Fatalf("http url should be valid: %v", err)
	}
	for _, u := range []string{"", "ftp://x", "javascript:alert(1)", "https://", "not-a-url"} {
		if err := ValidateRepoURL(u); err == nil {
			t.Fatalf("url %q should be invalid", u)
		}
	}
}
