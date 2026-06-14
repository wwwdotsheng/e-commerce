package order

import (
	"context"
	"e-commerce/pkg/clog"
	"fmt"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const maxRetryHeader = "x-retry-count"
const maxRetries = 3

type MqHandler struct {
	svc *Service
}

func NewMqHandler(svc *Service) *MqHandler {
	return &MqHandler{
		svc: svc,
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

	logger.Info("Received a message: " + orderID.String())
	err = h.svc.HandleOrderTimeout(ctx, orderID)

	if err != nil {
		retryCount := getRetryCount(d.Headers)
		if retryCount >= maxRetries {
			logger.Error("订单超时处理达到最大重试次数，丢弃消息",
				zap.String("order_id", orderID.String()),
				zap.Int("retry_count", retryCount),
				zap.Error(err),
			)
			_ = d.Reject(false) // 不重新入队
			return
		}
		logger.Warn("订单超时处理失败，将重新入队",
			zap.String("order_id", orderID.String()),
			zap.Int("retry_count", retryCount),
			zap.Error(err),
		)
		// 更新重试计数并重新入队
		if d.Headers == nil {
			d.Headers = amqp.Table{}
		}
		d.Headers[maxRetryHeader] = retryCount + 1
		_ = d.Nack(false, true)
		return
	}

	_ = d.Ack(false)
}

func getRetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	v, ok := headers[maxRetryHeader]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}
