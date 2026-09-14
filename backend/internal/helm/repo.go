package helm

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/k8s-platform/console/pkg/errcode"
)

// HelmRepo is a helm CLI repository entry (helm repo list).
type HelmRepo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// AddRepoInput is the body for helm repo add.
type AddRepoInput struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// HelmChart is a chart found via helm search repo.
type HelmChart struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	AppVersion  string `json:"app_version"`
	Description string `json:"description"`
}

var repoNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func ValidateRepoName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errcode.New(errcode.InvalidArgument, "仓库名称不能为空")
	}
	if !repoNameRe.MatchString(name) {
		return errcode.New(errcode.InvalidArgument, "仓库名称仅允许字母、数字、点、下划线和短横线，且需以字母或数字开头")
	}
	return nil
}

func ValidateRepoURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errcode.New(errcode.InvalidArgument, "仓库地址不能为空")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errcode.New(errcode.InvalidArgument, "仓库地址格式无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errcode.New(errcode.InvalidArgument, "仓库地址必须是 http 或 https URL")
	}
	if u.Host == "" {
		return errcode.New(errcode.InvalidArgument, "仓库地址缺少主机名")
	}
	return nil
}

func (m *Manager) ListRepos(ctx context.Context) ([]HelmRepo, error) {
	out, err := m.runHelm(ctx, 20*time.Second, "repo", "list", "-o", "json")
	if err != nil {
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "no repositories") {
			return []HelmRepo{}, nil
		}
		return nil, errcode.Wrap(errcode.Internal, err, "helm repo list 失败")
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "null" {
		return []HelmRepo{}, nil
	}
	var repos []HelmRepo
	if err := json.Unmarshal([]byte(out), &repos); err != nil {
		return nil, errcode.Wrap(errcode.Internal, err, "解析 helm repo list 失败")
	}
	if repos == nil {
		repos = []HelmRepo{}
	}
	return repos, nil
}

func (m *Manager) AddRepo(ctx context.Context, in AddRepoInput) error {
	if err := ValidateRepoName(in.Name); err != nil {
		return err
	}
	if err := ValidateRepoURL(in.URL); err != nil {
		return err
	}
	args := []string{"repo", "add", strings.TrimSpace(in.Name), strings.TrimSpace(in.URL)}
	if strings.TrimSpace(in.Username) != "" {
		args = append(args, "--username", in.Username)
	}
	if in.Password != "" {
		args = append(args, "--password", in.Password)
	}
	if _, err := m.runHelm(ctx, 45*time.Second, args...); err != nil {
		return errcode.Wrap(errcode.Internal, err, "helm repo add 失败")
	}
	return nil
}

func (m *Manager) RemoveRepo(ctx context.Context, name string) error {
	if err := ValidateRepoName(name); err != nil {
		return err
	}
	if _, err := m.runHelm(ctx, 20*time.Second, "repo", "remove", strings.TrimSpace(name)); err != nil {
		return errcode.Wrap(errcode.Internal, err, "helm repo remove 失败")
	}
	return nil
}

func (m *Manager) UpdateRepos(ctx context.Context, name string) error {
	args := []string{"repo", "update"}
	if strings.TrimSpace(name) != "" {
		if err := ValidateRepoName(name); err != nil {
			return err
		}
		args = append(args, strings.TrimSpace(name))
	}
	if _, err := m.runHelm(ctx, 90*time.Second, args...); err != nil {
		return errcode.Wrap(errcode.Internal, err, "helm repo update 失败")
	}
	return nil
}

type helmSearchRow struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	AppVersion  string `json:"app_version"`
	Description string `json:"description"`
}

func (m *Manager) SearchCharts(ctx context.Context, repo, keyword string) ([]HelmChart, error) {
	query := strings.TrimSpace(keyword)
	if repo = strings.TrimSpace(repo); repo != "" {
		if err := ValidateRepoName(repo); err != nil {
			return nil, err
		}
		if query == "" {
			query = repo + "/"
		} else if !strings.Contains(query, "/") {
			query = repo + "/" + query
		}
	}
	args := []string{"search", "repo", "-o", "json"}
	if query != "" {
		args = append(args, query)
	}
	out, err := m.runHelm(ctx, 45*time.Second, args...)
	if err != nil {
		return nil, errcode.Wrap(errcode.Internal, err, "helm search repo 失败")
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "null" {
		return []HelmChart{}, nil
	}
	var rows []helmSearchRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, errcode.Wrap(errcode.Internal, err, "解析 helm search 结果失败")
	}
	charts := make([]HelmChart, 0, len(rows))
	for _, r := range rows {
		if repo != "" && !strings.HasPrefix(r.Name, repo+"/") {
			continue
		}
		charts = append(charts, HelmChart{
			Name:        r.Name,
			Version:     r.Version,
			AppVersion:  r.AppVersion,
			Description: r.Description,
		})
	}
	return charts, nil
}

func (m *Manager) CheckCLI(ctx context.Context) (bool, string) {
	out, err := m.runHelm(ctx, 8*time.Second, "version", "--short")
	if err != nil {
		return false, err.Error()
	}
	return true, strings.TrimSpace(out)
}
