package tests

import (
	"context"
	"strings"

	"e-commerce/internal/order"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	amqp "github.com/rabbitmq/amqp091-go"
)

var _ = Describe("Order MQ publishing", func() {
	It("发布到不存在的延迟队列时返回错误", func() {
		conn, err := amqp.Dial(testRabbitMQURL)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = conn.Close() }()
		ch, err := conn.Channel()
		Expect(err).NotTo(HaveOccurred())

		cfg := testOrderMQConfig
		cfg.DelayQueue = "e2e_missing_delay_" + uuid.NewString()
		repo := order.NewRepository(testDB, ch, &cfg)

		err = repo.PublishTimeoutMessage(context.Background(), uuid.New())
		Expect(err).To(HaveOccurred())
		Expect(strings.Contains(err.Error(), "unroutable")).To(BeTrue(), "unexpected error: %v", err)
	})
})
