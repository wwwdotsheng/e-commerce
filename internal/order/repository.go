package order

import (
	"context"
	"e-commerce/internal/config"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
)

var (
	repoErrOrderIdempotencyConflict = errors.New("order idempotency key already exists")
)

var constraintMap = map[string]error{
	model.ConstraintOrderIdempotencyKey: repoErrOrderIdempotencyConflict,
}

type Repository struct {
	*database.BaseRepo
	mqCh  *amqp.Channel
	mqCfg *config.OrderMQConfig
}

func NewRepository(db *gorm.DB, mqCh *amqp.Channel, cfg *config.OrderMQConfig) *Repository {
	return &Repository{
		BaseRepo: database.NewBaseRepo(db),
		mqCh:     mqCh,
		mqCfg:    cfg,
	}
}

func (repo *Repository) SetupMQ(cfg *config.OrderMQConfig) error {
	mqCh := repo.mqCh

	err := mqCh.ExchangeDeclare(cfg.Exchange, "direct", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare exchange %s: %w", cfg.Exchange, err)
	}
	dq, err := mqCh.QueueDeclare(cfg.ConsumerQueue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", cfg.ConsumerQueue, err)
	}
	err = mqCh.QueueBind(dq.Name, cfg.RoutingKey, cfg.Exchange, false, nil)
	if err != nil {
		return fmt.Errorf("failed to bind queue %s routing key %s: %w", cfg.ConsumerQueue, cfg.RoutingKey, err)
	}

	args := amqp.Table{
		"x-dead-letter-exchange":    cfg.Exchange,
		"x-dead-letter-routing-key": cfg.RoutingKey,
		"x-message-ttl":             cfg.TTLMs,
	}

	_, err = mqCh.QueueDeclare(
		cfg.DelayQueue,
		true,
		false,
		false,
		false,
		args,
	)
	if err != nil {
		return fmt.Errorf("failed to declare delay queue %s: %w", cfg.DelayQueue, err)
	}
	return nil
}

func (repo *Repository) CreateOrder(ctx context.Context, order *model.Order) error {
	if err := repo.GetDB(ctx).Create(order).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.SQLState() == pgerrcode.UniqueViolation {
			if businessErr, ok := constraintMap[pgErr.ConstraintName]; ok {
				return businessErr
			}
		}
		return fmt.Errorf("failed to create order %s: %w", order.ID, err)
	}
	return nil
}

func (repo *Repository) PublishTimeoutMessage(ctx context.Context, orderID uuid.UUID) error {
	return repo.mqCh.PublishWithContext(ctx,
		"",
		repo.mqCfg.DelayQueue,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			Body:         []byte(orderID.String()),
		},
	)
}

func (repo *Repository) HandleOrderTimeout(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	var order model.Order
	err := repo.GetDB(ctx).Where("id = ?", orderID).First(&order).Error
	if err != nil {
		return nil, fmt.Errorf("查询订单失败 %s: %w", orderID, err)
	}

	if order.Status != model.OrderStatusProcessing {
		return &order, nil // 已处理过
	}

	result := repo.GetDB(ctx).
		Model(&model.Order{}).
		Where("id = ? AND status = ?", orderID, model.OrderStatusProcessing).
		Update("status", model.OrderStatusTimeout)
	if result.Error != nil {
		return nil, fmt.Errorf("更新订单超时状态失败 %s: %w", orderID, result.Error)
	}

	return &order, nil
}

func (repo *Repository) ListOrdersByUserID(ctx context.Context, userID uuid.UUID, pageNum, pageSize int) ([]*model.Order, int64, error) {
	var orders []*model.Order
	var total int64

	baseQuery := repo.GetDB(ctx).Model(&model.Order{}).Where("user_id = ?", userID)

	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := baseQuery.Session(&gorm.Session{}).
		Offset((pageNum - 1) * pageSize).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&orders).Error

	return orders, total, err
}
