package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/k8s-platform/console/internal/cluster"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// HelmRelease represents a Helm release summary
type HelmRelease struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Revision   string `json:"revision"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"appVersion"`
	Updated    string `json:"updated"`
}

// InstallInput holds parameters for helm install/upgrade
type InstallInput struct {
	ClusterCode string `json:"cluster_code"`
	ReleaseName string `json:"release_name"`
	Namespace   string `json:"namespace"`
	ChartRef    string `json:"chart_ref"`
	RepoURL     string `json:"repo_url"`
	Values      string `json:"values"`
	Version     string `json:"version"`
	Wait        bool   `json:"wait"`
	DryRun      bool   `json:"dry_run"`
}

// Manager manages Helm operations
type Manager struct {
	ClusterMgr *cluster.Manager
	Log        *logger.Logger
}

func NewManager(cm *cluster.Manager, log *logger.Logger) *Manager {
	return &Manager{ClusterMgr: cm, Log: log}
}

// ListReleases lists Helm releases by querying k8s secrets with label owner=helm
func (m *Manager) ListReleases(ctx context.Context, clusterCode, namespace string) ([]HelmRelease, error) {
	cfg, err := m.ClusterMgr.GetRestConfig(clusterCode)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, errcode.Wrap(errcode.Internal, err, "创建 kubernetes clientset 失败")
	}

	listOpts := metav1.ListOptions{
		LabelSelector: "owner=helm",
	}
	ns := namespace
	if ns == "" || ns == "all" || ns == "ALL" {
		ns = ""
	}

	secrets, err := clientset.CoreV1().Secrets(ns).List(ctx, listOpts)
	if err != nil {
		return nil, errcode.Wrap(errcode.Internal, err, "查询 Helm release secrets 失败")
	}

	// Group by release name, pick highest revision
	releaseMap := make(map[string]*HelmRelease)
	for _, sec := range secrets.Items {
		name := sec.Labels["name"]
		if name == "" {
			continue
		}
		status := sec.Labels["status"]
		version := sec.Labels["version"]
		chartName := sec.Labels["chart"]
		if chartName == "" {
			chartName = "-"
		}

		existing, ok := releaseMap[name]
		if ok && existing.Revision >= version {
			continue
		}

		releaseMap[name] = &HelmRelease{
			Name:       name,
			Namespace:  sec.Namespace,
			Revision:   version,
			Status:     status,
			Chart:      chartName,
			AppVersion: "",
			Updated:    sec.CreationTimestamp.Format(time.RFC3339),
		}
	}

	releases := make([]HelmRelease, 0, len(releaseMap))
	for _, r := range releaseMap {
		releases = append(releases, *r)
	}
	return releases, nil
}

// Install installs or upgrades a Helm release via helm CLI
func (m *Manager) Install(ctx context.Context, in InstallInput) (string, error) {
	kubeconfigPath, cleanup, err := m.writeKubeconfig(in.ClusterCode)
	if err != nil {
		return "", err
	}
	defer cleanup()

	// Check if release exists → upgrade or install
	existing := m.checkReleaseExists(ctx, kubeconfigPath, in.Namespace, in.ReleaseName)

	args := []string{
		"--kubeconfig", kubeconfigPath,
		"--namespace", in.Namespace,
	}
	if existing {
		args = append(args, "upgrade", in.ReleaseName, in.ChartRef)
	} else {
		args = append(args, "install", in.ReleaseName, in.ChartRef)
	}

	if in.Version != "" {
		args = append(args, "--version", in.Version)
	}
	if in.Wait {
		args = append(args, "--wait")
	}
	if in.DryRun {
		args = append(args, "--dry-run")
	}
	if strings.TrimSpace(in.RepoURL) != "" {
		if err := ValidateRepoURL(in.RepoURL); err != nil {
			return "", err
		}
		args = append(args, "--repo", strings.TrimSpace(in.RepoURL))
	}
	if in.Values != "" {
		valuesPath := filepath.Join(filepath.Dir(kubeconfigPath), "values.yaml")
		if err := os.WriteFile(valuesPath, []byte(in.Values), 0o600); err != nil {
			return "", fmt.Errorf("write values file: %w", err)
		}
		args = append(args, "-f", valuesPath)
	}

	m.Log.Infof("📦 helm exec: helm %s", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("helm install/upgrade failed: %s: %w", stderr.String(), err)
	}
	return stdout.String(), nil
}

// Uninstall removes a Helm release
func (m *Manager) Uninstall(ctx context.Context, clusterCode, namespace, releaseName string) (string, error) {
	kubeconfigPath, cleanup, err := m.writeKubeconfig(clusterCode)
	if err != nil {
		return "", err
	}
	defer cleanup()

	args := []string{
		"--kubeconfig", kubeconfigPath,
		"--namespace", namespace,
		"uninstall", releaseName,
	}
	cmd := exec.CommandContext(ctx, "helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("helm uninstall failed: %s: %w", stderr.String(), err)
	}
	return stdout.String(), nil
}

// Rollback rolls back a Helm release to a previous revision
func (m *Manager) Rollback(ctx context.Context, clusterCode, namespace, releaseName string, revision int) (string, error) {
	kubeconfigPath, cleanup, err := m.writeKubeconfig(clusterCode)
	if err != nil {
		return "", err
	}
	defer cleanup()

	args := []string{
		"--kubeconfig", kubeconfigPath,
		"--namespace", namespace,
		"rollback", releaseName,
	}
	if revision > 0 {
		args = append(args, fmt.Sprintf("%d", revision))
	}
	cmd := exec.CommandContext(ctx, "helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("helm rollback failed: %s: %w", stderr.String(), err)
	}
	return stdout.String(), nil
}

// ListHistory lists the revision history of a release via helm CLI
func (m *Manager) ListHistory(ctx context.Context, clusterCode, namespace, releaseName string) ([]map[string]interface{}, error) {
	kubeconfigPath, cleanup, err := m.writeKubeconfig(clusterCode)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{
		"--kubeconfig", kubeconfigPath,
		"--namespace", namespace,
		"history", releaseName, "-o", "json",
	}
	cmd := exec.CommandContext(ctx, "helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helm history failed: %s: %w", stderr.String(), err)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("parse helm history: %w", err)
	}
	return result, nil
}

// ---------- internal helpers ----------

func (m *Manager) writeKubeconfig(clusterCode string) (string, func(), error) {
	_, kubeconfigBytes, err := m.ClusterMgr.GetKubeConfig(clusterCode)
	if err != nil {
		return "", nil, err
	}
	dir := filepath.Join(os.TempDir(), "k8s-platform-helm")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%d.yaml", clusterCode, time.Now().UnixNano()))
	if err := os.WriteFile(path, kubeconfigBytes, 0o600); err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(path) }
	return path, cleanup, nil
}

func (m *Manager) runHelm(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

func (m *Manager) checkReleaseExists(ctx context.Context, kubeconfigPath, namespace, releaseName string) bool {
	args := []string{
		"--kubeconfig", kubeconfigPath,
		"--namespace", namespace,
		"status", releaseName, "-o", "json",
	}
	cmd := exec.CommandContext(ctx, "helm", args...)
	return cmd.Run() == nil
}
