package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/RoyOfficial/sakura-finance-backend/internal/config"
)

// ErrUserExists 用户名或邮箱已存在。
var ErrUserExists = errors.New("用户名或邮箱已存在")

// User 系统用户。
type User struct {
	ID        int64     `json:"id"`
	Nickname  string    `json:"nickname"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Password  string    `json:"-"` // 存储 bcrypt 哈希，不对外暴露
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Store 封装数据访问，按数据库方言改写占位符。
type Store struct {
	db     *sql.DB
	dbType config.DBType
}

func New(db *sql.DB, dbType config.DBType) *Store {
	return &Store{db: db, dbType: dbType}
}

// rebind 将 ? 占位符按方言改写：PostgreSQL 使用 $1、$2……
func (s *Store) rebind(query string) string {
	if s.dbType != config.DBPostgreSQL {
		return query
	}
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteString(fmt.Sprintf("$%d", n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// CountUsers 返回用户总数，用于判断是否已存在管理员。
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// CreateUser 创建用户；用户名或邮箱冲突时返回 ErrUserExists。
func (s *Store) CreateUser(ctx context.Context, u *User) error {
	now := time.Now()
	u.CreatedAt, u.UpdatedAt = now, now

	query := s.rebind(`INSERT INTO users
		(nickname, username, email, password, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)

	// PostgreSQL 通过 RETURNING 获取自增 id；MySQL/SQLite 用 LastInsertId。
	if s.dbType == config.DBPostgreSQL {
		query += " RETURNING id"
		err := s.db.QueryRowContext(ctx, query,
			u.Nickname, u.Username, u.Email, u.Password, u.Role, u.CreatedAt, u.UpdatedAt,
		).Scan(&u.ID)
		return wrapInsertErr(err)
	}

	res, err := s.db.ExecContext(ctx, query,
		u.Nickname, u.Username, u.Email, u.Password, u.Role, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return wrapInsertErr(err)
	}
	if id, err := res.LastInsertId(); err == nil {
		u.ID = id
	}
	return nil
}

// wrapInsertErr 将唯一约束冲突归一化为 ErrUserExists。
func wrapInsertErr(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate") {
		return ErrUserExists
	}
	return err
}
