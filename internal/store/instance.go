package store

import (
	"context"
	"time"
)

// Instance 云服务器实例，下单支付成功后生成。
type Instance struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	OrderID   int64     `json:"orderId"`
	ProductID int64     `json:"productId"`
	Name      string    `json:"name"`
	CPU       int       `json:"cpu"`
	RAM       int       `json:"ram"`
	Disk      int       `json:"disk"`
	Bandwidth int       `json:"bandwidth"`
	Status    string    `json:"status"` // running / expired
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// ListInstances 返回用户的云服务器实例，按创建时间倒序。
func (s *Store) ListInstances(ctx context.Context, userID int64) ([]*Instance, error) {
	query := s.rebind(`SELECT id, user_id, order_id, product_id, name, cpu, ram, disk, bandwidth,
		status, expires_at, created_at
		FROM instances WHERE user_id = ? ORDER BY id DESC`)
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Instance
	for rows.Next() {
		it := &Instance{}
		if err := rows.Scan(&it.ID, &it.UserID, &it.OrderID, &it.ProductID, &it.Name,
			&it.CPU, &it.RAM, &it.Disk, &it.Bandwidth, &it.Status, &it.ExpiresAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, it)
	}
	return list, rows.Err()
}
