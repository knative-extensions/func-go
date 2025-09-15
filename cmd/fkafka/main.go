package main

import (
	"context"
	"fmt"
	"os"

	"github.com/IBM/sarama"

	kafka "knative.dev/func-go/kafka"
)

// Main illustrates how scaffolding works to wrap a user's function.
func main() {
	// Instanced Example
	// (in scaffolding 'New()' will be in module 'f')
	if err := kafka.Start(New()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	// Static Example
	// (in scaffolding 'Handle' will be in the module 'f')
	// if err := kafka.Start(kafka.DefaultHandler{Handle}); err != nil {
	//   fmt.Fprintln(os.Stderr, err.Error())
	//   os.Exit(1)
	// }
}

// Handle is an example static function implementation.
func Handle(ctx context.Context, msg *sarama.ConsumerMessage) error {
	fmt.Printf("Static Kafka Handler invoked - Topic: %s, Partition: %d, Offset: %d\n", 
		msg.Topic, msg.Partition, msg.Offset)
	fmt.Printf("Message: %s\n", string(msg.Value))
	return nil
}

// MyFunction is an example instanced Kafka function implementation.
type MyFunction struct{}

func New() *MyFunction {
	return &MyFunction{}
}

func (f *MyFunction) Handle(ctx context.Context, msg *sarama.ConsumerMessage) error {
	fmt.Printf("Instanced Kafka handler invoked - Topic: %s, Partition: %d, Offset: %d\n", 
		msg.Topic, msg.Partition, msg.Offset)
	fmt.Printf("Key: %s\n", string(msg.Key))
	fmt.Printf("Value: %s\n", string(msg.Value))
	
	// Process headers if present
	if len(msg.Headers) > 0 {
		fmt.Println("Headers:")
		for _, header := range msg.Headers {
			fmt.Printf("  %s: %s\n", string(header.Key), string(header.Value))
		}
	}
	
	// Example: Log timestamp
	fmt.Printf("Timestamp: %s\n", msg.Timestamp)
	
	return nil
}

// Optional lifecycle methods - MyFunction can implement these interfaces:

// Start is called when the function starts
func (f *MyFunction) Start(ctx context.Context, cfg map[string]string) error {
	fmt.Println("Kafka function starting...")
	// Initialize any resources here
	return nil
}

// Stop is called during graceful shutdown
func (f *MyFunction) Stop(ctx context.Context) error {
	fmt.Println("Kafka function stopping...")
	// Cleanup resources here
	return nil
}

// Ready reports if the function is ready to process messages
func (f *MyFunction) Ready(ctx context.Context) (bool, error) {
	// Add any readiness checks here
	return true, nil
}

// Alive reports if the function is alive
func (f *MyFunction) Alive(ctx context.Context) (bool, error) {
	// Add any liveness checks here
	return true, nil
}