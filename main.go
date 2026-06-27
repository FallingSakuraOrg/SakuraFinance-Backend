package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RoyOfficial/sakura-finance-backend/internal/config"
	"github.com/RoyOfficial/sakura-finance-backend/internal/database"
	"github.com/RoyOfficial/sakura-finance-backend/internal/server"
	"github.com/RoyOfficial/sakura-finance-backend/internal/store"
)

func main() {
	addr := envOr("SAKURA_ADDR", ":8080")
	dataDir := envOr("SAKURA_DATA_DIR", "data")

	cfgMgr, err := config.NewManager(dataDir)
	if err != nil {
		log.Fatalf("初始化配置失败: %v", err)
	}

	srv := server.New(cfgMgr)

	// 若系统此前已初始化，启动时直接重连数据库，无需重新走 /api/init。
	if cfgMgr.IsInitialized() {
		cfg := cfgMgr.Get()
		db, err := database.Open(cfg, cfgMgr.DataDir())
		if err != nil {
			log.Fatalf("重连数据库失败: %v", err)
		}
		if err := database.Migrate(db, cfg.DBType); err != nil {
			log.Fatalf("数据库迁移失败: %v", err)
		}
		srv.AttachDB(db, store.New(db, cfg.DBType))
		log.Printf("已加载现有配置: %s (数据库: %s)", cfg.Name, cfg.DBType)
		if cfg.AdminSlug != "" {
			log.Printf("管理后台登录入口: /admin/%s/login", cfg.AdminSlug)
		}
	} else {
		log.Print("系统尚未初始化，等待前端完成 /api/init")
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 优雅关闭。
	go func() {
		log.Printf("SakuraFinance 后端已启动，监听 %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务异常退出: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Print("正在关闭服务……")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("关闭超时: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
