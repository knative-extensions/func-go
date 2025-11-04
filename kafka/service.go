// Package kafka implements a Functions Kafka middleware for use by
// scaffolding which exposes a function as a Kafka consumer which handles
// Kafka messages.
package kafka

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"
)

const (
	DefaultLogLevel      = LogDebug
	DefaultConsumerGroup = "func-go-consumer"
	InstanceStopTimeout  = 30 * time.Second
)

// Start an instance using a new Service
func Start(f any) error {
	log.Debug().Msg("func runtime creating kafka function instance")
	return New(f).Start(context.Background())
}

// Service exposes a Function Instance as a Kafka consumer.
type Service struct {
	f      any
	stop   chan error
	reader *kafka.Reader
}

// New Service which serves the given instance.
func New(f any) *Service {
	svc := &Service{
		f:    f,
		stop: make(chan error),
	}
	return svc
}

// Start serving
func (s *Service) Start(ctx context.Context) (err error) {
	// Get Kafka configuration from environment
	brokers := getBrokers()
	topics := getTopics()
	groupID := getConsumerGroup()

	log.Debug().
		Strs("brokers", brokers).
		Strs("topics", topics).
		Str("group", groupID).
		Msg("kafka function starting")

	// Create Kafka reader
	s.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		GroupTopics: topics,
		GroupID:     groupID,
		MinBytes:    10e3, // 10KB
		MaxBytes:    10e6, // 10MB
	})

	// Start the function instance
	if err = s.startInstance(ctx); err != nil {
		return
	}

	// Wait for signals
	s.handleSignals()

	// Start consuming messages
	go func() {
		s.stop <- s.consume(ctx)
	}()

	log.Debug().Msg("waiting for stop signals or errors")
	// Wait for either a context cancellation or a signal on the stop channel.
	select {
	case err = <-s.stop:
		if err != nil {
			log.Error().Err(err).Msg("kafka function error")
		}
	case <-ctx.Done():
		log.Debug().Msg("kafka function canceled")
	}
	return s.shutdown(err)
}

// consume reads messages from Kafka and dispatches them to the handler
func (s *Service) consume(ctx context.Context) error {
	handlerType := getHandlerType(s.f)
	log.Debug().Str("type", handlerType).Msg("starting message consumption")

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, err := s.reader.ReadMessage(ctx)
			if err != nil {
				if err == context.Canceled {
					return nil
				}
				log.Error().Err(err).Msg("error reading kafka message")
				continue
			}

			log.Debug().
				Str("topic", msg.Topic).
				Int("partition", msg.Partition).
				Int64("offset", msg.Offset).
				Msg("received kafka message")

			if handlerType == "cloudevents" {
				if err := s.handleCloudEvent(ctx, msg); err != nil {
					log.Error().Err(err).Msg("error handling cloudevent")
				}
			} else {
				if err := s.handleRawMessage(ctx, msg); err != nil {
					log.Error().Err(err).Msg("error handling raw message")
				}
			}
		}
	}
}

