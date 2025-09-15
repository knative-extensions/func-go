package kafka

import (
	"context"

	"github.com/IBM/sarama"
)

// Handler is a Kafka message function Handler, which is invoked when it
// receives a Kafka message. It must implement the Handle method:
//
//	Handle(context.Context, *sarama.ConsumerMessage) error
//
// It can optionally implement any of Start, Stop, Ready, and Alive.
type Handler interface {
	// Handle a Kafka message.
	Handle(context.Context, *sarama.ConsumerMessage) error
}

// Starter is a function which defines a method to be called on function start.
type Starter interface {
	// Start instance event hook.
	Start(context.Context, map[string]string) error
}

// Stopper is function which defines a method to be called on function stop.
type Stopper interface {
	// Stop instance event hook.
	Stop(context.Context) error
}

// ReadinessReporter is a function which defines a method to be used to
// determine readiness.
type ReadinessReporter interface {
	// Ready to be invoked or not.
	Ready(context.Context) (bool, error)
}

// LivenessReporter is a function which defines a method to be used to
// determine liveness.
type LivenessReporter interface {
	// Alive allows the instance to report it's liveness status.
	Alive(context.Context) (bool, error)
}

// DefaultHandler is used for simple static function implementations which
// need only define a single exported function named Handle which must be
// of the signature: func(context.Context, *sarama.ConsumerMessage) error
type DefaultHandler struct {
	Handler HandleFunc
}

func (f DefaultHandler) Handle(ctx context.Context, msg *sarama.ConsumerMessage) error {
	return f.Handler(ctx, msg)
}

type HandleFunc func(context.Context, *sarama.ConsumerMessage) error

// ConsumerGroupHandler implements sarama.ConsumerGroupHandler interface
// to bridge between Sarama's consumer group and our function handler
type ConsumerGroupHandler struct {
	handler Handler
	ready   chan bool
}

func NewConsumerGroupHandler(handler Handler) *ConsumerGroupHandler {
	return &ConsumerGroupHandler{
		handler: handler,
		ready:   make(chan bool),
	}
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (h *ConsumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	close(h.ready)
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (h *ConsumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
func (h *ConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}
			
			if err := h.handler.Handle(session.Context(), message); err != nil {
				// Log error but continue processing
				// In production, you might want to implement retry logic or dead letter queues
				continue
			}
			
			// Mark message as processed
			session.MarkMessage(message, "")
			
		case <-session.Context().Done():
			return nil
		}
	}
}

// Ready returns a channel that's closed when the consumer is ready
func (h *ConsumerGroupHandler) Ready() <-chan bool {
	return h.ready
}