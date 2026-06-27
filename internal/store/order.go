package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrInsufficientBalance 余额不足，无法完成余额支付。
var ErrInsufficientBalance = errors.New("余额不足")

// ErrEmptyCart 购物车为空，无法结算。
var ErrEmptyCart = errors.New("购物车为空")

// Order 订单。
type Order struct {
	ID          int64        `json:"id"`
	OrderNo     string       `json:"orderNo"`
	UserID      int64        `json:"userId"`
	TotalAmount float64      `json:"totalAmount"`
	Status      string       `json:"status"` // pending / paid / cancelled
	PayMethod   string       `json:"payMethod"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	Items       []*OrderItem `json:"items,omitempty"`
}

// OrderItem 订单明细，下单时快照商品名与价格。
type OrderItem struct {
	ID          int64   `json:"id"`
	OrderID     int64   `json:"orderId"`
	ProductID   int64   `json:"productId"`
	ProductName string  `json:"productName"`
	Price       float64 `json:"price"`
	Quantity    int     `json:"quantity"`
}

// instanceTTL 云服务器实例默认有效期（30 天）。
const instanceTTL = 30 * 24 * time.Hour

// Checkout 结算用户购物车，事务内完成：
// 建订单与明细 → 扣库存 → 生成云服务器实例 → 清空购物车。
// payMethod=="balance" 时额外校验并扣减余额；网关支付（本阶段模拟成功）不动余额。
// 返回创建好的订单。
func (s *Store) Checkout(ctx context.Context, userID int64, orderNo, payMethod string) (*Order, error) {
	useBalance := payMethod == "balance"

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() // 提交后再次 Rollback 是空操作，安全。

	// 1. 读取购物车（联表带商品价格/库存/状态）。
	cartQuery := s.rebind(`SELECT c.product_id, c.quantity, p.name, p.price, p.stock, p.status,
		p.cpu, p.ram, p.disk, p.bandwidth
		FROM cart_items c JOIN products p ON p.id = c.product_id
		WHERE c.user_id = ?`)
	rows, err := tx.QueryContext(ctx, cartQuery, userID)
	if err != nil {
		return nil, err
	}

	type line struct {
		productID                       int64
		qty                             int
		name                            string
		price                           float64
		stock                           int
		status                          string
		cpu, ram, disk, bandwidth       int
	}
	var lines []line
	var total float64
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.productID, &l.qty, &l.name, &l.price, &l.stock, &l.status,
			&l.cpu, &l.ram, &l.disk, &l.bandwidth); err != nil {
			rows.Close()
			return nil, err
		}
		if l.status != "on" {
			rows.Close()
			return nil, fmt.Errorf("商品「%s」已下架", l.name)
		}
		if l.stock < l.qty {
			rows.Close()
			return nil, fmt.Errorf("商品「%s」库存不足", l.name)
		}
		total += l.price * float64(l.qty)
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(lines) == 0 {
		return nil, ErrEmptyCart
	}

	// 2. 余额支付时校验余额（网关支付本阶段模拟成功，不校验）。
	if useBalance {
		var balance float64
		balQuery := s.rebind("SELECT balance FROM users WHERE id = ?")
		if err := tx.QueryRowContext(ctx, balQuery, userID).Scan(&balance); err != nil {
			return nil, err
		}
		if balance < total {
			return nil, ErrInsufficientBalance
		}
	}

	now := time.Now()
	order := &Order{
		OrderNo:     orderNo,
		UserID:      userID,
		TotalAmount: total,
		Status:      "paid",
		PayMethod:   payMethod,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 3. 建订单。
	if err := s.execInsert(ctx, tx, &order.ID,
		`INSERT INTO orders (order_no, user_id, total_amount, status, pay_method, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
		order.OrderNo, order.UserID, order.TotalAmount, order.Status, order.PayMethod, now, now); err != nil {
		return nil, err
	}

	expiresAt := now.Add(instanceTTL)
	for _, l := range lines {
		// 3a. 订单明细。
		var itemID int64
		if err := s.execInsert(ctx, tx, &itemID,
			`INSERT INTO order_items (order_id, product_id, product_name, price, quantity)
				VALUES (?, ?, ?, ?, ?)`,
			order.ID, l.productID, l.name, l.price, l.qty); err != nil {
			return nil, err
		}
		order.Items = append(order.Items, &OrderItem{
			ID: itemID, OrderID: order.ID, ProductID: l.productID,
			ProductName: l.name, Price: l.price, Quantity: l.qty,
		})

		// 3b. 扣库存。
		if _, err := tx.ExecContext(ctx,
			s.rebind("UPDATE products SET stock = stock - ? WHERE id = ?"),
			l.qty, l.productID); err != nil {
			return nil, err
		}

		// 3c. 每件生成一台云服务器实例。
		for i := 0; i < l.qty; i++ {
			var instID int64
			if err := s.execInsert(ctx, tx, &instID,
				`INSERT INTO instances
					(user_id, order_id, product_id, name, cpu, ram, disk, bandwidth, status, expires_at, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				userID, order.ID, l.productID, l.name, l.cpu, l.ram, l.disk, l.bandwidth,
				"running", expiresAt, now); err != nil {
				return nil, err
			}
		}
	}

	// 4. 余额支付时扣减余额；网关支付（模拟成功）不动余额。
	// TODO: 接入真实支付网关后，网关支付应在收到回调确认后再标记订单 paid。
	if useBalance {
		if _, err := tx.ExecContext(ctx,
			s.rebind("UPDATE users SET balance = balance - ?, updated_at = ? WHERE id = ?"),
			total, now, userID); err != nil {
			return nil, err
		}
	}

	// 5. 清空购物车。
	if _, err := tx.ExecContext(ctx,
		s.rebind("DELETE FROM cart_items WHERE user_id = ?"), userID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return order, nil
}

// ListOrders 返回用户订单（含明细），按时间倒序。
func (s *Store) ListOrders(ctx context.Context, userID int64) ([]*Order, error) {
	query := s.rebind(`SELECT id, order_no, user_id, total_amount, status, pay_method, created_at, updated_at
		FROM orders WHERE user_id = ? ORDER BY id DESC`)
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Order
	byID := map[int64]*Order{}
	for rows.Next() {
		o := &Order{}
		if err := rows.Scan(&o.ID, &o.OrderNo, &o.UserID, &o.TotalAmount,
			&o.Status, &o.PayMethod, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, o)
		byID[o.ID] = o
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return list, nil
	}

	// 一次性取出全部订单明细再归并，避免 N+1 查询。
	itemRows, err := s.db.QueryContext(ctx, s.rebind(
		"SELECT id, order_id, product_id, product_name, price, quantity FROM order_items WHERE order_id IN ("+
			placeholders(len(list))+")"), orderIDs(list)...)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()
	for itemRows.Next() {
		it := &OrderItem{}
		if err := itemRows.Scan(&it.ID, &it.OrderID, &it.ProductID,
			&it.ProductName, &it.Price, &it.Quantity); err != nil {
			return nil, err
		}
		if o := byID[it.OrderID]; o != nil {
			o.Items = append(o.Items, it)
		}
	}
	return list, itemRows.Err()
}

// orderIDs 抽取订单 id 列表，用于 IN 查询。
func orderIDs(list []*Order) []any {
	ids := make([]any, len(list))
	for i, o := range list {
		ids[i] = o.ID
	}
	return ids
}
