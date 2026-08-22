package auth

import (
	"context"
)

// MVP_TODO: IAuthProvider 认证提供者插件接口（设计文档 Section 8.4）
// 支持 LDAP、OIDC、OAuth2、SAML、CAS 等外部身份认证源集成
type IAuthProvider interface {
	ProviderType() string

	Name() string

	Init(ctx context.Context, config map[string]interface{}) error

	Authenticate(ctx context.Context, username, password string) (*AuthUserInfo, error)

	AuthenticateByToken(ctx context.Context, token string) (*AuthUserInfo, error)

	RefreshToken(ctx context.Context, refreshToken string) (newAccessToken, newRefreshToken string, err error)

	Logout(ctx context.Context, token string) error

	ListUsers(ctx context.Context, keyword string, page, size int) ([]AuthUserInfo, int64, error)

	GetUser(ctx context.Context, userID string) (*AuthUserInfo, error)

	SyncUsers(ctx context.Context, sinceUnix int64) ([]AuthUserInfo, error)
}

type AuthUserInfo struct {
	UserID       string
	Username     string
	DisplayName  string
	Email        string
	Phone        string
	Department   string
	Position     string
	EmployeeID   string
	AvatarURL    string
	ExtraAttrs   map[string]string
	Groups       []string
	Roles        []string
}
