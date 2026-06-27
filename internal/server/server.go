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
	mux.HandleFunc("GET /api/status", s.handleStatus)

	// 提供已上传的 logo 静态访问。
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/",
		http.FileServer(http.Dir(s.cfg.DataDir()+"/uploads"))))

	return withCORS(mux)
}
