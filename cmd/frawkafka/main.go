package main

import (
	"context"
	"fmt"
	"os"

	kafka "knative.dev/func-go/kafka"
)

// Main illustrates how scaffolding works to wrap a user's function.
func main() {
	// Instanced example (in scaffolding, 'New()' will be in module 'f')
	if err := kafka.Start(New()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	// Static example (in scaffolding 'Handle' will be in module f
	// if err := kafka.Start(kafka.DefaultHandler{Handle}); err != nil {
	// 	fmt.Fprintln(os.Stderr, err.Error())
	// 	os.Exit(1)
	// }
}

// Example Static Kafka Handler implementation.
func Handle(ctx context.Context, msg kafka.Message) error {
	fmt.Println("Static Kafka handler invoked")
	fmt.Printf("Topic: %s, Value: %s\n", msg.Topic, string(msg.Value))
	return nil
}

// MyFunction is an example instanced Kafka function implementation.
type MyFunction struct{}

func New() *MyFunction {
	return &MyFunction{}
}

func (f *MyFunction) Handle(ctx context.Context, msg kafka.Message) error {
	fmt.Println("Instanced Kafka handler invoked")
	fmt.Printf("Topic: %s, Partition: %d, Offset: %d\n", msg.Topic, msg.Partition, msg.Offset)
	fmt.Printf("Key: %s, Value: %s\n", string(msg.Key), string(msg.Value))
	return nil
}
