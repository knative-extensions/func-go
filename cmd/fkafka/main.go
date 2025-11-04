package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudevents/sdk-go/v2/event"
	kafka "knative.dev/func-go/kafka"
)

// Main illustrates how scaffolding works to wrap a user's function.
func main() {
	// Example 1: Instanced CloudEvents Handler
	// (in scaffolding 'New()' will be in module 'f')
	if err := kafka.Start(NewCloudEventsFunction()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	// Example 2: Instanced Raw Message Handler
	// Uncomment to use raw message handler instead:
	// if err := kafka.Start(NewRawMessageFunction()); err != nil {
	// 	fmt.Fprintln(os.Stderr, err.Error())
	// 	os.Exit(1)
	// }

	// Example 3: Static CloudEvents Handler
	// (in scaffolding 'HandleCloudEvent' will be in the module 'f')
	// if err := kafka.Start(kafka.DefaultHandler{HandleCloudEvent}); err != nil {
	// 	fmt.Fprintln(os.Stderr, err.Error())
	// 	os.Exit(1)
	// }

	// Example 4: Static Raw Message Handler
	// (in scaffolding 'HandleRawMessage' will be in module 'f')
	// if err := kafka.Start(kafka.DefaultHandler{HandleRawMessage}); err != nil {
	// 	fmt.Fprintln(os.Stderr, err.Error())
	// 	os.Exit(1)
	// }
}

// HandleCloudEvent is an example static CloudEvent function implementation.
func HandleCloudEvent(ctx context.Context, e event.Event) (*event.Event, error) {
	fmt.Println("Static CloudEvent Handler invoked")
	fmt.Printf("CloudEvent - Type: %s, Source: %s\n", e.Type(), e.Source())
	return &e, nil // echo to caller
}

// HandleRawMessage is an example static raw message function implementation.
func HandleRawMessage(ctx context.Context, msg kafka.Message) error {
	fmt.Println("Static Raw Message Handler invoked")
	fmt.Printf("Kafka Message - Topic: %s, Partition: %d, Offset: %d\n",
		msg.Topic, msg.Partition, msg.Offset)
	fmt.Printf("Key: %s, Value: %s\n", string(msg.Key), string(msg.Value))
	return nil
}

// CloudEventsFunction is an example instanced CloudEvents function implementation.
type CloudEventsFunction struct{}

func NewCloudEventsFunction() *CloudEventsFunction {
	return &CloudEventsFunction{}
}

/*
Supported CloudEvent method signatures:

	Handle()
	Handle() error
	Handle(context.Context)
	Handle(context.Context) error
	Handle(event.Event)
	Handle(event.Event) error
	Handle(context.Context, event.Event)
	Handle(context.Context, event.Event) error
	Handle(event.Event) *event.Event
	Handle(event.Event) (*event.Event, error)
	Handle(context.Context, event.Event) *event.Event
	Handle(context.Context, event.Event) (*event.Event, error)
*/
func (f *CloudEventsFunction) Handle(ctx context.Context, e event.Event) (*event.Event, error) {
	fmt.Println("Instanced CloudEvents handler invoked")
	fmt.Printf("CloudEvent - Type: %s, Source: %s, ID: %s\n", e.Type(), e.Source(), e.ID())
	
	// Print data if available
	if len(e.Data()) > 0 {
		fmt.Printf("Data: %s\n", string(e.Data()))
	}
	
	return &e, nil // echo to caller
}

// RawMessageFunction is an example instanced raw message function implementation.
type RawMessageFunction struct{}

func NewRawMessageFunction() *RawMessageFunction {
	return &RawMessageFunction{}
}

/*
Supported Raw Message method signatures:

	Handle(kafka.Message) error
	Handle(context.Context, kafka.Message) error
*/
func (f *RawMessageFunction) Handle(ctx context.Context, msg kafka.Message) error {
	fmt.Println("Instanced Raw Message handler invoked")
	fmt.Printf("Kafka Message - Topic: %s, Partition: %d, Offset: %d\n",
		msg.Topic, msg.Partition, msg.Offset)
	fmt.Printf("Key: %s, Value: %s\n", string(msg.Key), string(msg.Value))
	
	// Print headers if available
	if len(msg.Headers) > 0 {
		fmt.Println("Headers:")
		for k, v := range msg.Headers {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
	
	return nil
}
