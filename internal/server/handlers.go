package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RoyOfficial/sakura-finance-backend/internal/config"
	"github.com/RoyOfficial/sakura-finance-backend/internal/database"
	"github.com/RoyOfficial/sakura-finance-backend/internal/store"

	"golang.org/x/crypto/bcrypt"
)

const maxLogoSize = 2 << 20 // 2MB，与前端校验一致

// handleStatus 返回初始化进度，便于前端或运维查询。
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{
		"initialized":  s.cfg.IsInitialized(),
		"adminCreated": s.cfg.IsAdminCreated(),
	})
}

// handleInitStatus 处理 GET /api/init：前端路由守卫据此判断是否已初始化。
// 仅当系统配置与管理员都完成时才视为已初始化，否则引导用户继续初始化流程。
func (s *Server) handleInitStatus(w http.ResponseWriter, r *http.Request) {
	initialized := s.cfg.IsInitialized() && s.cfg.IsAdminCreated()
	writeJSON(w, http.StatusOK, map[string]any{"initialized": initialized})
}

// handleInit 处理 POST /api/init：保存系统配置、连接数据库并建表。
func (s *Server) handleInit(w http.ResponseWriter, r *http.Request) {
	if s.cfg.IsInitialized() {
		fail(w, http.StatusConflict, "系统已初始化，无法重复配置")
		return
	}

	// 限制整个请求体大小，留出 logo 与表单字段空间。
	r.Body = http.MaxBytesReader(w, r.Body, maxLogoSize+1<<20)
	if err := r.ParseMultipartForm(maxLogoSize + 1<<20); err != nil {
		fail(w, http.StatusBadRequest, "表单解析失败，请检查上传内容")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	dbType := config.DBType(strings.TrimSpace(r.FormValue("dbType")))
	intro := strings.TrimSpace(r.FormValue("cloudProvider"))

	if name == "" {
		fail(w, http.StatusBadRequest, "系统名称不能为空")
		return
	}
	if !isValidDBType(dbType) {
		fail(w, http.StatusBadRequest, "不支持的数据库类型")
		return
	}

	cfg := config.SystemConfig{
		Name:   name,
		Intro:  intro,
		DBType: dbType,
	}

	// 非 SQLite 需要完整连接信息。
	if dbType != config.DBSQLite {
		cfg.DBHost = strings.TrimSpace(r.FormValue("dbHost"))
		cfg.DBPort = strings.TrimSpace(r.FormValue("dbPort"))
		cfg.DBUser = strings.TrimSpace(r.FormValue("dbUser"))
		cfg.DBPassword = r.FormValue("dbPassword")
		cfg.DBName = strings.TrimSpace(r.FormValue("dbName"))
		if cfg.DBHost == "" || cfg.DBPort == "" || cfg.DBUser == "" || cfg.DBName == "" {
			fail(w, http.StatusBadRequest, "请填写完整的数据库连接信息")
			return
		}
	}

	// 保存 logo（可选）。
	if logoPath, err := s.saveLogo(r); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	} else {
		cfg.LogoPath = logoPath
	}

	// 连接数据库并建表，失败则不写入配置，便于用户修正后重试。
	db, err := database.Open(cfg, s.cfg.DataDir())
	if err != nil {
		fail(w, http.StatusBadRequest, "数据库连接失败："+err.Error())
		return
	}
	if err := database.Migrate(db, cfg.DBType); err != nil {
		_ = db.Close()
		fail(w, http.StatusInternalServerError, "数据库初始化失败："+err.Error())
		return
	}

	cfg.Initialized = true
	if err := s.cfg.Save(cfg); err != nil {
		_ = db.Close()
		fail(w, http.StatusInternalServerError, "配置保存失败："+err.Error())
		return
	}

	s.AttachDB(db, store.New(db, cfg.DBType))
	ok(w, nil)
}

// handleCreateAdmin 处理 POST /api/admin：创建首个管理员账户。
func (s *Server) handleCreateAdmin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.IsInitialized() {
		fail(w, http.StatusConflict, "请先完成系统配置")
		return
	}
	if s.cfg.IsAdminCreated() {
		fail(w, http.StatusConflict, "管理员已创建，无法重复创建")
		return
	}

	st := s.getStore()
	if st == nil {
		fail(w, http.StatusInternalServerError, "数据库未就绪，请重启服务后重试")
		return
	}

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

	user := &store.User{
		Nickname: body.Nickname,
		Username: body.Username,
		Email:    body.Email,
		Password: string(hash),
		Role:     "admin",
	}
	if err := st.CreateUser(ctx, user); err != nil {
		if errors.Is(err, store.ErrUserExists) {
			fail(w, http.StatusConflict, "用户名或邮箱已被使用")
			return
		}
		fail(w, http.StatusInternalServerError, "创建管理员失败："+err.Error())
		return
	}

	// 标记管理员已创建。
	cfg := s.cfg.Get()
	cfg.AdminCreated = true
	if err := s.cfg.Save(cfg); err != nil {
		fail(w, http.StatusInternalServerError, "状态保存失败："+err.Error())
		return
	}

	ok(w, map[string]any{"userId": user.ID})
}

// saveLogo 保存上传的 logo 文件，返回相对访问路径；无文件时返回空字符串。
func (s *Server) saveLogo(r *http.Request) (string, error) {
	file, header, err := r.FormFile("logo")
	if errors.Is(err, http.ErrMissingFile) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("读取 logo 失败")
	}
	defer file.Close()

	if header.Size > maxLogoSize {
		return "", fmt.Errorf("Logo 大小不能超过 2MB")
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !isImageExt(ext) {
		return "", fmt.Errorf("Logo 必须是图片文件")
	}

	uploadDir := filepath.Join(s.cfg.DataDir(), "uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建上传目录")
	}

	dest := filepath.Join(uploadDir, "logo"+ext)
	out, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("保存 logo 失败")
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		return "", fmt.Errorf("写入 logo 失败")
	}
	return "/uploads/logo" + ext, nil
}

func isValidDBType(t config.DBType) bool {
	switch t {
	case config.DBSQLite, config.DBMySQL, config.DBPostgreSQL:
		return true
	}
	return false
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico":
		return true
	}
	return false
}
