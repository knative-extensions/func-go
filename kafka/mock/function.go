package mock

import (
	"context"

	"github.com/cloudevents/sdk-go/v2/event"
)

// Message represents a Kafka message (to avoid import cycle)
type Message struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   map[string]string
}

// Function is a mock Kafka function for testing
type Function struct {
	OnStart        func(context.Context, map[string]string) error
	OnStop         func(context.Context) error
	OnHandleMsg    func(context.Context, Message) error
	OnHandleEvent  func(context.Context, event.Event) (*event.Event, error)
	HandlerType    string // "raw" or "cloudevents"
}

func (f *Function) Start(ctx context.Context, cfg map[string]string) error {
	if f.OnStart != nil {
		return f.OnStart(ctx, cfg)
	}
	return nil
}

func (f *Function) Stop(ctx context.Context) error {
	if f.OnStop != nil {
		return f.OnStop(ctx)
	}
	return nil
}

// Handle for raw messages
func (f *Function) Handle(ctx context.Context, msg Message) error {
	if f.OnHandleMsg != nil {
		return f.OnHandleMsg(ctx, msg)
	}
	return nil
}

// HandleCloudEvent for CloudEvents
func (f *Function) HandleCloudEvent(ctx context.Context, e event.Event) (*event.Event, error) {
	if f.OnHandleEvent != nil {
		return f.OnHandleEvent(ctx, e)
	}
	return &e, nil
}
