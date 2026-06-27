package store

import (
	"context"
	"time"
)

// Category 商品分类。
type Category struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Sort      int       `json:"sort"`
	CreatedAt time.Time `json:"createdAt"`
}

const categoryColumns = `id, name, sort, created_at`

func scanCategory(row interface{ Scan(...any) error }) (*Category, error) {
	c := &Category{}
	if err := row.Scan(&c.ID, &c.Name, &c.Sort, &c.CreatedAt); err != nil {
		return nil, err
	}
	return c, nil
}

// ListCategories 返回全部分类，按 sort 升序、id 升序。
func (s *Store) ListCategories(ctx context.Context) ([]*Category, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+categoryColumns+" FROM categories ORDER BY sort ASC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// CreateCategory 新建分类。
func (s *Store) CreateCategory(ctx context.Context, c *Category) error {
	c.CreatedAt = time.Now()
	return s.execInsert(ctx, s.db, &c.ID,
		"INSERT INTO categories (name, sort, created_at) VALUES (?, ?, ?)",
		c.Name, c.Sort, c.CreatedAt)
}

// UpdateCategory 更新分类名称与排序。
func (s *Store) UpdateCategory(ctx context.Context, c *Category) error {
	query := s.rebind("UPDATE categories SET name = ?, sort = ? WHERE id = ?")
	_, err := s.db.ExecContext(ctx, query, c.Name, c.Sort, c.ID)
	return err
}

// DeleteCategory 删除分类。
func (s *Store) DeleteCategory(ctx context.Context, id int64) error {
	query := s.rebind("DELETE FROM categories WHERE id = ?")
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}
