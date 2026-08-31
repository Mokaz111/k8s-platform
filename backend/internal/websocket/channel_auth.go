package websocket

import (
	"encoding/json"
	"strings"

	"github.com/k8s-platform/console/internal/auth"
)

func authorizeChannel(pt *auth.PermissionTree, channel string) bool {
	if pt == nil || channel == "" {
		return false
	}
	if strings.Contains(channel, "*") {
		return false
	}
	switch {
	case channel == TypeTaskProgress:
		return pt.HasPerm("backup:list")
	case channel == TypeClusterEvent:
		return pt.HasPerm("cluster:list")
	case strings.HasPrefix(channel, TypePodLogs+":"):
		parts := strings.Split(channel, ":")
		// pod_logs:{cluster}:{namespace}:{pod}
		if len(parts) != 4 {
			return false
		}
		clusterCode, namespace := parts[1], parts[2]
		if clusterCode == "" || namespace == "" || parts[3] == "" {
			return false
		}
		if !pt.HasPerm("cluster:list") && !pt.HasPerm("resource:get") {
			return false
		}
		return pt.HasNamespaceScope(clusterCode, namespace)
	default:
		return false
	}
}

func authorizeTaskProgressPayload(pt *auth.PermissionTree, raw []byte) bool {
	if !authorizeChannel(pt, TypeTaskProgress) {
		return false
	}
	var p struct {
		ClusterCode string `json:"cluster_code"`
		Namespace   string `json:"namespace"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return false
	}
	if p.ClusterCode == "" {
		return pt.HasClusterScope("")
	}
	if p.Namespace == "" || strings.HasPrefix(p.Namespace, "multi:") {
		return pt.HasClusterScope(p.ClusterCode)
	}
	return pt.HasNamespaceScope(p.ClusterCode, p.Namespace)
}
