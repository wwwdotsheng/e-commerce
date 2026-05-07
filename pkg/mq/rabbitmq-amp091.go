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
) (*amqp.Channel, func()) {
	conn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%d", config.User, config.Password, config.Host, config.Port))
	if err != nil {
		logger.Error("无法连接 RabbitMQ:", zap.Error(err))
		panic("连接rabbit-mq失败")
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		logger.Error("无法开启 Channel", zap.Error(err))
		panic("开启Channel失败")
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

	return ch, cleanup
}
