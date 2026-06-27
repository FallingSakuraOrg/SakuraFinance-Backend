package store

import (
	"context"
	"time"
)

// CartItem 购物车项，带冗余的商品展示信息（联表查询填充）。
type CartItem struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	ProductID int64     `json:"productId"`
	Quantity  int       `json:"quantity"`
	CreatedAt time.Time `json:"createdAt"`

	// 以下来自联表 products，便于前端直接展示。
	ProductName string  `json:"productName"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	Status      string  `json:"status"`
}

// ListCart 返回用户购物车，联表带出商品名称、价格、库存、状态。
func (s *Store) ListCart(ctx context.Context, userID int64) ([]*CartItem, error) {
	query := s.rebind(`SELECT c.id, c.user_id, c.product_id, c.quantity, c.created_at,
		p.name, p.price, p.stock, p.status
		FROM cart_items c JOIN products p ON p.id = c.product_id
		WHERE c.user_id = ? ORDER BY c.id DESC`)
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*CartItem
	for rows.Next() {
		it := &CartItem{}
		if err := rows.Scan(
			&it.ID, &it.UserID, &it.ProductID, &it.Quantity, &it.CreatedAt,
			&it.ProductName, &it.Price, &it.Stock, &it.Status,
		); err != nil {
			return nil, err
		}
		list = append(list, it)
	}
	return list, rows.Err()
}

// UpsertCartItem 加入购物车；若已存在同商品则累加数量。
func (s *Store) UpsertCartItem(ctx context.Context, userID, productID int64, qty int) error {
	// 先尝试更新已有项，影响行数为 0 再插入，跨方言通用。
	upd := s.rebind("UPDATE cart_items SET quantity = quantity + ? WHERE user_id = ? AND product_id = ?")
	res, err := s.db.ExecContext(ctx, upd, qty, userID, productID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	ins := s.rebind("INSERT INTO cart_items (user_id, product_id, quantity, created_at) VALUES (?, ?, ?, ?)")
	_, err = s.db.ExecContext(ctx, ins, userID, productID, qty, time.Now())
	return err
}

// UpdateCartQty 设置某购物车项的数量（限本人）。
func (s *Store) UpdateCartQty(ctx context.Context, userID, productID int64, qty int) error {
	query := s.rebind("UPDATE cart_items SET quantity = ? WHERE user_id = ? AND product_id = ?")
	_, err := s.db.ExecContext(ctx, query, qty, userID, productID)
	return err
}

// RemoveCartItem 移除购物车项（限本人）。
func (s *Store) RemoveCartItem(ctx context.Context, userID, productID int64) error {
	query := s.rebind("DELETE FROM cart_items WHERE user_id = ? AND product_id = ?")
	_, err := s.db.ExecContext(ctx, query, userID, productID)
	return err
}