// handleCloudEvent processes the Kafka message as a CloudEvent
func (s *Service) handleCloudEvent(ctx context.Context, msg kafka.Message) error {
	// Try to decode the message as a CloudEvent
	ce := event.New()

	// Try to unmarshal from JSON first (structured mode)
	if err := json.Unmarshal(msg.Value, &ce); err == nil {
		// Successfully unmarshalled as structured CloudEvent
		return s.invokeCloudEventHandler(ctx, ce)
	}

	// Try binary mode - check headers for CloudEvent attributes
	if ceType, ok := getHeader(msg.Headers, "ce-type"); ok {
		ce.SetType(ceType)
		if ceSource, ok := getHeader(msg.Headers, "ce-source"); ok {
			ce.SetSource(ceSource)
		}
		if ceID, ok := getHeader(msg.Headers, "ce-id"); ok {
			ce.SetID(ceID)
		}
		if ceSpecVersion, ok := getHeader(msg.Headers, "ce-specversion"); ok {
			ce.SetSpecVersion(ceSpecVersion)
		}
		
		// Set data
		if err := ce.SetData(cloudevents.ApplicationJSON, msg.Value); err != nil {
			return fmt.Errorf("failed to set cloudevent data: %w", err)
		}

		return s.invokeCloudEventHandler(ctx, ce)
	}

	// If not a CloudEvent, create a generic one from the Kafka message
	ce.SetType("kafka.message")
	ce.SetSource(fmt.Sprintf("kafka://%s", msg.Topic))
	ce.SetID(fmt.Sprintf("%s-%d-%d", msg.Topic, msg.Partition, msg.Offset))
	if err := ce.SetData(cloudevents.ApplicationJSON, msg.Value); err != nil {
		return fmt.Errorf("failed to set cloudevent data: %w", err)
	}

	return s.invokeCloudEventHandler(ctx, ce)
}

// invokeCloudEventHandler invokes the CloudEvent handler function
func (s *Service) invokeCloudEventHandler(ctx context.Context, ce event.Event) error {
	if dh, ok := s.f.(DefaultHandler); ok {
		return invokeCloudEventHandlerFn(ctx, dh.Handler, ce)
	}
	return invokeCloudEventHandlerFn(ctx, s.f, ce)
}

// invokeCloudEventHandlerFn invokes the CloudEvent handler based on its signature
func invokeCloudEventHandlerFn(ctx context.Context, h any, ce event.Event) error {
	// Handle different CloudEvent handler signatures
	type handlerCtxEvtEvtErr interface {
		Handle(context.Context, event.Event) (*event.Event, error)
	}
	type handlerEvtEvtErr interface {
		Handle(event.Event) (*event.Event, error)
	}
	type handlerCtxEvtErr interface {
		Handle(context.Context, event.Event) error
	}
	type handlerEvtErr interface {
		Handle(event.Event) error
	}
	type handlerCtxEvt interface {
		Handle(context.Context, event.Event)
	}
	type handlerEvt interface {
		Handle(event.Event)
	}
	type handlerCtxErr interface {
		Handle(context.Context) error
	}
	type handlerCtx interface {
		Handle(context.Context)
	}
	type handlerErr interface {
		Handle() error
	}
	type handler interface {
		Handle()
	}

	switch handler := h.(type) {
	case handlerCtxEvtEvtErr:
		_, err := handler.Handle(ctx, ce)
		return err
	case handlerEvtEvtErr:
		_, err := handler.Handle(ce)
		return err
	case handlerCtxEvtErr:
		return handler.Handle(ctx, ce)
	case handlerEvtErr:
		return handler.Handle(ce)
	case handlerCtxEvt:
		handler.Handle(ctx, ce)
		return nil
	case handlerEvt:
		handler.Handle(ce)
		return nil
	case handlerCtxErr:
		return handler.Handle(ctx)
	case handlerCtx:
		handler.Handle(ctx)
		return nil
	case handlerErr:
		return handler.Handle()
	case handler:
		handler.Handle()
		return nil
	default:
		return fmt.Errorf("unsupported CloudEvent handler signature")
	}
}

// handleRawMessage processes the Kafka message as a raw message
func (s *Service) handleRawMessage(ctx context.Context, msg kafka.Message) error {
	kafkaMsg := Message{
		Topic:     msg.Topic,
		Partition: int32(msg.Partition),
		Offset:    msg.Offset,
		Key:       msg.Key,
		Value:     msg.Value,
		Headers:   make(map[string]string),
	}

	// Convert headers
	for _, h := range msg.Headers {
		kafkaMsg.Headers[h.Key] = string(h.Value)
	}

	// Invoke handler
	if dh, ok := s.f.(DefaultHandler); ok {
		return invokeRawHandler(ctx, dh.Handler, kafkaMsg)
	}
	return invokeRawHandler(ctx, s.f, kafkaMsg)
}

