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

// ErrNotFound 通用的记录未找到错误，供商品、订单等实体复用。
var ErrNotFound = errors.New("记录不存在")

// normalizeNotFound 将 sql.ErrNoRows 归一化为 ErrNotFound，便于上层判断。
// 适用于泛型扫描后返回 (T, error) 的场景。
func normalizeNotFound[T any](v T, err error) (T, error) {
	if errors.Is(err, sql.ErrNoRows) {
		var zero T
		return zero, ErrNotFound
	}
	return v, err
}

// User 系统用户。
type User struct {
	ID        int64     `json:"id"`
	UUID      string    `json:"uuid"`
	Nickname  string    `json:"nickname"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Password  string    `json:"-"` // 存储 bcrypt 哈希，不对外暴露
	Role      string    `json:"role"`
	Balance   float64   `json:"balance"`
	QQ        string    `json:"qq"`
	Phone     string    `json:"phone"`
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

// execInsert 执行插入并回填自增主键。query 使用 ? 占位符（内部已 rebind），
// PostgreSQL 追加 RETURNING id，其余方言用 LastInsertId。
func (s *Store) execInsert(ctx context.Context, q execer, id *int64, query string, args ...any) error {
	query = s.rebind(query)
	if s.dbType == config.DBPostgreSQL {
		return q.QueryRowContext(ctx, query+" RETURNING id", args...).Scan(id)
	}
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if v, err := res.LastInsertId(); err == nil {
		*id = v
	}
	return nil
}

// execer 抽象 *sql.DB 与 *sql.Tx 的公共方法，便于事务内外复用。
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// placeholders 返回 n 个以逗号分隔的 ? 占位符，如 "?,?,?"，供 IN 查询拼接。
// 结果仍需经 rebind 处理以适配 PostgreSQL。
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
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
		(uuid, nickname, username, email, password, role, balance, qq, phone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	// PostgreSQL 通过 RETURNING 获取自增 id；MySQL/SQLite 用 LastInsertId。
	if s.dbType == config.DBPostgreSQL {
		query += " RETURNING id"
		err := s.db.QueryRowContext(ctx, query,
			u.UUID, u.Nickname, u.Username, u.Email, u.Password, u.Role,
			u.Balance, u.QQ, u.Phone, u.CreatedAt, u.UpdatedAt,
		).Scan(&u.ID)
		return wrapInsertErr(err)
	}

	res, err := s.db.ExecContext(ctx, query,
		u.UUID, u.Nickname, u.Username, u.Email, u.Password, u.Role,
		u.Balance, u.QQ, u.Phone, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return wrapInsertErr(err)
	}
	if id, err := res.LastInsertId(); err == nil {
		u.ID = id
	}
	return nil
}

// ErrUserNotFound 未找到用户。
var ErrUserNotFound = errors.New("用户不存在")

const userColumns = `id, uuid, nickname, username, email, password, role, balance, qq, phone, created_at, updated_at`

// scanUser 按 userColumns 顺序扫描一行到 User。
func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	err := row.Scan(
		&u.ID, &u.UUID, &u.Nickname, &u.Username, &u.Email, &u.Password,
		&u.Role, &u.Balance, &u.QQ, &u.Phone, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetByUsername 按用户名查询用户，未找到返回 ErrUserNotFound。
func (s *Store) GetByUsername(ctx context.Context, username string) (*User, error) {
	query := s.rebind("SELECT " + userColumns + " FROM users WHERE username = ?")
	return scanUser(s.db.QueryRowContext(ctx, query, username))
}

// GetByID 按 id 查询用户，未找到返回 ErrUserNotFound。
func (s *Store) GetByID(ctx context.Context, id int64) (*User, error) {
	query := s.rebind("SELECT " + userColumns + " FROM users WHERE id = ?")
	return scanUser(s.db.QueryRowContext(ctx, query, id))
}

// UpdateProfile 更新用户本人可改的资料：昵称、邮箱、QQ、手机号。
func (s *Store) UpdateProfile(ctx context.Context, id int64, nickname, email, qq, phone string) error {
	query := s.rebind(`UPDATE users SET nickname = ?, email = ?, qq = ?, phone = ?, updated_at = ?
		WHERE id = ?`)
	_, err := s.db.ExecContext(ctx, query, nickname, email, qq, phone, time.Now(), id)
	return wrapInsertErr(err) // 邮箱唯一冲突归一化为 ErrUserExists
}

// ListAdmins 返回全部管理员账户。
func (s *Store) ListAdmins(ctx context.Context) ([]*User, error) {
	query := s.rebind("SELECT " + userColumns + " FROM users WHERE role = ? ORDER BY id ASC")
	rows, err := s.db.QueryContext(ctx, query, "admin")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

// ListUsers 返回全部普通用户账户（供管理员查看与充值）。
func (s *Store) ListUsers(ctx context.Context) ([]*User, error) {
	query := s.rebind("SELECT " + userColumns + " FROM users WHERE role = ? ORDER BY id DESC")
	rows, err := s.db.QueryContext(ctx, query, "user")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

// AddBalance 给用户增加余额（amount 为正数），返回更新后的余额。
// 用于管理员手动充值与用户自助充值（模拟支付成功）。
func (s *Store) AddBalance(ctx context.Context, userID int64, amount float64) (float64, error) {
	query := s.rebind("UPDATE users SET balance = balance + ?, updated_at = ? WHERE id = ?")
	res, err := s.db.ExecContext(ctx, query, amount, time.Now(), userID)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrUserNotFound
	}
	u, err := s.GetByID(ctx, userID)
	if err != nil {
		return 0, err
	}
	return u.Balance, nil
}

// AdminUpdateUser 管理员修改用户的昵称与用户名。
// 用户名冲突时归一化为 ErrUserExists；用户不存在返回 ErrUserNotFound。
func (s *Store) AdminUpdateUser(ctx context.Context, id int64, nickname, username string) error {
	query := s.rebind("UPDATE users SET nickname = ?, username = ?, updated_at = ? WHERE id = ?")
	res, err := s.db.ExecContext(ctx, query, nickname, username, time.Now(), id)
	if err != nil {
		return wrapInsertErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdatePassword 重置用户密码（传入已加密的 bcrypt 哈希）。
func (s *Store) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	query := s.rebind("UPDATE users SET password = ?, updated_at = ? WHERE id = ?")
	res, err := s.db.ExecContext(ctx, query, passwordHash, time.Now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// DeleteUser 删除用户，同时清理其购物车与云服务器实例，避免残留脏数据。
// 历史订单与订单明细保留以备查账。
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, s.rebind("DELETE FROM cart_items WHERE user_id = ?"), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.rebind("DELETE FROM instances WHERE user_id = ?"), id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, s.rebind("DELETE FROM users WHERE id = ?"), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	return tx.Commit()
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
