package auth

import (
	"reflect"
	"sort"
	"testing"

	"github.com/k8s-platform/console/internal/models"
)

// ---------- PermissionTree.HasPerm ----------
func TestPermissionTree_HasPerm(t *testing.T) {
	cases := []struct {
		name   string
		perms  map[string]struct{}
		admin  bool
		check  string
		expect bool
	}{
		{"平台管理员恒真", map[string]struct{}{}, true, "any:thing", true},
		{"空权限 false", map[string]struct{}{}, false, "cluster:view", false},
		{"精确匹配 true", map[string]struct{}{"cluster:view": {}}, false, "cluster:view", true},
		{"精确不匹配", map[string]struct{}{"cluster:view": {}}, false, "backup:create", false},
		{"*:* 通配", map[string]struct{}{"*:*": {}}, false, "anything:goes", true},
		{"模块通配 module:*", map[string]struct{}{"backup:*": {}}, false, "backup:create", true},
		{"模块通配 非本模块", map[string]struct{}{"backup:*": {}}, false, "cluster:create", false},
		{"模块通配 精确优先", map[string]struct{}{"resource:*": {}, "resource:delete": {}}, false, "resource:delete", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pt := &PermissionTree{Perms: c.perms, IsPlatformAdmin: c.admin}
			got := pt.HasPerm(c.check)
			if got != c.expect {
				t.Fatalf("HasPerm(%q)=%v, want %v", c.check, got, c.expect)
			}
		})
	}
}

// ---------- PermissionTree.HasClusterScope ----------
func TestPermissionTree_HasClusterScope(t *testing.T) {
	const (
		cA = "cluster-a"
		cB = "cluster-b"
	)
	cases := []struct {
		name   string
		pt     *PermissionTree
		check  string
		expect bool
	}{
		{
			name:   "nil 树返回 false",
			pt:     nil,
			check:  cA,
			expect: false,
		},
		{
			name:   "平台管理员 true",
			pt:     &PermissionTree{IsPlatformAdmin: true},
			check:  cA,
			expect: true,
		},
		{
			name: "platform scope 全部放行",
			pt: &PermissionTree{Scopes: []Scope{
				{ScopeType: models.ScopePlatform},
			}},
			check:  cA,
			expect: true,
		},
		{
			name: "cluster scope 精确命中",
			pt: &PermissionTree{Scopes: []Scope{
				{ScopeType: models.ScopeCluster, ClusterCode: cA},
			}},
			check:  cA,
			expect: true,
		},
		{
			name: "cluster scope 未命中",
			pt: &PermissionTree{Scopes: []Scope{
				{ScopeType: models.ScopeCluster, ClusterCode: cA},
			}},
			check:  cB,
			expect: false,
		},
		{
			name: "namespace scope 不算整集群",
			pt: &PermissionTree{Scopes: []Scope{
				{ScopeType: models.ScopeNamespace, ClusterCode: cA, Namespace: "default"},
			}},
			check:  cA,
			expect: false,
		},
		{
			name: "namespace scope 跨集群无访问",
			pt: &PermissionTree{Scopes: []Scope{
				{ScopeType: models.ScopeNamespace, ClusterCode: cA, Namespace: "default"},
			}},
			check:  cB,
			expect: false,
		},
		{
			name:   "空 scope 无访问",
			pt:     &PermissionTree{},
			check:  cA,
			expect: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.pt.HasClusterScope(c.check)
			if got != c.expect {
				t.Fatalf("HasClusterScope(%q)=%v, want %v", c.check, got, c.expect)
			}
		})
	}
}

func TestPermissionTree_HasAnyAccessToCluster(t *testing.T) {
	const cA = "cluster-a"
	nsOnly := &PermissionTree{Scopes: []Scope{
		{ScopeType: models.ScopeNamespace, ClusterCode: cA, Namespace: "default"},
	}}
	if !nsOnly.HasAnyAccessToCluster(cA) {
		t.Fatal("namespace scope 应对本集群 HasAnyAccessToCluster=true")
	}
	if nsOnly.HasClusterScope(cA) {
		t.Fatal("namespace scope 不应 HasClusterScope")
	}
	if nsOnly.HasAnyAccessToCluster("other") {
		t.Fatal("跨集群应无访问")
	}
}

