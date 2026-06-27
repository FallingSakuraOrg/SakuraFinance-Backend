package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RoyOfficial/sakura-finance-backend/internal/store"

	"github.com/google/uuid"
)

// ---- 公开商品浏览 ----

// handleListCategories 处理 GET /api/categories：返回全部分类。
func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	if st == nil {
		fail(w, http.StatusServiceUnavailable, "系统尚未初始化")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	list, err := st.ListCategories(ctx)
	if err != nil {
		fail(w, http.StatusInternalServerError, "查询失败："+err.Error())
		return
	}
	ok(w, map[string]any{"categories": list})
}

// handleListProducts 处理 GET /api/products?category=：仅返回上架商品。
func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	if st == nil {
		fail(w, http.StatusServiceUnavailable, "系统尚未初始化")
		return
	}
	var categoryID int64
	if v := strings.TrimSpace(r.URL.Query().Get("category")); v != "" {
		categoryID, _ = strconv.ParseInt(v, 10, 64)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	list, err := st.ListProducts(ctx, categoryID, true)
	if err != nil {
		fail(w, http.StatusInternalServerError, "查询失败："+err.Error())
		return
	}
	ok(w, map[string]any{"products": list})
}

// handleAdminListProducts 处理 GET /api/admin/products：返回全部商品（含下架）。
func (s *Server) handleAdminListProducts(w http.ResponseWriter, r *http.Request) {
	st := s.getStore()
	var categoryID int64
	if v := strings.TrimSpace(r.URL.Query().Get("category")); v != "" {
		categoryID, _ = strconv.ParseInt(v, 10, 64)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	list, err := st.ListProducts(ctx, categoryID, false)
	if err != nil {
		fail(w, http.StatusInternalServerError, "查询失败："+err.Error())
		return
	}
	ok(w, map[string]any{"products": list})
}

// ---- 购物车 ----

func (s *Server) handleGetCart(w http.ResponseWriter, r *http.Request) {
	uid, st, ok2 := s.authUserStore(w, r)
	if !ok2 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	list, err := st.ListCart(ctx, uid)
	if err != nil {
		fail(w, http.StatusInternalServerError, "查询失败："+err.Error())
		return
	}
	ok(w, map[string]any{"items": list})
}

func (s *Server) handleAddCart(w http.ResponseWriter, r *http.Request) {
	uid, st, ok2 := s.authUserStore(w, r)
	if !ok2 {
		return
	}
	var body struct {
		ProductID int64 `json:"productId"`
		Quantity  int   `json:"quantity"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.ProductID <= 0 {
		fail(w, http.StatusBadRequest, "商品无效")
		return
	}
	if body.Quantity <= 0 {
		body.Quantity = 1
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 校验商品存在且上架。
	p, err := st.GetProduct(ctx, body.ProductID)
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "商品不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "查询失败："+err.Error())
		return
	}
	if p.Status != "on" {
		fail(w, http.StatusBadRequest, "商品已下架")
		return
	}
	if err := st.UpsertCartItem(ctx, uid, body.ProductID, body.Quantity); err != nil {
		fail(w, http.StatusInternalServerError, "加入购物车失败："+err.Error())
		return
	}
	ok(w, nil)
}

func (s *Server) handleUpdateCart(w http.ResponseWriter, r *http.Request) {
	uid, st, ok2 := s.authUserStore(w, r)
	if !ok2 {
		return
	}
	pid, err := pathID(r, "id")
	if err != nil {
		fail(w, http.StatusBadRequest, "无效的商品 id")
		return
	}
	var body struct {
		Quantity int `json:"quantity"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if body.Quantity <= 0 {
		// 数量降到 0 视为移除。
		if err := st.RemoveCartItem(ctx, uid, pid); err != nil {
			fail(w, http.StatusInternalServerError, "更新失败："+err.Error())
			return
		}
		ok(w, nil)
		return
	}
	if err := st.UpdateCartQty(ctx, uid, pid, body.Quantity); err != nil {
		fail(w, http.StatusInternalServerError, "更新失败："+err.Error())
		return
	}
	ok(w, nil)
}

func (s *Server) handleRemoveCart(w http.ResponseWriter, r *http.Request) {
	uid, st, ok2 := s.authUserStore(w, r)
	if !ok2 {
		return
	}
	pid, err := pathID(r, "id")
	if err != nil {
		fail(w, http.StatusBadRequest, "无效的商品 id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := st.RemoveCartItem(ctx, uid, pid); err != nil {
		fail(w, http.StatusInternalServerError, "移除失败："+err.Error())
		return
	}
	ok(w, nil)
}

// ---- 结算 ----

// handleCheckout 处理 POST /api/checkout：由购物车生成订单。
// 本阶段支持余额支付（payMethod=balance）；选择支付网关时返回未接入提示。
func (s *Server) handleCheckout(w http.ResponseWriter, r *http.Request) {
	uid, st, ok2 := s.authUserStore(w, r)
	if !ok2 {
		return
	}
	var body struct {
		PayMethod string `json:"payMethod"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.PayMethod != "" && body.PayMethod != "balance" {
		fail(w, http.StatusBadRequest, "支付网关尚未接入，请使用余额支付")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	order, err := st.CheckoutByBalance(ctx, uid, newOrderNo())
	switch {
	case errors.Is(err, store.ErrEmptyCart):
		fail(w, http.StatusBadRequest, "购物车为空")
		return
	case errors.Is(err, store.ErrInsufficientBalance):
		fail(w, http.StatusBadRequest, "余额不足，请先充值")
		return
	case err != nil:
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	ok(w, map[string]any{"order": order})
}

func (s *Server) handleListOrders(w http.ResponseWriter, r *http.Request) {
	uid, st, ok2 := s.authUserStore(w, r)
	if !ok2 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	list, err := st.ListOrders(ctx, uid)
	if err != nil {
		fail(w, http.StatusInternalServerError, "查询失败："+err.Error())
		return
	}
	ok(w, map[string]any{"orders": list})
}

func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	uid, st, ok2 := s.authUserStore(w, r)
	if !ok2 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	list, err := st.ListInstances(ctx, uid)
	if err != nil {
		fail(w, http.StatusInternalServerError, "查询失败："+err.Error())
		return
	}
	ok(w, map[string]any{"instances": list})
}

// ---- 用户资料 ----

// handleUpdateProfile 处理 PUT /api/me：更新本人资料。
func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	uid, st, ok2 := s.authUserStore(w, r)
	if !ok2 {
		return
	}
	var body struct {
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
		QQ       string `json:"qq"`
		Phone    string `json:"phone"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	body.Nickname = strings.TrimSpace(body.Nickname)
	body.Email = strings.TrimSpace(body.Email)
	if body.Nickname == "" || body.Email == "" {
		fail(w, http.StatusBadRequest, "昵称和邮箱不能为空")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := st.UpdateProfile(ctx, uid, body.Nickname, body.Email,
		strings.TrimSpace(body.QQ), strings.TrimSpace(body.Phone)); err != nil {
		if errors.Is(err, store.ErrUserExists) {
			fail(w, http.StatusConflict, "邮箱已被使用")
			return
		}
		fail(w, http.StatusInternalServerError, "更新失败："+err.Error())
		return
	}
	user, err := st.GetByID(ctx, uid)
	if err != nil {
		fail(w, http.StatusInternalServerError, "获取用户信息失败")
		return
	}
	ok(w, map[string]any{"user": user})
}

// authUserStore 从上下文取用户 id 并取 store；任一缺失则已写出错误响应。
func (s *Server) authUserStore(w http.ResponseWriter, r *http.Request) (int64, *store.Store, bool) {
	claims, ok2 := userFromContext(r.Context())
	if !ok2 {
		fail(w, http.StatusUnauthorized, "未登录或登录已失效")
		return 0, nil, false
	}
	st := s.getStore()
	if st == nil {
		fail(w, http.StatusServiceUnavailable, "系统尚未初始化")
		return 0, nil, false
	}
	return claims.UserID, st, true
}

// newOrderNo 生成订单号：时间前缀 + 短随机串，便于排序与去重。
func newOrderNo() string {
	return fmt.Sprintf("%s%s", time.Now().Format("20060102150405"),
		strings.ToUpper(uuid.NewString()[:6]))
}
