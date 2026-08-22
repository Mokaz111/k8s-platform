package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/k8s-platform/console/internal/config"
)

type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

type CustomClaims struct {
	UserID   uint64   `json:"user_id"`
	Username string   `json:"username"`
	Scopes   []string `json:"scopes,omitempty"`
	Type     TokenType `json:"type"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret        []byte
	issuer        string
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewJWTManager(cfg *config.AuthConfig) *JWTManager {
	return &JWTManager{
		secret:     []byte(cfg.JWTSecret),
		issuer:     cfg.JWTIssuer,
		accessTTL:  time.Duration(cfg.AccessTokenTTL) * time.Second,
		refreshTTL: time.Duration(cfg.RefreshTokenTTL) * time.Second,
	}
}

func (m *JWTManager) IssueTokenPair(userID uint64, username string, scopes []string) (accessToken string, refreshToken string, err error) {
	accessToken, err = m.signToken(userID, username, scopes, AccessToken, m.accessTTL)
	if err != nil {
		return "", "", err
	}
	refreshToken, err = m.signToken(userID, username, scopes, RefreshToken, m.refreshTTL)
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

func (m *JWTManager) IssueAccessToken(userID uint64, username string, scopes []string) (string, error) {
	return m.signToken(userID, username, scopes, AccessToken, m.accessTTL)
}

func (m *JWTManager) IssueRefreshToken(userID uint64, username string, scopes []string) (string, error) {
	return m.signToken(userID, username, scopes, RefreshToken, m.refreshTTL)
}

func (m *JWTManager) signToken(userID uint64, username string, scopes []string, tokenType TokenType, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := CustomClaims{
		UserID:   userID,
		Username: username,
		Scopes:   scopes,
		Type:     tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *JWTManager) ParseToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

func (m *JWTManager) ExtractToken(c *gin.Context) (string, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return "", errors.New("authorization header is empty")
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", errors.New("authorization header format must be Bearer <token>")
	}
	if parts[1] == "" {
		return "", errors.New("token is empty")
	}
	return parts[1], nil
}

func (m *JWTManager) AccessTTL() time.Duration {
	return m.accessTTL
}

func (m *JWTManager) RefreshTTL() time.Duration {
	return m.refreshTTL
}