// invokeRawHandler invokes the raw message handler
func invokeRawHandler(ctx context.Context, h any, msg Message) error {
	switch handler := h.(type) {
	case handlerMsg:
		return handler.Handle(msg)
	case handlerCtxMsg:
		return handler.Handle(ctx, msg)
	default:
		return fmt.Errorf("unsupported handler signature")
	}
}

func getHeader(headers []kafka.Header, key string) (string, bool) {
	for _, h := range headers {
		if h.Key == key {
			return string(h.Value), true
		}
	}
	return "", false
}

func getBrokers() []string {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}
	return strings.Split(brokers, ",")
}

func getTopics() []string {
	topics := os.Getenv("KAFKA_TOPICS")
	if topics == "" {
		topics = "func-topic"
	}
	return strings.Split(topics, ",")
}

func getConsumerGroup() string {
	group := os.Getenv("KAFKA_CONSUMER_GROUP")
	if group == "" {
		group = DefaultConsumerGroup
	}
	return group
}

func (s *Service) startInstance(ctx context.Context) error {
	if i, ok := s.f.(Starter); ok {
		cfg, err := newCfg()
		if err != nil {
			return err
		}
		go func() {
			if err := i.Start(ctx, cfg); err != nil {
				s.stop <- err
			}
		}()
	} else {
		log.Debug().Msg("function does not implement Start. Skipping")
	}
	return nil
}

func (s *Service) handleSignals() {
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs)
	go func() {
		for {
			sig := <-sigs
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				log.Debug().Any("signal", sig).Msg("signal received")
				s.stop <- nil
			} else if runtime.GOOS == "linux" && sig == syscall.Signal(0x17) {
				// Ignore SIGURG; signal 23 (0x17)
				// See https://go.googlesource.com/proposal/+/master/design/24543-non-cooperative-preemption.md
			}
		}
	}()
}

// readCfg returns a map representation of ./cfg
// Empty map is returned if ./cfg does not exist.
// Error is returned for invalid entries.
// keys and values are space-trimmed.
// Quotes are removed from values.
func readCfg() (map[string]string, error) {
	cfg := map[string]string{}

	f, err := os.Open("cfg")
	if err != nil {
		log.Debug().Msg("no static config")
		return cfg, nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	i := 0
	for scanner.Scan() {
		i++
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return cfg, fmt.Errorf("config line %v invalid: %v", i, line)
		}
		cfg[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), "\"")
	}
	return cfg, scanner.Err()
}

// newCfg creates a final map of config values built from the static
// values in `cfg` and all environment variables.
func newCfg() (cfg map[string]string, err error) {
	if cfg, err = readCfg(); err != nil {
		return
	}

	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		cfg[pair[0]] = pair[1]
	}
	return
}

// shutdown is invoked when the stop channel receives a message and attempts to
// gracefully cease execution.
func (s *Service) shutdown(sourceErr error) (err error) {
	log.Debug().Msg("kafka function stopping")
	var readerErr, instanceErr error

	// Close the Kafka reader
	if s.reader != nil {
		readerErr = s.reader.Close()
	}

	// Start a graceful shutdown of the Function instance
	if i, ok := s.f.(Stopper); ok {
		ctx, cancel := context.WithTimeout(context.Background(), InstanceStopTimeout)
		defer cancel()
		instanceErr = i.Stop(ctx)
	}

	return collapseErrors("shutdown error", sourceErr, instanceErr, readerErr)
}

// collapseErrors returns the first non-nil error which it is passed,
// printing the rest to log with the given prefix.
func collapseErrors(msg string, ee ...error) (err error) {
	for _, e := range ee {
		if e != nil {
			if err == nil {
				err = e
			} else {
				log.Error().Err(e).Msg(msg)
			}
		}
	}
	return
}
