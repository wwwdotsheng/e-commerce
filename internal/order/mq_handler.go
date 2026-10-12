package order

import (
	"context"
	"e-commerce/pkg/clog"
	"fmt"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const maxRetries = 3

type timeoutProcessor interface {
	HandleOrderTimeout(context.Context, uuid.UUID) error
}
type MqHandler struct {
	svc        timeoutProcessor
	retryDelay time.Duration
}

func NewMqHandler(svc timeoutProcessor) *MqHandler {
	return &MqHandler{
		svc:        svc,
		retryDelay: 100 * time.Millisecond,
	}
}

func (h *MqHandler) ListenTimeout(ctx context.Context, ch *amqp.Channel, queueName string) error {
	msgs, err := ch.Consume(
		queueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register a consumer: %w", err)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				clog.L(ctx).Error("订单超时消费者 panic", zap.Any("recover", r))
			}
		}()
		for {
			select {
			case <-ctx.Done():
				clog.L(ctx).Info("订单超时消费者退出")
				return
			case d, ok := <-msgs:
				if !ok {
					clog.L(ctx).Info("订单超时消息通道已关闭")
					return
				}
				h.handleSingleMessage(ctx, d)
			}
		}
	}()

	return nil
}

func (h *MqHandler) handleSingleMessage(ctx context.Context, d amqp.Delivery) {
	logger := clog.L(ctx)
	orderID, err := uuid.Parse(string(d.Body))
	if err != nil {
		logger.Error("无法从消息中解析订单ID",
			zap.String("body", string(d.Body)),
			zap.String("message_id", d.MessageId),
		)
		// 格式错误的消息无法重试，直接丢弃（或可转发到死信队列）
		_ = d.Reject(false)
		return
	}

	// Bound retries within one delivery rather than mutating local headers:
	// basic.nack does not publish modified headers back to RabbitMQ.
	// A process restart can redeliver it; business handling must stay idempotent.
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			_ = d.Nack(false, true)
			return
		}
		err = h.svc.HandleOrderTimeout(ctx, orderID)
		if err == nil {
			if ackErr := d.Ack(false); ackErr != nil {
				logger.Error("ack timeout delivery failed", zap.Error(ackErr))
			}
			return
		}
		if ctx.Err() != nil {
			_ = d.Nack(false, true)
			return
		}
		if attempt == maxRetries {
			logger.Error("timeout delivery exhausted retries", zap.String("order_id", orderID.String()), zap.Int("retries", maxRetries), zap.Error(err))
			_ = d.Reject(false)
			return
		}
		logger.Warn("retry timeout delivery", zap.String("order_id", orderID.String()), zap.Int("retry", attempt+1), zap.Error(err))
		timer := time.NewTimer(h.retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = d.Nack(false, true)
			return
		case <-timer.C:
		}
	}
}
