package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/k8s-platform/console/internal/config"
	"github.com/k8s-platform/console/internal/models"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Scope struct {
	ScopeType     models.ScopeType `json:"scope_type"`
	ClusterCode   string           `json:"cluster_code,omitempty"`
	Namespace     string           `json:"namespace,omitempty"`
}

type PermissionTree struct {
	Perms            map[string]struct{} `json:"perms"`
	Scopes           []Scope             `json:"scopes"`
	IsPlatformAdmin  bool                `json:"is_platform_admin"`
}

func (pt *PermissionTree) HasPerm(code string) bool {
	if pt.IsPlatformAdmin {
		return true
	}
	if _, ok := pt.Perms["*:*"]; ok {
		return true
	}
	if _, ok := pt.Perms[code]; ok {
		return true
	}
	for i := 0; i < len(code); i++ {
		if code[i] == ':' {
			moduleWildcard := code[:i] + ":*"
			if _, ok := pt.Perms[moduleWildcard]; ok {
				return true
			}
			break
		}
	}
	return false
}

func (pt *PermissionTree) PermsSlice() []string {
	s := make([]string, 0, len(pt.Perms))
	for k := range pt.Perms {
		s = append(s, k)
	}
	return s
}

type LoginResult struct {
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int64  `json:"expires_in"`
	User            *UserBrief `json:"user"`
}

type UserBrief struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
}

type Service struct {
	DB       *gorm.DB
	Redis    *redis.Client
	JWT      *JWTManager
	Config   *config.AuthConfig
	Log      *logger.Logger
}

const (
	rbacUserCacheKeyFmt = "rbac:user:%d"
	rbacCacheTTL        = time.Hour
)

func NewService(db *gorm.DB, rdb *redis.Client, cfg *config.AuthConfig, log *logger.Logger) *Service {
	return &Service{
		DB:     db,
		Redis:  rdb,
		JWT:    NewJWTManager(cfg),
		Config: cfg,
		Log:    log,
	}
}

func (s *Service) cacheKey(userID uint64) string {
	return fmt.Sprintf(rbacUserCacheKeyFmt, userID)
}

func (s *Service) Login(ctx context.Context, username, password, clientIP string) (*LoginResult, *errcode.Error) {
	var user models.SysUser
	if err := s.DB.Where("username = ?", username).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errcode.New(errcode.Unauthenticated, "用户名或密码错误")
		}
		s.Log.With("trace_id", traceID(ctx)).Errorw("login query user error", "err", err)
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}

	if user.Status != models.UserStatusEnabled {
		return nil, errcode.New(errcode.Unauthenticated, "账号已被禁用或锁定")
	}

	if !user.CheckPassword(password) {
		return nil, errcode.New(errcode.Unauthenticated, "用户名或密码错误")
	}

	now := time.Now()
	s.DB.Model(&user).Updates(map[string]interface{}{
		"last_login_at": &now,
		"last_login_ip": clientIP,
	})

	pt, err := s.loadPermissionTree(ctx, user.ID)
	if err != nil {
		return nil, errcode.Wrap(errcode.Internal, err, "加载权限树失败")
	}

	scopes := make([]string, 0, len(pt.Scopes))
	for _, sc := range pt.Scopes {
		switch sc.ScopeType {
		case models.ScopePlatform:
			scopes = append(scopes, "platform")
		case models.ScopeCluster:
			scopes = append(scopes, "cluster:"+sc.ClusterCode)
		case models.ScopeNamespace:
			scopes = append(scopes, "cluster:"+sc.ClusterCode+":"+sc.Namespace)
		}
	}

	accessToken, refreshToken, err := s.JWT.IssueTokenPair(user.ID, user.Username, scopes)
	if err != nil {
		s.Log.With("trace_id", traceID(ctx)).Errorw("issue token error", "err", err)
		return nil, errcode.Wrap(errcode.Internal, err, "生成 Token 失败")
	}

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.JWT.AccessTTL().Seconds()),
		User: &UserBrief{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Email:       user.Email,
		},
	}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*LoginResult, *errcode.Error) {
	claims, err := s.JWT.ParseToken(refreshToken)
	if err != nil {
		return nil, errcode.New(errcode.Unauthenticated, "Refresh Token 无效或已过期")
	}
	if claims.Type != RefreshToken {
		return nil, errcode.New(errcode.Unauthenticated, "Token 类型错误")
	}

	var user models.SysUser
	if err := s.DB.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errcode.New(errcode.Unauthenticated, "用户不存在")
		}
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}
	if user.Status != models.UserStatusEnabled {
		return nil, errcode.New(errcode.Unauthenticated, "账号已被禁用或锁定")
	}

	newAccess, err := s.JWT.IssueAccessToken(user.ID, user.Username, claims.Scopes)
	if err != nil {
		return nil, errcode.Wrap(errcode.Internal, err, "生成 Token 失败")
	}
	newRefresh, err := s.JWT.IssueRefreshToken(user.ID, user.Username, claims.Scopes)
	if err != nil {
		return nil, errcode.Wrap(errcode.Internal, err, "生成 Refresh Token 失败")
	}

	return &LoginResult{
		AccessToken:  newAccess,
		RefreshToken: newRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.JWT.AccessTTL().Seconds()),
		User: &UserBrief{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Email:       user.Email,
		},
	}, nil
}

