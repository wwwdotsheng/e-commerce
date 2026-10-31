package order

import (
	"context"
	"e-commerce/internal/config"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// FindByIdempotencyKey must also validate ownership and payload at the service layer.
func (repo *Repository) FindByIdempotencyKey(ctx context.Context, key string) (*model.Order, error) {
	var o model.Order
	err := repo.GetDB(ctx).Where("idempotency_key = ?", key).First(&o).Error
	return &o, err
}

// HandleOrderTimeout runs inside the service transaction. A nil order means
// another transition already handled it; callers must not release resources.
func (repo *Repository) HandleOrderTimeout(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	var o model.Order
	if err := repo.GetDB(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", orderID).First(&o).Error; err != nil {
		return nil, fmt.Errorf("query timeout order %s: %w", orderID, err)
	}
	if o.Status != model.OrderStatusProcessing {
		return nil, nil
	}
	closedAt := time.Now()
	closeReason := model.OrderCloseReasonPaymentTimeout
	result := repo.GetDB(ctx).Model(&model.Order{}).
		Where("id = ? AND status = ?", orderID, model.OrderStatusProcessing).
		Updates(map[string]interface{}{"status": model.OrderStatusTimeout, "closed_at": closedAt, "close_reason": closeReason})
	if result.Error != nil {
		return nil, fmt.Errorf("close timeout order %s: %w", orderID, result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, nil
	}
	o.Status = model.OrderStatusTimeout
	o.ClosedAt = &closedAt
	o.CloseReason = &closeReason
	return &o, nil
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
