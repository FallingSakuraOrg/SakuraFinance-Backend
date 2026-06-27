package store

import (
	"context"
	"time"
)

// Product 商品（云服务器规格 + 价格）。
type Product struct {
	ID          int64     `json:"id"`
	CategoryID  int64     `json:"categoryId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CPU         int       `json:"cpu"`
	RAM         int       `json:"ram"`
	Disk        int       `json:"disk"`
	Bandwidth   int       `json:"bandwidth"`
	Price       float64   `json:"price"`
	Stock       int       `json:"stock"`
	Status      string    `json:"status"` // on / off
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const productColumns = `id, category_id, name, description, cpu, ram, disk, bandwidth, price, stock, status, created_at, updated_at`

func scanProduct(row interface{ Scan(...any) error }) (*Product, error) {
	p := &Product{}
	err := row.Scan(
		&p.ID, &p.CategoryID, &p.Name, &p.Description, &p.CPU, &p.RAM,
		&p.Disk, &p.Bandwidth, &p.Price, &p.Stock, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ListProducts 查询商品。categoryID>0 时按分类过滤；onlyOn 为真时仅返回上架商品。
func (s *Store) ListProducts(ctx context.Context, categoryID int64, onlyOn bool) ([]*Product, error) {
	query := "SELECT " + productColumns + " FROM products"
	var conds []string
	var args []any
	if categoryID > 0 {
		conds = append(conds, "category_id = ?")
		args = append(args, categoryID)
	}
	if onlyOn {
		conds = append(conds, "status = ?")
		args = append(args, "on")
	}
	for i, c := range conds {
		if i == 0 {
			query += " WHERE " + c
		} else {
			query += " AND " + c
		}
	}
	query += " ORDER BY id DESC"

	rows, err := s.db.QueryContext(ctx, s.rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// GetProduct 按 id 查询商品，未找到返回 ErrNotFound。
func (s *Store) GetProduct(ctx context.Context, id int64) (*Product, error) {
	query := s.rebind("SELECT " + productColumns + " FROM products WHERE id = ?")
	p, err := scanProduct(s.db.QueryRowContext(ctx, query, id))
	return normalizeNotFound(p, err)
}

// CreateProduct 新建商品。
func (s *Store) CreateProduct(ctx context.Context, p *Product) error {
	now := time.Now()
	p.CreatedAt, p.UpdatedAt = now, now
	if p.Status == "" {
		p.Status = "on"
	}
	return s.execInsert(ctx, s.db, &p.ID,
		`INSERT INTO products
			(category_id, name, description, cpu, ram, disk, bandwidth, price, stock, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.CategoryID, p.Name, p.Description, p.CPU, p.RAM, p.Disk, p.Bandwidth,
		p.Price, p.Stock, p.Status, p.CreatedAt, p.UpdatedAt)
}

// UpdateProduct 更新商品。
func (s *Store) UpdateProduct(ctx context.Context, p *Product) error {
	p.UpdatedAt = time.Now()
	query := s.rebind(`UPDATE products SET
		category_id = ?, name = ?, description = ?, cpu = ?, ram = ?, disk = ?,
		bandwidth = ?, price = ?, stock = ?, status = ?, updated_at = ?
		WHERE id = ?`)
	_, err := s.db.ExecContext(ctx, query,
		p.CategoryID, p.Name, p.Description, p.CPU, p.RAM, p.Disk, p.Bandwidth,
		p.Price, p.Stock, p.Status, p.UpdatedAt, p.ID)
	return err
}

// DeleteProduct 删除商品。
func (s *Store) DeleteProduct(ctx context.Context, id int64) error {
	query := s.rebind("DELETE FROM products WHERE id = ?")
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}
