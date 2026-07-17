package main

import (
	"context"
	"fmt"
	"os"

	cloudevents "github.com/cloudevents/sdk-go/v2/event"

	"knative.dev/func-go/kafka"
)

// Main illustrates how scaffolding works to wrap a user's function
// when the event source is Kafka.
//
// The function uses a standard CloudEvents handler signature.
// Kafka messages are automatically wrapped as CloudEvents by the runtime.
//
// Required environment variables:
//
//	KAFKA_BROKERS       — comma-separated broker addresses
//	KAFKA_TOPIC         — topic to consume from
//	KAFKA_CONSUMER_GROUP — consumer group ID
func main() {
	if err := kafka.Start(New()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

// MyFunction is an example instanced CloudEvents function that consumes
// from Kafka.
type MyFunction struct{}

func New() *MyFunction {
	return &MyFunction{}
}

func (f *MyFunction) Handle(ctx context.Context, e cloudevents.Event) error {
	fmt.Printf("Received event: type=%s source=%s id=%s\n",
		e.Type(), e.Source(), e.ID())
	if e.Data() != nil {
		fmt.Printf("Data: %s\n", string(e.Data()))
	}
	return nil
}
