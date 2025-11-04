package kafka

import (
	"context"
)

// Handler is a Kafka message handler which is invoked when it receives
// a message. It must implement the Handle method with one of the following
// signatures:
//
// For raw messages:
//   Handle(context.Context, Message) error
//   Handle(Message) error
//
// For CloudEvents:
//   Handle(context.Context, event.Event) (*event.Event, error)
//   Handle(event.Event) (*event.Event, error)
//   ... and other CloudEvents signatures
//
// It can optionally implement any of Start, Stop, Ready, and Alive.
type Handler any

// Message represents a Kafka message
type Message struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   map[string]string
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
	// Alive allows the instance to report its liveness status.
	Alive(context.Context) (bool, error)
}

// DefaultHandler is used for simple static function implementations which
// need only define a single exported function named Handle.
type DefaultHandler struct {
	Handler any
}

// Handler interface for raw Kafka messages
type handlerMsg interface {
	Handle(Message) error
}

type handlerCtxMsg interface {
	Handle(context.Context, Message) error
}

func getHandlerType(f any) string {
	// Try to determine if it's a CloudEvents handler or raw message handler
	// by checking the method signature
	switch f.(type) {
	case handlerMsg, handlerCtxMsg:
		return "raw"
	default:
		return "cloudevents"
	}
}
