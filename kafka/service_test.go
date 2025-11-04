package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	"knative.dev/func-go/kafka/mock"
)

// TestStart_Invoked ensures that the Start method of a function is invoked
// if it is implemented by the function instance.
func TestStart_Invoked(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "localhost:9092")
	t.Setenv("KAFKA_TOPICS", "test-topic")
	
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		errCh       = make(chan error)
		timeoutCh   = time.After(500 * time.Millisecond)
		onStart     = func(_ context.Context, _ map[string]string) error {
			startCh <- true
			return nil
		}
	)
	defer cancel()

	f := &mock.Function{OnStart: onStart, HandlerType: "raw"}

	go func() {
		if err := New(f).Start(ctx); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-timeoutCh:
		t.Fatal("function failed to notify of start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		t.Log("start signal received")
	}
	cancel()
}

// TestStart_Static checks that static method Start(f) is a convenience method
// for New(f).Start()
func TestStart_Static(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "localhost:9092")
	t.Setenv("KAFKA_TOPICS", "test-topic")
	
	var (
		startCh   = make(chan any)
		errCh     = make(chan error)
		timeoutCh = time.After(500 * time.Millisecond)
		onStart   = func(_ context.Context, _ map[string]string) error {
			startCh <- true
			return nil
		}
	)

	f := &mock.Function{OnStart: onStart, HandlerType: "raw"}

	go func() {
		if err := Start(f); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-timeoutCh:
		t.Fatal("function failed to notify of start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		t.Log("start signal received")
	}
}

// TestStart_CfgEnvs ensures that the function's Start method receives a map
// containing all available environment variables as a parameter.
func TestStart_CfgEnvs(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "localhost:9092")
	t.Setenv("KAFKA_TOPICS", "test-topic")
	t.Setenv("TEST_ENV", "example_value")
	
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		errCh       = make(chan error)
		timeoutCh   = time.After(500 * time.Millisecond)
		onStart     = func(_ context.Context, cfg map[string]string) error {
			v := cfg["TEST_ENV"]
			if v != "example_value" {
				t.Fatalf("did not receive TEST_ENV. got %v", cfg["TEST_ENV"])
			}
			startCh <- true
			return nil
		}
	)
	defer cancel()

	f := &mock.Function{OnStart: onStart, HandlerType: "raw"}

	go func() {
		if err := New(f).Start(ctx); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-timeoutCh:
		t.Fatal("function failed to notify of start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		t.Log("start signal received with correct config")
	}
	cancel()
}

// TestGetBrokers tests the broker configuration parsing
func TestGetBrokers(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     []string
	}{
		{
			name:     "single broker",
			envValue: "broker1:9092",
			want:     []string{"broker1:9092"},
		},
		{
			name:     "multiple brokers",
			envValue: "broker1:9092,broker2:9092,broker3:9092",
			want:     []string{"broker1:9092", "broker2:9092", "broker3:9092"},
		},
		{
			name:     "default when empty",
			envValue: "",
			want:     []string{"localhost:9092"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("KAFKA_BROKERS", tt.envValue)
			}
			got := getBrokers()
			if len(got) != len(tt.want) {
				t.Errorf("getBrokers() length = %v, want %v", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getBrokers()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestGetTopics tests the topic configuration parsing
func TestGetTopics(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     []string
	}{
		{
			name:     "single topic",
			envValue: "topic1",
			want:     []string{"topic1"},
		},
		{
			name:     "multiple topics",
			envValue: "topic1,topic2,topic3",
			want:     []string{"topic1", "topic2", "topic3"},
		},
		{
			name:     "default when empty",
			envValue: "",
			want:     []string{"func-topic"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("KAFKA_TOPICS", tt.envValue)
			}
			got := getTopics()
			if len(got) != len(tt.want) {
				t.Errorf("getTopics() length = %v, want %v", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getTopics()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestGetConsumerGroup tests the consumer group configuration
func TestGetConsumerGroup(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "custom group",
			envValue: "my-consumer-group",
			want:     "my-consumer-group",
		},
		{
			name:     "default when empty",
			envValue: "",
			want:     DefaultConsumerGroup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("KAFKA_CONSUMER_GROUP", tt.envValue)
			}
			got := getConsumerGroup()
			if got != tt.want {
				t.Errorf("getConsumerGroup() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestHandlerType tests the handler type detection
func TestHandlerType(t *testing.T) {
	// Test CloudEvents handler
	ceFunc := &CloudEventTestFunction{}
	handlerType := getHandlerType(ceFunc)
	if handlerType != "cloudevents" {
		t.Errorf("expected 'cloudevents' handler type for CE function, got %v", handlerType)
	}
	
	// Test raw message handler
	rawFunc := &RawMessageTestFunction{}
	handlerType = getHandlerType(rawFunc)
	if handlerType != "raw" {
		t.Errorf("expected 'raw' handler type for raw function, got %v", handlerType)
	}
}

// CloudEventTestFunction is a test function that handles CloudEvents
type CloudEventTestFunction struct{}

func (f *CloudEventTestFunction) Handle(ctx context.Context, e event.Event) (*event.Event, error) {
	return &e, nil
}

// RawMessageTestFunction is a test function that handles raw messages
type RawMessageTestFunction struct{}

func (f *RawMessageTestFunction) Handle(ctx context.Context, msg Message) error {
	return nil
}
