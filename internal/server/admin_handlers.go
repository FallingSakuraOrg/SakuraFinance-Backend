package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RoyOfficial/sakura-finance-backend/internal/config"
	"github.com/RoyOfficial/sakura-finance-backend/internal/store"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// handleAdminLogin 处理 POST /api/admin/login：管理员登录。
// 需同时满足 slug 匹配、账户为 admin 角色且密码正确。
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	if st == nil {
		fail(w, http.StatusServiceUnavailable, "系统尚未初始化")
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Slug     string `json:"slug"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	// 入口后缀不匹配直接拒绝，不泄露更多信息。
	if strings.TrimSpace(body.Slug) != s.cfg.AdminSlug() {
		fail(w, http.StatusNotFound, "页面不存在")
		return
	}
	body.Username = strings.TrimSpace(body.Username)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user, err := st.GetByUsername(ctx, body.Username)
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
	if user.Role != "admin" {
		fail(w, http.StatusForbidden, "该账户无管理员权限")
		return
	}

	token, err := s.signToken(user.ID, user.Role)
	if err != nil {
		fail(w, http.StatusInternalServerError, "令牌签发失败")
		return
	}
	ok(w, map[string]any{"token": token, "user": user})
}

// handleAdminSlug 处理 GET /api/admin/slug：管理员查看当前登录入口后缀。
func (s *Server) handleAdminSlug(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"slug": s.cfg.AdminSlug()})
}

// ---- 分类管理 ----

func (s *Server) handleAdminCreateCategory(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	var body struct {
		Name string `json:"name"`
		Sort int    `json:"sort"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		fail(w, http.StatusBadRequest, "分类名称不能为空")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	c := &store.Category{Name: body.Name, Sort: body.Sort}
	if err := st.CreateCategory(ctx, c); err != nil {
		fail(w, http.StatusInternalServerError, "创建失败："+err.Error())
		return
	}
	ok(w, map[string]any{"category": c})
}

func (s *Server) handleAdminUpdateCategory(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, http.StatusBadRequest, "无效的分类 id")
		return
	}
	var body struct {
		Name string `json:"name"`
		Sort int    `json:"sort"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		fail(w, http.StatusBadRequest, "分类名称不能为空")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := st.UpdateCategory(ctx, &store.Category{ID: id, Name: body.Name, Sort: body.Sort}); err != nil {
		fail(w, http.StatusInternalServerError, "更新失败："+err.Error())
		return
	}
	ok(w, nil)
}

func (s *Server) handleAdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, http.StatusBadRequest, "无效的分类 id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := st.DeleteCategory(ctx, id); err != nil {
		fail(w, http.StatusInternalServerError, "删除失败："+err.Error())
		return
	}
	ok(w, nil)
}

// ---- 商品管理 ----

// productBody 创建/更新商品的请求体。
type productBody struct {
	CategoryID  int64   `json:"categoryId"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	CPU         int     `json:"cpu"`
	RAM         int     `json:"ram"`
	Disk        int     `json:"disk"`
	Bandwidth   int     `json:"bandwidth"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	Status      string  `json:"status"`
}

func (b productBody) toProduct() *store.Product {
	status := b.Status
	if status != "off" {
		status = "on"
	}
	return &store.Product{
		CategoryID: b.CategoryID, Name: strings.TrimSpace(b.Name), Description: b.Description,
		CPU: b.CPU, RAM: b.RAM, Disk: b.Disk, Bandwidth: b.Bandwidth,
		Price: b.Price, Stock: b.Stock, Status: status,
	}
}

func (s *Server) handleAdminCreateProduct(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	var body productBody
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if strings.TrimSpace(body.Name) == "" || body.CategoryID <= 0 {
		fail(w, http.StatusBadRequest, "商品名称与所属分类不能为空")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	p := body.toProduct()
	if err := st.CreateProduct(ctx, p); err != nil {
		fail(w, http.StatusInternalServerError, "创建失败："+err.Error())
		return
	}
	ok(w, map[string]any{"product": p})
}

func (s *Server) handleAdminUpdateProduct(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, http.StatusBadRequest, "无效的商品 id")
		return
	}
	var body productBody
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if strings.TrimSpace(body.Name) == "" || body.CategoryID <= 0 {
		fail(w, http.StatusBadRequest, "商品名称与所属分类不能为空")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	p := body.toProduct()
	p.ID = id
	if err := st.UpdateProduct(ctx, p); err != nil {
		fail(w, http.StatusInternalServerError, "更新失败："+err.Error())
		return
	}
	ok(w, nil)
}

func (s *Server) handleAdminDeleteProduct(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, http.StatusBadRequest, "无效的商品 id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := st.DeleteProduct(ctx, id); err != nil {
		fail(w, http.StatusInternalServerError, "删除失败："+err.Error())
		return
	}
	ok(w, nil)
}

// ---- 支付配置 ----

func (s *Server) handleAdminGetPayment(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	ok(w, map[string]any{"paymentMethods": cfg.PaymentMethods})
}

func (s *Server) handleAdminUpdatePayment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PaymentMethods []config.PaymentMethod `json:"paymentMethods"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	cfg := s.cfg.Get()
	cfg.PaymentMethods = body.PaymentMethods
	if err := s.cfg.Save(cfg); err != nil {
		fail(w, http.StatusInternalServerError, "保存失败："+err.Error())
		return
	}
	ok(w, nil)
}

