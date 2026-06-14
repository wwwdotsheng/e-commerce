package mq

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

type Config struct {
	User     string
	Password string
	Host     string
	Port     int
}

func InitMq(
	ctx context.Context,
	logger Logger,
	config Config,
) (*amqp.Channel, func(), error) {
	conn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%d", config.User, config.Password, config.Host, config.Port))
	if err != nil {
		return nil, nil, fmt.Errorf("连接 RabbitMQ 失败: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("开启 Channel 失败: %w", err)
	}

	logger.Info("连接rabbit-mq成功")

	cleanup := func() {
		if !ch.IsClosed() {
			_ = ch.Close()
		}
		if !conn.IsClosed() {
			_ = conn.Close()
		}
	}

	return ch, cleanup, nil
}
