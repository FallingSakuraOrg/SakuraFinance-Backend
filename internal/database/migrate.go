package database

import (
	"database/sql"
	"fmt"

	"github.com/RoyOfficial/sakura-finance-backend/internal/config"
)

// Migrate 创建应用所需的表结构，针对不同数据库方言使用对应 DDL。
func Migrate(db *sql.DB, dbType config.DBType) error {
	for _, stmt := range usersTableDDL(dbType) {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
}

// usersTableDDL 返回 users 表的建表语句。
// 自增主键语法在三种数据库间存在差异，因此分别处理。
func usersTableDDL(dbType config.DBType) []string {
	switch dbType {
	case config.DBMySQL:
		return []string{`
CREATE TABLE IF NOT EXISTS users (
	id           BIGINT AUTO_INCREMENT PRIMARY KEY,
	nickname     VARCHAR(64)  NOT NULL,
	username     VARCHAR(64)  NOT NULL UNIQUE,
	email        VARCHAR(128) NOT NULL UNIQUE,
	password     VARCHAR(255) NOT NULL,
	role         VARCHAR(16)  NOT NULL DEFAULT 'user',
	created_at   DATETIME     NOT NULL,
	updated_at   DATETIME     NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`}

	case config.DBPostgreSQL:
		return []string{`
CREATE TABLE IF NOT EXISTS users (
	id           BIGSERIAL    PRIMARY KEY,
	nickname     VARCHAR(64)  NOT NULL,
	username     VARCHAR(64)  NOT NULL UNIQUE,
	email        VARCHAR(128) NOT NULL UNIQUE,
	password     VARCHAR(255) NOT NULL,
	role         VARCHAR(16)  NOT NULL DEFAULT 'user',
	created_at   TIMESTAMPTZ  NOT NULL,
	updated_at   TIMESTAMPTZ  NOT NULL
);`}

	default: // SQLite
		return []string{`
CREATE TABLE IF NOT EXISTS users (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	nickname     TEXT NOT NULL,
	username     TEXT NOT NULL UNIQUE,
	email        TEXT NOT NULL UNIQUE,
	password     TEXT NOT NULL,
	role         TEXT NOT NULL DEFAULT 'user',
	created_at   DATETIME NOT NULL,
	updated_at   DATETIME NOT NULL
);`}
	}
}
