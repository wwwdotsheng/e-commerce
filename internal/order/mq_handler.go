package order

import (
	"context"
	"e-commerce/pkg/clog"
	"fmt"
	"log"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

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
		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-msgs:
				if !ok {
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
		log.Printf("failed to parse the order id %s", orderID.String())
		d.Reject(false)
		return
	}

	logger.Info("Received a message: " + orderID.String())
	err = h.svc.HandleOrderTimeout(ctx, orderID)

	if err != nil {
		fmt.Errorf("order %s failed to process the message: %w", orderID, err)
		d.Nack(false, true)
		return
	}

}
