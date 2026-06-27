package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/RoyOfficial/sakura-finance-backend/internal/store"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// handleRegister 处理 POST /api/register：注册普通用户，UUID 由后端生成。
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	if st == nil {
		fail(w, http.StatusServiceUnavailable, "系统尚未初始化")
		return
	}

	var body struct {
		Nickname string `json:"nickname"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		QQ       string `json:"qq"`
		Phone    string `json:"phone"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	body.Nickname = strings.TrimSpace(body.Nickname)
	body.Username = strings.TrimSpace(body.Username)
	body.Email = strings.TrimSpace(body.Email)
	body.QQ = strings.TrimSpace(body.QQ)
	body.Phone = strings.TrimSpace(body.Phone)

	if body.Nickname == "" || body.Username == "" || body.Email == "" {
		fail(w, http.StatusBadRequest, "昵称、用户名和邮箱不能为空")
		return
	}
	if len(body.Password) < 6 {
		fail(w, http.StatusBadRequest, "密码至少 6 位")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(w, http.StatusInternalServerError, "密码加密失败")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user := &store.User{
		UUID:     uuid.NewString(),
		Nickname: body.Nickname,
		Username: body.Username,
		Email:    body.Email,
		Password: string(hash),
		Role:     "user",
		QQ:       body.QQ,
		Phone:    body.Phone,
	}
	if err := st.CreateUser(ctx, user); err != nil {
		if errors.Is(err, store.ErrUserExists) {
			fail(w, http.StatusConflict, "用户名或邮箱已被使用")
			return
		}
		fail(w, http.StatusInternalServerError, "注册失败："+err.Error())
		return
	}

	token, err := s.signToken(user.ID, user.Role)
	if err != nil {
		fail(w, http.StatusInternalServerError, "令牌签发失败")
		return
	}
	ok(w, map[string]any{"token": token, "user": user})
}

// handleLogin 处理 POST /api/login：普通用户登录，仅允许 role=user。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	if st == nil {
		fail(w, http.StatusServiceUnavailable, "系统尚未初始化")
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	body.Username = strings.TrimSpace(body.Username)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user, err := st.GetByUsername(ctx, body.Username)
	// 用户不存在与密码错误返回相同提示，避免泄露用户名是否存在。
	if errors.Is(err, store.ErrUserNotFound) {
		fail(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "登录失败："+err.Error())
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)) != nil {
		fail(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	// 此接口仅供普通用户登录，管理员请走管理员入口。
	if user.Role != "user" {
		fail(w, http.StatusForbidden, "请使用管理员入口登录")
		return
	}

	token, err := s.signToken(user.ID, user.Role)
	if err != nil {
		fail(w, http.StatusInternalServerError, "令牌签发失败")
		return
	}
	ok(w, map[string]any{"token": token, "user": user})
}

// handleMe 处理 GET /api/me：返回当前登录用户信息。
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims, ok2 := userFromContext(r.Context())
	if !ok2 {
		fail(w, http.StatusUnauthorized, "未登录或登录已失效")
		return
	}
	st := s.getStore()
	if st == nil {
		fail(w, http.StatusServiceUnavailable, "系统尚未初始化")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user, err := st.GetByID(ctx, claims.UserID)
	if errors.Is(err, store.ErrUserNotFound) {
		fail(w, http.StatusUnauthorized, "用户不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "获取用户信息失败")
		return
	}
	ok(w, map[string]any{"user": user})
}
