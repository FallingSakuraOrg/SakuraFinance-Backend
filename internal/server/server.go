package server

import (
	"database/sql"
	"net/http"
	"sync"

	"github.com/RoyOfficial/sakura-finance-backend/internal/config"
	"github.com/RoyOfficial/sakura-finance-backend/internal/store"
)

// Server 持有应用共享状态。数据库连接在 /api/init 成功后才建立，
// 因此 db/store 受互斥锁保护，可在运行期被赋值。
type Server struct {
	cfg *config.Manager

	mu    sync.RWMutex
	db    *sql.DB
	store *store.Store
}

func New(cfg *config.Manager) *Server {
	return &Server{cfg: cfg}
}

// AttachDB 在初始化或启动时挂载数据库与对应 store。
func (s *Server) AttachDB(db *sql.DB, st *store.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
	s.store = st
}

func (s *Server) getStore() *store.Store {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store
}

// Routes 构建带 CORS 的路由（Go 1.22+ 方法路由）。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/init", s.handleInitStatus)
	mux.HandleFunc("POST /api/init", s.handleInit)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("POST /api/admin", s.handleCreateAdmin)
	mux.HandleFunc("POST /api/register", s.handleRegister)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("GET /api/me", s.requireAuth(s.handleMe))
	mux.HandleFunc("PUT /api/me", s.requireAuth(s.handleUpdateProfile))
	mux.HandleFunc("GET /api/status", s.handleStatus)

	// 公开商品浏览。
	mux.HandleFunc("GET /api/categories", s.handleListCategories)
	mux.HandleFunc("GET /api/products", s.handleListProducts)
	// 公开支付方式（仅 type/name，供前台展示可用网关）。
	mux.HandleFunc("GET /api/payment-methods", s.handlePublicPaymentMethods)

	// 用户购物车与订单（需登录）。
	mux.HandleFunc("GET /api/cart", s.requireAuth(s.handleGetCart))
	mux.HandleFunc("POST /api/cart", s.requireAuth(s.handleAddCart))
	mux.HandleFunc("PUT /api/cart/{id}", s.requireAuth(s.handleUpdateCart))
	mux.HandleFunc("DELETE /api/cart/{id}", s.requireAuth(s.handleRemoveCart))
	mux.HandleFunc("POST /api/checkout", s.requireAuth(s.handleCheckout))
	mux.HandleFunc("POST /api/recharge", s.requireAuth(s.handleRecharge))
	mux.HandleFunc("GET /api/orders", s.requireAuth(s.handleListOrders))
	mux.HandleFunc("GET /api/instances", s.requireAuth(s.handleListInstances))

	// 管理后台。登录入口公开（内部校验 slug + 管理员角色），其余需管理员权限。
	mux.HandleFunc("POST /api/admin/login", s.handleAdminLogin)
	mux.HandleFunc("GET /api/admin/slug", s.requireAdmin(s.handleAdminSlug))
	mux.HandleFunc("GET /api/admin/products", s.requireAdmin(s.handleAdminListProducts))
	mux.HandleFunc("POST /api/admin/products", s.requireAdmin(s.handleAdminCreateProduct))
	mux.HandleFunc("PUT /api/admin/products/{id}", s.requireAdmin(s.handleAdminUpdateProduct))
	mux.HandleFunc("DELETE /api/admin/products/{id}", s.requireAdmin(s.handleAdminDeleteProduct))
	mux.HandleFunc("POST /api/admin/categories", s.requireAdmin(s.handleAdminCreateCategory))
	mux.HandleFunc("PUT /api/admin/categories/{id}", s.requireAdmin(s.handleAdminUpdateCategory))
	mux.HandleFunc("DELETE /api/admin/categories/{id}", s.requireAdmin(s.handleAdminDeleteCategory))
	mux.HandleFunc("GET /api/admin/payment", s.requireAdmin(s.handleAdminGetPayment))
	mux.HandleFunc("PUT /api/admin/payment", s.requireAdmin(s.handleAdminUpdatePayment))
	mux.HandleFunc("GET /api/admin/admins", s.requireAdmin(s.handleAdminListAdmins))
	mux.HandleFunc("POST /api/admin/admins", s.requireAdmin(s.handleAdminCreateAdmin))
	mux.HandleFunc("GET /api/admin/users", s.requireAdmin(s.handleAdminListUsers))
	mux.HandleFunc("PUT /api/admin/users/{id}", s.requireAdmin(s.handleAdminUpdateUser))
	mux.HandleFunc("DELETE /api/admin/users/{id}", s.requireAdmin(s.handleAdminDeleteUser))
	mux.HandleFunc("PUT /api/admin/users/{id}/password", s.requireAdmin(s.handleAdminResetPassword))
	mux.HandleFunc("POST /api/admin/users/{id}/recharge", s.requireAdmin(s.handleAdminRecharge))

	// 提供已上传的 logo 静态访问。
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/",
		http.FileServer(http.Dir(s.cfg.DataDir()+"/uploads"))))

	return withCORS(mux)
}
