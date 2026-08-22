package kubeconfig

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/k8s-platform/console/pkg/errcode"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ParseResult struct {
	APIServer       string
	ClientCertExpAt *time.Time
	TokenExpAt      *time.Time
	RestConfig      *rest.Config
}

func Parse(raw []byte) (*ParseResult, error) {
	if len(raw) == 0 {
		return nil, errcode.New(errcode.KubeconfigInvalid, "kubeconfig 不能为空")
	}

	cfg, err := clientcmd.Load(raw)
	if err != nil {
		return nil, errcode.Wrap(errcode.KubeconfigInvalid, err,
			fmt.Sprintf("kubeconfig YAML 解析失败: %v", truncate(err.Error(), 120)))
	}

	if len(cfg.Clusters) == 0 {
		return nil, errcode.New(errcode.KubeconfigInvalid, "kubeconfig 缺失 clusters 配置")
	}
	if len(cfg.Contexts) == 0 {
		return nil, errcode.New(errcode.KubeconfigInvalid, "kubeconfig 缺失 contexts 配置")
	}
	if len(cfg.AuthInfos) == 0 {
		return nil, errcode.New(errcode.KubeconfigInvalid, "kubeconfig 缺失 users 配置")
	}

	ctxName := cfg.CurrentContext
	if ctxName == "" {
		for k := range cfg.Contexts {
			ctxName = k
			break
		}
	}
	ctx, ok := cfg.Contexts[ctxName]
	if !ok {
		return nil, errcode.New(errcode.KubeconfigInvalid,
			fmt.Sprintf("current-context %q 对应的 context 不存在", ctxName))
	}
	if ctx.Cluster == "" {
		return nil, errcode.New(errcode.KubeconfigInvalid, "context 未指定 cluster")
	}
	if ctx.AuthInfo == "" {
		return nil, errcode.New(errcode.KubeconfigInvalid, "context 未指定 user")
	}

	cluster, ok := cfg.Clusters[ctx.Cluster]
	if !ok {
		return nil, errcode.New(errcode.KubeconfigInvalid,
			fmt.Sprintf("cluster %q 不存在", ctx.Cluster))
	}
	if cluster.Server == "" {
		return nil, errcode.New(errcode.KubeconfigInvalid, "cluster.server 地址为空")
	}

	authInfo, ok := cfg.AuthInfos[ctx.AuthInfo]
	if !ok {
		return nil, errcode.New(errcode.KubeconfigInvalid,
			fmt.Sprintf("user %q 不存在", ctx.AuthInfo))
	}

	if !hasAuthMethod(authInfo) {
		return nil, errcode.New(errcode.KubeconfigInvalid,
			"kubeconfig user 未提供任何认证方式（token/client-cert/username）")
	}

	clientCfg := clientcmd.NewDefaultClientConfig(*cfg, &clientcmd.ConfigOverrides{
		CurrentContext: ctxName,
	})
	restCfg, err := clientCfg.ClientConfig()
	if err != nil {
		return nil, errcode.Wrap(errcode.KubeconfigInvalid, err,
			fmt.Sprintf("构建 rest.Config 失败: %v", truncate(err.Error(), 120)))
	}
	if restCfg.Timeout == 0 {
		restCfg.Timeout = 10 * time.Second
	}

	result := &ParseResult{
		APIServer:  cluster.Server,
		RestConfig: restCfg,
	}

	if certPEM := resolveCertPEM(authInfo); certPEM != nil {
		if expAt, err := extractCertExpire(certPEM); err == nil && !expAt.IsZero() {
			result.ClientCertExpAt = &expAt
		}
	}

	if authInfo.Token != "" {
		if expAt, err := extractJWTExpire(authInfo.Token); err == nil && !expAt.IsZero() {
			result.TokenExpAt = &expAt
		}
	}

	return result, nil
}

func hasAuthMethod(a *clientcmdapi.AuthInfo) bool {
	return a.Token != "" ||
		a.TokenFile != "" ||
		a.ClientCertificate != "" || a.ClientCertificateData != nil ||
		(a.Username != "" && a.Password != "") ||
		a.AuthProvider != nil || a.Exec != nil
}

func resolveCertPEM(a *clientcmdapi.AuthInfo) []byte {
	if len(a.ClientCertificateData) > 0 {
		return a.ClientCertificateData
	}
	return nil
}

func extractCertExpire(certPEM []byte) (time.Time, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, fmt.Errorf("invalid PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse cert: %w", err)
	}
	return cert.NotAfter, nil
}

func extractJWTExpire(tokenStr string) (time.Time, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	var claims jwt.RegisteredClaims
	_, _, err := parser.ParseUnverified(tokenStr, &claims)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse jwt: %w", err)
	}
	if claims.ExpiresAt != nil {
		return claims.ExpiresAt.Time, nil
	}
	return time.Time{}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
