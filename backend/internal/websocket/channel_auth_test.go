package websocket

import (
	"testing"

	"github.com/k8s-platform/console/internal/auth"
	"github.com/k8s-platform/console/internal/models"
)

func TestAuthorizeChannel(t *testing.T) {
	pt := &auth.PermissionTree{
		Perms: map[string]struct{}{
			"backup:list":  {},
			"cluster:list": {},
		},
		Scopes: []auth.Scope{
			{ScopeType: models.ScopeNamespace, ClusterCode: "c1", Namespace: "dev"},
		},
	}

	if !authorizeChannel(pt, "task_progress") {
		t.Fatal("backup:list should allow task_progress")
	}
	if authorizeChannel(pt, "pod_logs:c1:kube-system:nginx") {
		t.Fatal("should deny other namespace")
	}
	if !authorizeChannel(pt, "pod_logs:c1:dev:nginx") {
		t.Fatal("should allow scoped pod logs")
	}
	if authorizeChannel(pt, "pod_logs:*") {
		t.Fatal("wildcard must be denied")
	}
	if authorizeChannel(nil, "task_progress") {
		t.Fatal("nil tree must deny")
	}
}

func TestAuthorizeTaskProgressPayload(t *testing.T) {
	nsPT := &auth.PermissionTree{
		Perms: map[string]struct{}{"backup:list": {}},
		Scopes: []auth.Scope{
			{ScopeType: models.ScopeNamespace, ClusterCode: "c1", Namespace: "dev"},
		},
	}
	clusterPT := &auth.PermissionTree{
		Perms: map[string]struct{}{"backup:list": {}},
		Scopes: []auth.Scope{
			{ScopeType: models.ScopeCluster, ClusterCode: "c1"},
		},
	}

	dev := []byte(`{"cluster_code":"c1","namespace":"dev"}`)
	otherNS := []byte(`{"cluster_code":"c1","namespace":"prod"}`)
	batch := []byte(`{"cluster_code":"c1","namespace":"multi:3"}`)
	noCluster := []byte(`{"task_id":"1"}`)

	if !authorizeTaskProgressPayload(nsPT, dev) {
		t.Fatal("namespace user should see own ns progress")
	}
	if authorizeTaskProgressPayload(nsPT, otherNS) {
		t.Fatal("namespace user should not see other ns")
	}
	if authorizeTaskProgressPayload(nsPT, batch) {
		t.Fatal("namespace user should not see batch progress")
	}
	if authorizeTaskProgressPayload(nsPT, noCluster) {
		t.Fatal("namespace user should not see unscoped progress")
	}
	if !authorizeTaskProgressPayload(clusterPT, batch) {
		t.Fatal("cluster admin should see batch progress")
	}
	if !authorizeTaskProgressPayload(clusterPT, otherNS) {
		t.Fatal("cluster admin should see any ns in cluster")
	}
}