// ---- 管理员账户 ----

func (s *Server) handleAdminListAdmins(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	admins, err := st.ListAdmins(ctx)
	if err != nil {
		fail(w, http.StatusInternalServerError, "查询失败："+err.Error())
		return
	}
	ok(w, map[string]any{"admins": admins})
}

func (s *Server) handleAdminCreateAdmin(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	var body struct {
		Nickname string `json:"nickname"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	body.Nickname = strings.TrimSpace(body.Nickname)
	body.Username = strings.TrimSpace(body.Username)
	body.Email = strings.TrimSpace(body.Email)
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

	u := &store.User{
		UUID:     uuid.NewString(),
		Nickname: body.Nickname,
		Username: body.Username,
		Email:    body.Email,
		Password: string(hash),
		Role:     "admin",
	}
	if err := st.CreateUser(ctx, u); err != nil {
		if errors.Is(err, store.ErrUserExists) {
			fail(w, http.StatusConflict, "用户名或邮箱已被使用")
			return
		}
		fail(w, http.StatusInternalServerError, "创建失败："+err.Error())
		return
	}
	ok(w, map[string]any{"userId": u.ID})
}

// pathID 从路由路径参数中解析 int64 id。
func pathID(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}

// ---- 用户管理与手动充值 ----

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	users, err := st.ListUsers(ctx)
	if err != nil {
		fail(w, http.StatusInternalServerError, "查询失败："+err.Error())
		return
	}
	ok(w, map[string]any{"users": users})
}

// handleAdminRecharge 处理 POST /api/admin/users/{id}/recharge：管理员手动给用户加余额。
func (s *Server) handleAdminRecharge(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, http.StatusBadRequest, "无效的用户 id")
		return
	}
	var body struct {
		Amount float64 `json:"amount"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Amount <= 0 {
		fail(w, http.StatusBadRequest, "充值金额必须大于 0")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	balance, err := st.AddBalance(ctx, id, body.Amount)
	if errors.Is(err, store.ErrUserNotFound) {
		fail(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "充值失败："+err.Error())
		return
	}
	ok(w, map[string]any{"balance": balance})
}

// handleAdminUpdateUser 处理 PUT /api/admin/users/{id}：修改用户昵称与用户名。
func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, http.StatusBadRequest, "无效的用户 id")
		return
	}
	var body struct {
		Nickname string `json:"nickname"`
		Username string `json:"username"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	body.Nickname = strings.TrimSpace(body.Nickname)
	body.Username = strings.TrimSpace(body.Username)
	if body.Nickname == "" || body.Username == "" {
		fail(w, http.StatusBadRequest, "昵称和用户名不能为空")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	err = st.AdminUpdateUser(ctx, id, body.Nickname, body.Username)
	if errors.Is(err, store.ErrUserNotFound) {
		fail(w, http.StatusNotFound, "用户不存在")
		return
	}
	if errors.Is(err, store.ErrUserExists) {
		fail(w, http.StatusConflict, "用户名已被使用")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "更新失败："+err.Error())
		return
	}
	ok(w, nil)
}

// handleAdminResetPassword 处理 PUT /api/admin/users/{id}/password：重置用户密码。
func (s *Server) handleAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, http.StatusBadRequest, "无效的用户 id")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
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

	err = st.UpdatePassword(ctx, id, string(hash))
	if errors.Is(err, store.ErrUserNotFound) {
		fail(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "重置失败："+err.Error())
		return
	}
	ok(w, nil)
}

// handleAdminDeleteUser 处理 DELETE /api/admin/users/{id}：删除用户。
func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, http.StatusBadRequest, "无效的用户 id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	err = st.DeleteUser(ctx, id)
	if errors.Is(err, store.ErrUserNotFound) {
		fail(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "删除失败："+err.Error())
		return
	}
	ok(w, nil)
}
