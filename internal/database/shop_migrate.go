package database

import "github.com/RoyOfficial/sakura-finance-backend/internal/config"

// shopTableDDL 返回商城相关表的建表语句：分类、商品、购物车、订单、订单项、云服务器实例。
// 自增主键、金额、时间类型在三种方言间存在差异，因此分别处理。
func shopTableDDL(dbType config.DBType) []string {
	switch dbType {
	case config.DBMySQL:
		return []string{
			`CREATE TABLE IF NOT EXISTS categories (
	id          BIGINT AUTO_INCREMENT PRIMARY KEY,
	name        VARCHAR(64) NOT NULL,
	sort        INT NOT NULL DEFAULT 0,
	created_at  DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS products (
	id           BIGINT AUTO_INCREMENT PRIMARY KEY,
	category_id  BIGINT NOT NULL,
	name         VARCHAR(128) NOT NULL,
	description  TEXT,
	cpu          INT NOT NULL DEFAULT 1,
	ram          INT NOT NULL DEFAULT 1,
	disk         INT NOT NULL DEFAULT 20,
	bandwidth    INT NOT NULL DEFAULT 1,
	price        DECIMAL(14,2) NOT NULL DEFAULT 0,
	stock        INT NOT NULL DEFAULT 0,
	status       VARCHAR(8) NOT NULL DEFAULT 'on',
	created_at   DATETIME NOT NULL,
	updated_at   DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS cart_items (
	id          BIGINT AUTO_INCREMENT PRIMARY KEY,
	user_id     BIGINT NOT NULL,
	product_id  BIGINT NOT NULL,
	quantity    INT NOT NULL DEFAULT 1,
	created_at  DATETIME NOT NULL,
	UNIQUE KEY uq_cart_user_product (user_id, product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS orders (
	id            BIGINT AUTO_INCREMENT PRIMARY KEY,
	order_no      VARCHAR(40) NOT NULL UNIQUE,
	user_id       BIGINT NOT NULL,
	total_amount  DECIMAL(14,2) NOT NULL DEFAULT 0,
	status        VARCHAR(16) NOT NULL DEFAULT 'pending',
	pay_method    VARCHAR(32) NOT NULL DEFAULT '',
	created_at    DATETIME NOT NULL,
	updated_at    DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS order_items (
	id            BIGINT AUTO_INCREMENT PRIMARY KEY,
	order_id      BIGINT NOT NULL,
	product_id    BIGINT NOT NULL,
	product_name  VARCHAR(128) NOT NULL,
	price         DECIMAL(14,2) NOT NULL DEFAULT 0,
	quantity      INT NOT NULL DEFAULT 1
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
			`CREATE TABLE IF NOT EXISTS instances (
	id          BIGINT AUTO_INCREMENT PRIMARY KEY,
	user_id     BIGINT NOT NULL,
	order_id    BIGINT NOT NULL,
	product_id  BIGINT NOT NULL,
	name        VARCHAR(128) NOT NULL,
	cpu         INT NOT NULL DEFAULT 1,
	ram         INT NOT NULL DEFAULT 1,
	disk        INT NOT NULL DEFAULT 20,
	bandwidth   INT NOT NULL DEFAULT 1,
	status      VARCHAR(16) NOT NULL DEFAULT 'running',
	expires_at  DATETIME NOT NULL,
	created_at  DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}

	case config.DBPostgreSQL:
		return []string{
			`CREATE TABLE IF NOT EXISTS categories (
	id          BIGSERIAL PRIMARY KEY,
	name        VARCHAR(64) NOT NULL,
	sort        INT NOT NULL DEFAULT 0,
	created_at  TIMESTAMPTZ NOT NULL
);`,
			`CREATE TABLE IF NOT EXISTS products (
	id           BIGSERIAL PRIMARY KEY,
	category_id  BIGINT NOT NULL,
	name         VARCHAR(128) NOT NULL,
	description  TEXT,
	cpu          INT NOT NULL DEFAULT 1,
	ram          INT NOT NULL DEFAULT 1,
	disk         INT NOT NULL DEFAULT 20,
	bandwidth    INT NOT NULL DEFAULT 1,
	price        NUMERIC(14,2) NOT NULL DEFAULT 0,
	stock        INT NOT NULL DEFAULT 0,
	status       VARCHAR(8) NOT NULL DEFAULT 'on',
	created_at   TIMESTAMPTZ NOT NULL,
	updated_at   TIMESTAMPTZ NOT NULL
);`,
			`CREATE TABLE IF NOT EXISTS cart_items (
	id          BIGSERIAL PRIMARY KEY,
	user_id     BIGINT NOT NULL,
	product_id  BIGINT NOT NULL,
	quantity    INT NOT NULL DEFAULT 1,
	created_at  TIMESTAMPTZ NOT NULL,
	UNIQUE (user_id, product_id)
);`,
			`CREATE TABLE IF NOT EXISTS orders (
	id            BIGSERIAL PRIMARY KEY,
	order_no      VARCHAR(40) NOT NULL UNIQUE,
	user_id       BIGINT NOT NULL,
	total_amount  NUMERIC(14,2) NOT NULL DEFAULT 0,
	status        VARCHAR(16) NOT NULL DEFAULT 'pending',
	pay_method    VARCHAR(32) NOT NULL DEFAULT '',
	created_at    TIMESTAMPTZ NOT NULL,
	updated_at    TIMESTAMPTZ NOT NULL
);`,
			`CREATE TABLE IF NOT EXISTS order_items (
	id            BIGSERIAL PRIMARY KEY,
	order_id      BIGINT NOT NULL,
	product_id    BIGINT NOT NULL,
	product_name  VARCHAR(128) NOT NULL,
	price         NUMERIC(14,2) NOT NULL DEFAULT 0,
	quantity      INT NOT NULL DEFAULT 1
);`,
			`CREATE TABLE IF NOT EXISTS instances (
	id          BIGSERIAL PRIMARY KEY,
	user_id     BIGINT NOT NULL,
	order_id    BIGINT NOT NULL,
	product_id  BIGINT NOT NULL,
	name        VARCHAR(128) NOT NULL,
	cpu         INT NOT NULL DEFAULT 1,
	ram         INT NOT NULL DEFAULT 1,
	disk        INT NOT NULL DEFAULT 20,
	bandwidth   INT NOT NULL DEFAULT 1,
	status      VARCHAR(16) NOT NULL DEFAULT 'running',
	expires_at  TIMESTAMPTZ NOT NULL,
	created_at  TIMESTAMPTZ NOT NULL
);`,
		}

	default: // SQLite
		return []string{
			`CREATE TABLE IF NOT EXISTS categories (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT NOT NULL,
	sort        INTEGER NOT NULL DEFAULT 0,
	created_at  DATETIME NOT NULL
);`,
			`CREATE TABLE IF NOT EXISTS products (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	category_id  INTEGER NOT NULL,
	name         TEXT NOT NULL,
	description  TEXT,
	cpu          INTEGER NOT NULL DEFAULT 1,
	ram          INTEGER NOT NULL DEFAULT 1,
	disk         INTEGER NOT NULL DEFAULT 20,
	bandwidth    INTEGER NOT NULL DEFAULT 1,
	price        REAL NOT NULL DEFAULT 0,
	stock        INTEGER NOT NULL DEFAULT 0,
	status       TEXT NOT NULL DEFAULT 'on',
	created_at   DATETIME NOT NULL,
	updated_at   DATETIME NOT NULL
);`,
			`CREATE TABLE IF NOT EXISTS cart_items (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id     INTEGER NOT NULL,
	product_id  INTEGER NOT NULL,
	quantity    INTEGER NOT NULL DEFAULT 1,
	created_at  DATETIME NOT NULL,
	UNIQUE (user_id, product_id)
);`,
			`CREATE TABLE IF NOT EXISTS orders (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	order_no      TEXT NOT NULL UNIQUE,
	user_id       INTEGER NOT NULL,
	total_amount  REAL NOT NULL DEFAULT 0,
	status        TEXT NOT NULL DEFAULT 'pending',
	pay_method    TEXT NOT NULL DEFAULT '',
	created_at    DATETIME NOT NULL,
	updated_at    DATETIME NOT NULL
);`,
			`CREATE TABLE IF NOT EXISTS order_items (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	order_id      INTEGER NOT NULL,
	product_id    INTEGER NOT NULL,
	product_name  TEXT NOT NULL,
	price         REAL NOT NULL DEFAULT 0,
	quantity      INTEGER NOT NULL DEFAULT 1
);`,
			`CREATE TABLE IF NOT EXISTS instances (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id     INTEGER NOT NULL,
	order_id    INTEGER NOT NULL,
	product_id  INTEGER NOT NULL,
	name        TEXT NOT NULL,
	cpu         INTEGER NOT NULL DEFAULT 1,
	ram         INTEGER NOT NULL DEFAULT 1,
	disk        INTEGER NOT NULL DEFAULT 20,
	bandwidth   INTEGER NOT NULL DEFAULT 1,
	status      TEXT NOT NULL DEFAULT 'running',
	expires_at  DATETIME NOT NULL,
	created_at  DATETIME NOT NULL
);`,
		}
	}
}
