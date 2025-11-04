# Kafka Middleware

This package implements a Kafka middleware for Knative Go functions, enabling functions to consume messages from Kafka topics.

## Features

- **CloudEvents Support**: Process Kafka messages as CloudEvents (similar to the `cloudevents` package)
- **Raw Message Support**: Process raw Kafka messages directly (similar to the `http` package)
- **Lifecycle Hooks**: Support for Start, Stop, Ready, and Alive hooks
- **Configurable**: Environment variable-based configuration for brokers, topics, and consumer groups

## Usage

### Environment Variables

Configure the Kafka middleware using the following environment variables:

- `KAFKA_BROKERS`: Comma-separated list of Kafka broker addresses (default: `localhost:9092`)
- `KAFKA_TOPICS`: Comma-separated list of Kafka topics to consume (default: `func-topic`)
- `KAFKA_CONSUMER_GROUP`: Consumer group ID (default: `func-go-consumer`)

### CloudEvents Handler

For processing Kafka messages as CloudEvents:

```go
package main

import (
    "context"
    "fmt"
    "os"
    
    "github.com/cloudevents/sdk-go/v2/event"
    kafka "knative.dev/func-go/kafka"
)

type Function struct{}

func (f *Function) Handle(ctx context.Context, e event.Event) (*event.Event, error) {
    fmt.Printf("Received CloudEvent: Type=%s, Source=%s\n", e.Type(), e.Source())
    return &e, nil
}

func main() {
    if err := kafka.Start(&Function{}); err != nil {
        fmt.Fprintln(os.Stderr, err.Error())
        os.Exit(1)
    }
}
```

Supported CloudEvent handler signatures:
- `Handle()`
- `Handle() error`
- `Handle(context.Context)`
- `Handle(context.Context) error`
- `Handle(event.Event)`
- `Handle(event.Event) error`
- `Handle(context.Context, event.Event)`
- `Handle(context.Context, event.Event) error`
- `Handle(event.Event) *event.Event`
- `Handle(event.Event) (*event.Event, error)`
- `Handle(context.Context, event.Event) *event.Event`
- `Handle(context.Context, event.Event) (*event.Event, error)`

### Raw Message Handler

For processing raw Kafka messages:

```go
package main

import (
    "context"
    "fmt"
    "os"
    
    kafka "knative.dev/func-go/kafka"
)

type Function struct{}

func (f *Function) Handle(ctx context.Context, msg kafka.Message) error {
    fmt.Printf("Received Kafka Message: Topic=%s, Partition=%d, Offset=%d\n",
        msg.Topic, msg.Partition, msg.Offset)
    fmt.Printf("Key=%s, Value=%s\n", string(msg.Key), string(msg.Value))
    return nil
}

func main() {
    if err := kafka.Start(&Function{}); err != nil {
        fmt.Fprintln(os.Stderr, err.Error())
        os.Exit(1)
    }
}
```

Supported raw message handler signatures:
- `Handle(kafka.Message) error`
- `Handle(context.Context, kafka.Message) error`

### Lifecycle Hooks

Optionally implement lifecycle interfaces:

```go
type Function struct{}

// Called when the function starts
func (f *Function) Start(ctx context.Context, cfg map[string]string) error {
    fmt.Println("Function starting...")
    return nil
}

// Called when the function stops
func (f *Function) Stop(ctx context.Context) error {
    fmt.Println("Function stopping...")
    return nil
}

// Readiness check
func (f *Function) Ready(ctx context.Context) (bool, error) {
    return true, nil
}

// Liveness check
func (f *Function) Alive(ctx context.Context) (bool, error) {
    return true, nil
}

func (f *Function) Handle(ctx context.Context, msg kafka.Message) error {
    // Handler implementation
    return nil
}
```

## Message Structure

The `kafka.Message` struct provides access to all Kafka message attributes:

```go
type Message struct {
    Topic     string            // Kafka topic name
    Partition int32             // Partition number
    Offset    int64             // Message offset
    Key       []byte            // Message key
    Value     []byte            // Message value
    Headers   map[string]string // Message headers
}
```

## CloudEvents Integration

The middleware automatically detects CloudEvents in Kafka messages:

1. **Structured Mode**: JSON-encoded CloudEvents in the message value
2. **Binary Mode**: CloudEvent attributes in Kafka headers (prefixed with `ce-`)
3. **Automatic Wrapping**: Non-CloudEvent messages are automatically wrapped as CloudEvents with type `kafka.message`

## Example

See `cmd/fcekafka/main.go` for CloudEvents handler example and `cmd/frawkafka/main.go` for raw message handler example.

## Testing

Run the tests with:

```bash
go test ./kafka/...
```

The package includes comprehensive tests for:
- Lifecycle hooks (Start, Stop)
- Configuration parsing
- Handler type detection
- Message processing
