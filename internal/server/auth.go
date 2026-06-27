package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// tokenTTL 登录态有效期。
const tokenTTL = 7 * 24 * time.Hour

// ctxUserKey 用于在请求上下文中传递已认证用户信息。
type ctxUserKey struct{}

// authClaims JWT 载荷：用户 id、角色与标准声明。
type authClaims struct {
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// randomSecret 生成随机的十六进制密钥，用作 JWT 签名密钥。
func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// randomSlug 生成 URL 安全的随机短串，用作管理后台登录入口后缀。
func randomSlug() (string, error) {
	b := make([]byte, 9) // 9 字节 -> base64url 12 字符，无填充
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// signToken 为指定用户签发 JWT。
func (s *Server) signToken(userID int64, role string) (string, error) {
	now := time.Now()
	claims := authClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret()))
}

// parseToken 校验并解析 JWT，返回其载荷。
func (s *Server) parseToken(tokenStr string) (*authClaims, error) {
	claims := &authClaims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(s.cfg.JWTSecret()), nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// requireAuth 是认证中间件：校验 Authorization: Bearer <token>，
// 通过后将用户载荷写入请求上下文。
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			fail(w, http.StatusUnauthorized, "未登录或登录已失效")
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := s.parseToken(tokenStr)
		if err != nil {
			fail(w, http.StatusUnauthorized, "未登录或登录已失效")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserKey{}, claims)
		next(w, r.WithContext(ctx))
	}
}

// userFromContext 从上下文取出已认证用户载荷。
func userFromContext(ctx context.Context) (*authClaims, bool) {
	claims, ok := ctx.Value(ctxUserKey{}).(*authClaims)
	return claims, ok
}

// requireAdmin 在 requireAuth 基础上要求 admin 角色，否则 403。
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromContext(r.Context())
		if !ok || claims.Role != "admin" {
			fail(w, http.StatusForbidden, "需要管理员权限")
			return
		}
		next(w, r)
	})
}