func (s *Service) GetUserPermissionTree(ctx context.Context, userID uint64) (*PermissionTree, *errcode.Error) {
	cached, err := s.getCachedPermissionTree(ctx, userID)
	if err == nil && cached != nil {
		return cached, nil
	}
	if err != nil && err != redis.Nil {
		s.Log.With("trace_id", traceID(ctx)).Warnw("get rbac cache error", "user_id", userID, "err", err)
	}

	pt, loadErr := s.loadPermissionTree(ctx, userID)
	if loadErr != nil {
		return nil, errcode.Wrap(errcode.Internal, loadErr)
	}

	if setErr := s.setCachedPermissionTree(ctx, userID, pt); setErr != nil {
		s.Log.With("trace_id", traceID(ctx)).Warnw("set rbac cache error", "user_id", userID, "err", setErr)
	}
	return pt, nil
}

func (s *Service) loadPermissionTree(ctx context.Context, userID uint64) (*PermissionTree, error) {
	type permRow struct {
		PermissionCode  string
		ScopeType       models.ScopeType
		ScopeCluster    string
		ScopeNamespace  string
		RoleCode        string
	}
	var rows []permRow

	err := s.DB.Table("sys_user_role ur").
		Select("rp.permission_code, ur.scope_type, ur.scope_cluster_code, ur.scope_namespace, r.code as role_code").
		Joins("LEFT JOIN sys_role r ON ur.role_id = r.id").
		Joins("LEFT JOIN sys_role_permission rp ON ur.role_id = rp.role_id").
		Where("ur.user_id = ? AND r.status = 1", userID).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query user permission tree: %w", err)
	}

	pt := &PermissionTree{
		Perms:  make(map[string]struct{}),
		Scopes: make([]Scope, 0),
	}

	scopeSet := make(map[string]struct{})
	for _, row := range rows {
		if row.PermissionCode != "" {
			pt.Perms[row.PermissionCode] = struct{}{}
		}
		scopeKey := string(row.ScopeType) + "|" + row.ScopeCluster + "|" + row.ScopeNamespace
		if _, ok := scopeSet[scopeKey]; !ok {
			scopeSet[scopeKey] = struct{}{}
			pt.Scopes = append(pt.Scopes, Scope{
				ScopeType:   row.ScopeType,
				ClusterCode: row.ScopeCluster,
				Namespace:   row.ScopeNamespace,
			})
		}
		if row.ScopeType == models.ScopePlatform {
			pt.IsPlatformAdmin = true
		}
	}

	return pt, nil
}

func (s *Service) getCachedPermissionTree(ctx context.Context, userID uint64) (*PermissionTree, error) {
	key := s.cacheKey(userID)
	data, err := s.Redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var pt PermissionTree
	if err := json.Unmarshal([]byte(data), &pt); err != nil {
		return nil, err
	}
	return &pt, nil
}

func (s *Service) setCachedPermissionTree(ctx context.Context, userID uint64, pt *PermissionTree) error {
	key := s.cacheKey(userID)
	data, err := json.Marshal(pt)
	if err != nil {
		return err
	}
	return s.Redis.Set(ctx, key, data, rbacCacheTTL).Err()
}

func (s *Service) InvalidateUserPermissionCache(ctx context.Context, userID uint64) error {
	key := s.cacheKey(userID)
	return s.Redis.Del(ctx, key).Err()
}

func traceID(ctx context.Context) string {
	if tid, ok := ctx.Value("trace_id").(string); ok {
		return tid
	}
	return ""
}
