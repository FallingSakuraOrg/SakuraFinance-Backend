package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/RoyOfficial/sakura-finance-backend/internal/config"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Open 根据系统配置建立数据库连接并验证可达性。
// SQLite 文件存放在 dataDir 下，无需用户提供连接信息。
func Open(cfg config.SystemConfig, dataDir string) (*sql.DB, error) {
	driver, dsn, err := dsnFor(cfg, dataDir)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}
	return db, nil
}

// dsnFor 返回 database/sql 驱动名与对应 DSN。
func dsnFor(cfg config.SystemConfig, dataDir string) (driver, dsn string, err error) {
	switch cfg.DBType {
	case config.DBSQLite:
		path := filepath.Join(dataDir, "sakura.db")
		// _pragma=busy_timeout 减少并发写锁冲突，foreign_keys=on 保证外键约束。
		return "sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path), nil

	case config.DBMySQL:
		// parseTime=true 让 DATETIME 映射为 time.Time。
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
			cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
		return "mysql", dsn, nil

	case config.DBPostgreSQL:
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			url.QueryEscape(cfg.DBUser), url.QueryEscape(cfg.DBPassword),
			cfg.DBHost, cfg.DBPort, cfg.DBName)
		return "pgx", dsn, nil

	default:
		return "", "", fmt.Errorf("不支持的数据库类型: %s", cfg.DBType)
	}
}
