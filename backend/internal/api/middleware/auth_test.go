package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResolvePermCodeExact(t *testing.T) {
	cases := []struct {
		method, path, want string
	}{
		{"GET", "/api/v1/clusters", "cluster:list"},
		{"POST", "/api/v1/clusters", "cluster:create"},
		{"GET", "/api/v1/clusters/:code", "cluster:list"},
		{"POST", "/api/v1/clusters/:code/ping", "cluster:ping"},
		{"GET", "/api/v1/clusters/:code/resources/:apiVersion/:kind", "resource:list"},
		{"POST", "/api/v1/clusters/:code/resources/:apiVersion/:kind", "resource:create"},
		{"PUT", "/api/v1/clusters/:code/resources/:apiVersion/:kind/:namespace/:name", "resource:update"},
		{"DELETE", "/api/v1/clusters/:code/resources/:apiVersion/:kind/:namespace/:name", "resource:delete"},
		{"POST", "/api/v1/clusters/:code/backups", "backup:create"},
		{"GET", "/api/v1/backups", "backup:list"},
		{"GET", "/api/v1/versions/diff", "version:diff"},
		{"POST", "/api/v1/clusters/:code/helm/releases", "helm:install"},
		{"GET", "/api/v1/clusters/:code/helm/check", "helm:view"},
		{"GET", "/api/v1/clusters/:code/pods/:namespace/:pod/logs", "cluster:list"},
		{"GET", "/api/v1/unknown", ""},
	}
	for _, tc := range cases {
		got := resolvePermCode(tc.method, tc.path)
		if got != tc.want {
			t.Fatalf("%s %s => %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestResolvePermCodeDoesNotPrefixCollapse(t *testing.T) {
	// 真实请求路径不得按 /api/v1/clusters 前缀误判为 cluster:create
	if got := resolvePermCode("POST", "/api/v1/clusters/prod/resources/apps/v1/Deployment"); got != "" {
		t.Fatalf("raw request path should not match exact table, got %q", got)
	}
}

func TestRedactQuery(t *testing.T) {
	got := redactQuery("token=abc.def&foo=1")
	if got != "foo=1&token=%2A%2A%2A" && got != "token=%2A%2A%2A&foo=1" {
		// url.Values.Encode 会按 key 排序
		if got != "foo=1&token=%2A%2A%2A" {
			t.Fatalf("redactQuery = %q", got)
		}
	}
	if redactQuery("page=1") != "page=1" {
		t.Fatalf("non-sensitive query should stay")
	}
}

func TestCurrentUserIDIgnoresHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-User-ID", "99")
	c.Request = req

	if _, ok := CurrentUserID(c); ok {
		t.Fatal("expected no user without JWT context")
	}
	c.Set(CtxUserID, uint64(7))
	id, ok := CurrentUserID(c)
	if !ok || id != 7 {
		t.Fatalf("got %d ok=%v", id, ok)
	}
}