// ---------- PermissionTree.HasNamespaceScope ----------
func TestPermissionTree_HasNamespaceScope(t *testing.T) {
	const (
		cA = "cluster-a"
		cB = "cluster-b"
		ns = "default"
	)
	cases := []struct {
		name   string
		pt     *PermissionTree
		cl, ns string
		expect bool
	}{
		{"nil tree", nil, cA, ns, false},
		{"平台管理员 any", &PermissionTree{IsPlatformAdmin: true}, cA, ns, true},
		{"platform scope", &PermissionTree{Scopes: []Scope{{ScopeType: models.ScopePlatform}}}, cB, "x", true},
		{"cluster scope 放行全部 ns", &PermissionTree{Scopes: []Scope{{ScopeType: models.ScopeCluster, ClusterCode: cA}}}, cA, "any", true},
		{"cluster scope 跨集群不放行", &PermissionTree{Scopes: []Scope{{ScopeType: models.ScopeCluster, ClusterCode: cA}}}, cB, ns, false},
		{"namespace scope 精确命中", &PermissionTree{Scopes: []Scope{{ScopeType: models.ScopeNamespace, ClusterCode: cA, Namespace: ns}}}, cA, ns, true},
		{"namespace scope ns 不同", &PermissionTree{Scopes: []Scope{{ScopeType: models.ScopeNamespace, ClusterCode: cA, Namespace: ns}}}, cA, "kube-system", false},
		{"namespace scope cluster 不同", &PermissionTree{Scopes: []Scope{{ScopeType: models.ScopeNamespace, ClusterCode: cA, Namespace: ns}}}, cB, ns, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.pt.HasNamespaceScope(c.cl, c.ns)
			if got != c.expect {
				t.Fatalf("HasNamespaceScope(%q,%q)=%v, want %v", c.cl, c.ns, got, c.expect)
			}
		})
	}
}

// ---------- PermissionTree.AllowedClusters ----------
func TestPermissionTree_AllowedClusters(t *testing.T) {
	const (
		cA = "cluster-a"
		cB = "cluster-b"
		cC = "cluster-c"
	)
	cases := []struct {
		name       string
		pt         *PermissionTree
		expectFull bool
		expectList []string
	}{
		{
			name:       "nil tree",
			pt:         nil,
			expectFull: false,
			expectList: nil,
		},
		{
			name:       "平台管理员",
			pt:         &PermissionTree{IsPlatformAdmin: true},
			expectFull: true,
			expectList: nil,
		},
		{
			name:       "platform scope",
			pt:         &PermissionTree{Scopes: []Scope{{ScopeType: models.ScopePlatform}}},
			expectFull: true,
			expectList: nil,
		},
		{
			name: "cluster + namespace 去重",
			pt: &PermissionTree{Scopes: []Scope{
				{ScopeType: models.ScopeCluster, ClusterCode: cA},
				{ScopeType: models.ScopeNamespace, ClusterCode: cA, Namespace: "x"},
				{ScopeType: models.ScopeNamespace, ClusterCode: cB, Namespace: "y"},
			}},
			expectFull: false,
			expectList: []string{cA, cB},
		},
		{
			name:       "空 scopes 空列表",
			pt:         &PermissionTree{Scopes: []Scope{{ScopeType: "bad-type", ClusterCode: cC}}},
			expectFull: false,
			expectList: []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			list, full := c.pt.AllowedClusters()
			if full != c.expectFull {
				t.Fatalf("isFull=%v, want %v", full, c.expectFull)
			}
			sort.Strings(list)
			sort.Strings(c.expectList)
			if len(c.expectList) == 0 && len(list) == 0 {
				return
			}
			if !reflect.DeepEqual(list, c.expectList) {
				t.Fatalf("list=%v, want %v", list, c.expectList)
			}
		})
	}
}

// ---------- PermissionTree.AllowedNamespaces ----------
func TestPermissionTree_AllowedNamespaces(t *testing.T) {
	const cA = "cluster-a"
	const cB = "cluster-b"

	cases := []struct {
		name       string
		pt         *PermissionTree
		cluster    string
		expectFull bool
		expectNs   []string
	}{
		{"nil tree", nil, cA, false, nil},
		{"平台管理员", &PermissionTree{IsPlatformAdmin: true}, cA, true, nil},
		{"platform scope", &PermissionTree{Scopes: []Scope{{ScopeType: models.ScopePlatform}}}, cA, true, nil},
		{"cluster scope on cA", &PermissionTree{Scopes: []Scope{
			{ScopeType: models.ScopeCluster, ClusterCode: cA},
		}}, cA, true, nil},
		{"cluster scope on cA, query cB", &PermissionTree{Scopes: []Scope{
			{ScopeType: models.ScopeCluster, ClusterCode: cA},
		}}, cB, false, []string{}},
		{"namespace scope exact list", &PermissionTree{Scopes: []Scope{
			{ScopeType: models.ScopeNamespace, ClusterCode: cA, Namespace: "default"},
			{ScopeType: models.ScopeNamespace, ClusterCode: cA, Namespace: "kube-system"},
			{ScopeType: models.ScopeNamespace, ClusterCode: cB, Namespace: "ignored"},
		}}, cA, false, []string{"default", "kube-system"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nsList, full := c.pt.AllowedNamespaces(c.cluster)
			if full != c.expectFull {
				t.Fatalf("isFullCluster=%v, want %v (ns=%v)", full, c.expectFull, nsList)
			}
			sort.Strings(nsList)
			sort.Strings(c.expectNs)
			if len(c.expectNs) == 0 && len(nsList) == 0 {
				return
			}
			if !reflect.DeepEqual(nsList, c.expectNs) {
				t.Fatalf("nsList=%v, want %v", nsList, c.expectNs)
			}
		})
	}
}
