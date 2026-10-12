package order

import (
	"context"
	"errors"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"testing"
	"time"
)

type stubTimeout struct{ calls, failures int }

func (s *stubTimeout) HandleOrderTimeout(context.Context, uuid.UUID) error {
	s.calls++
	if s.calls <= s.failures {
		return errors.New("injected timeout failure")
	}
	return nil
}

type acknowledgements struct {
	acks, nacks, rejects int
	requeue              bool
}

func (a *acknowledgements) Ack(uint64, bool) error { a.acks++; return nil }
func (a *acknowledgements) Nack(_ uint64, _ bool, requeue bool) error {
	a.nacks++
	a.requeue = requeue
	return nil
}
func (a *acknowledgements) Reject(_ uint64, requeue bool) error {
	a.rejects++
	a.requeue = requeue
	return nil
}
func TestTimeoutDelivery(t *testing.T) {
	for _, tc := range []struct {
		name                                       string
		failures, wantCalls, wantAcks, wantRejects int
	}{
		{"success", 0, 1, 1, 0},
		{"transient failure retries before acknowledgement", 2, 3, 1, 0},
		{"poison delivery is rejected after three retries", 100, 4, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubTimeout{failures: tc.failures}
			ack := &acknowledgements{}
			h := NewMqHandler(svc)
			h.retryDelay = time.Millisecond
			h.handleSingleMessage(context.Background(), amqp.Delivery{Body: []byte(uuid.NewString()), Acknowledger: ack, DeliveryTag: 1})
			if svc.calls != tc.wantCalls || ack.acks != tc.wantAcks || ack.rejects != tc.wantRejects || ack.nacks != 0 || ack.requeue {
				t.Fatalf("calls=%d ack=%d reject=%d nack=%d requeue=%v", svc.calls, ack.acks, ack.rejects, ack.nacks, ack.requeue)
			}
		})
	}
}
func TestMalformedTimeoutDelivery(t *testing.T) {
	svc := &stubTimeout{}
	ack := &acknowledgements{}
	NewMqHandler(svc).handleSingleMessage(context.Background(), amqp.Delivery{Body: []byte("invalid"), Acknowledger: ack, DeliveryTag: 1})
	if svc.calls != 0 || ack.rejects != 1 || ack.requeue {
		t.Fatal("malformed delivery must be rejected without invoking business logic")
	}
}
func TestCanceledTimeoutDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := &stubTimeout{}
	ack := &acknowledgements{}
	NewMqHandler(svc).handleSingleMessage(ctx, amqp.Delivery{Body: []byte(uuid.NewString()), Acknowledger: ack, DeliveryTag: 1})
	if svc.calls != 0 || ack.nacks != 1 || !ack.requeue || ack.acks != 0 {
		t.Fatal("shutdown must requeue an unprocessed delivery")
	}
}
