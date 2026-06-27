package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
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
// payMethod 为空或 "balance" 时走余额支付；否则需为已启用的支付网关类型
// （本阶段网关支付模拟成功，不动余额）。
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
	payMethod := body.PayMethod
	if payMethod == "" {
		payMethod = "balance"
	}
	// 网关支付需校验该网关已配置且启用。
	if payMethod != "balance" && !s.isGatewayEnabled(payMethod) {
		fail(w, http.StatusBadRequest, "该支付方式不可用")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	order, err := st.Checkout(ctx, uid, newOrderNo(), payMethod)
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
	if body.Nickname == "" {
		fail(w, http.StatusBadRequest, "昵称不能为空")
		return
	}
	if matched, _ := regexp.MatchString(`^[^\s@]+@[^\s@]+\.[^\s@]+$`, body.Email); !matched {
		fail(w, http.StatusBadRequest, "邮箱格式不正确")
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

// ---- 支付方式（公开）与充值 ----

// handlePublicPaymentMethods 处理 GET /api/payment-methods：
// 返回已启用支付网关的 type 与 name，绝不下发 account/secret。
func (s *Server) handlePublicPaymentMethods(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	list := make([]map[string]string, 0, len(cfg.PaymentMethods))
	for _, m := range cfg.PaymentMethods {
		if m.Enabled {
			list = append(list, map[string]string{"type": m.Type, "name": m.Name})
		}
	}
	ok(w, map[string]any{"paymentMethods": list})
}

// isGatewayEnabled 判断给定网关类型是否已配置并启用。
func (s *Server) isGatewayEnabled(payType string) bool {
	for _, m := range s.cfg.Get().PaymentMethods {
		if m.Enabled && m.Type == payType {
			return true
		}
	}
	return false
}

// handleRecharge 处理 POST /api/recharge：用户自助充值。
// 本阶段网关支付为模拟成功，校验通过后直接增加余额。
// TODO: 接入真实支付网关后，应改为创建充值订单 → 跳转支付 → 收到回调再加余额。
func (s *Server) handleRecharge(w http.ResponseWriter, r *http.Request) {
	uid, st, ok2 := s.authUserStore(w, r)
	if !ok2 {
		return
	}
	var body struct {
		Amount    float64 `json:"amount"`
		PayMethod string  `json:"payMethod"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Amount <= 0 {
		fail(w, http.StatusBadRequest, "充值金额必须大于 0")
		return
	}
	// 充值必须经由支付网关；无可用网关时引导用户联系管理员。
	if !s.isGatewayEnabled(body.PayMethod) {
		fail(w, http.StatusBadRequest, "请联系站点管理员手动充值")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	balance, err := st.AddBalance(ctx, uid, body.Amount)
	if err != nil {
		fail(w, http.StatusInternalServerError, "充值失败："+err.Error())
		return
	}
	ok(w, map[string]any{"balance": balance})
}
