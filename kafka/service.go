// Package kafka implements a Functions Kafka middleware for use by
// scaffolding which exposes a function as a Kafka consumer.
package kafka

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/rs/zerolog/log"
)

const (
	DefaultLogLevel     = LogDebug
	InstanceStopTimeout = 30 * time.Second
)

// Start an instance using a new Service
func Start(f Handler) error {
	log.Debug().Msg("func runtime creating kafka function instance")
	return New(f).Start(context.Background())
}

// Service exposes a Function Instance as a Kafka consumer.
type Service struct {
	f            Handler
	stop         chan error
	consumer     sarama.ConsumerGroup
	groupHandler *ConsumerGroupHandler
}

// New Service which serves the given instance.
func New(f Handler) *Service {
	svc := &Service{
		f:            f,
		stop:         make(chan error),
		groupHandler: NewConsumerGroupHandler(f),
	}

	// Print some helpful information about which interfaces the function
	// is correctly implementing
	logImplements(f)

	return svc
}

// log which interfaces the function implements.
func logImplements(f any) {
	if _, ok := f.(Starter); ok {
		log.Info().Msg("Function implements Start")
	}
	if _, ok := f.(Stopper); ok {
		log.Info().Msg("Function implements Stop")
	}
	if _, ok := f.(ReadinessReporter); ok {
		log.Info().Msg("Function implements Ready")
	}
	if _, ok := f.(LivenessReporter); ok {
		log.Info().Msg("Function implements Alive")
	}
}

// Start the Kafka consumer
// Will stop when the context is canceled, a runtime error is encountered,
// or an os interrupt or kill signal is received.
func (s *Service) Start(ctx context.Context) (err error) {
	log.Debug().Msg("kafka function starting")

	// Start function instance
	if err = s.startInstance(ctx); err != nil {
		return
	}

	// Start Kafka consumer
	if err = s.startKafkaConsumer(ctx); err != nil {
		return
	}

	// Handle signals
	s.handleSignals()

	log.Debug().Msg("waiting for stop signals or errors")
	// Wait for either a context cancellation or a signal on the stop channel.
	select {
	case err = <-s.stop:
		if err != nil {
			log.Error().Err(err).Msg("function error")
		}
	case <-ctx.Done():
		log.Debug().Msg("function canceled")
	}
	return s.shutdown(err)
}

func (s *Service) startKafkaConsumer(ctx context.Context) error {
	cfg, err := newCfg()
	if err != nil {
		return err
	}

	// Get Kafka configuration
	brokers := getKafkaBrokers(cfg)
	if len(brokers) == 0 {
		return fmt.Errorf("no kafka brokers configured")
	}

	groupID := getKafkaGroupID(cfg)
	if groupID == "" {
		return fmt.Errorf("no kafka group ID configured")
	}

	topics := getKafkaTopics(cfg)
	if len(topics) == 0 {
		return fmt.Errorf("no kafka topics configured")
	}

	// Create Sarama configuration
	saramaConfig := sarama.NewConfig()
	saramaConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	saramaConfig.Consumer.Return.Errors = true

	// Create consumer group
	consumer, err := sarama.NewConsumerGroup(brokers, groupID, saramaConfig)
	if err != nil {
		return fmt.Errorf("error creating consumer group: %v", err)
	}
	s.consumer = consumer

	// Start consuming in a goroutine
	go func() {
		defer func() {
			if err := consumer.Close(); err != nil {
				log.Error().Err(err).Msg("error closing consumer")
			}
		}()

		for {
			select {
			case <-ctx.Done():
				log.Debug().Msg("kafka consumer context cancelled")
				return
			default:
				if err := consumer.Consume(ctx, topics, s.groupHandler); err != nil {
					log.Error().Err(err).Msg("error from consumer")
					s.stop <- err
					return
				}
				// Check if context was cancelled during consume
				if ctx.Err() != nil {
					return
				}
			}
		}
	}()

	// Handle consumer errors
	go func() {
		for err := range consumer.Errors() {
			log.Error().Err(err).Msg("kafka consumer error")
		}
	}()

	// Wait for consumer to be ready
	select {
	case <-s.groupHandler.Ready():
		log.Info().Msg("kafka consumer ready")
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for kafka consumer to be ready")
	}

	return nil
}

func getKafkaBrokers(cfg map[string]string) []string {
	brokersStr := cfg["KAFKA_BROKERS"]
	if brokersStr == "" {
		brokersStr = cfg["KAFKA_BOOTSTRAP_SERVERS"]
	}
	if brokersStr == "" {
		return []string{"localhost:9092"} // Default for development
	}
	return strings.Split(brokersStr, ",")
}

func getKafkaGroupID(cfg map[string]string) string {
	groupID := cfg["KAFKA_GROUP_ID"]
	if groupID == "" {
		groupID = cfg["KAFKA_CONSUMER_GROUP"]
	}
	if groupID == "" {
		groupID = "func-go-consumer"
	}
	return groupID
}

func getKafkaTopics(cfg map[string]string) []string {
	topicsStr := cfg["KAFKA_TOPICS"]
	if topicsStr == "" {
		topicsStr = cfg["KAFKA_TOPIC"]
	}
	if topicsStr == "" {
		return []string{"my-topic"} // Default topic
	}
	return strings.Split(topicsStr, ",")
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
// Passed in is the message received on the stop channel, which is either an
// error in the case of a runtime error, or nil in the case of a context
// cancellation or sigint/sigkill.
func (s *Service) shutdown(sourceErr error) (err error) {
	log.Debug().Msg("function stopping")
	var instanceErr, consumerErr error

	// Start a graceful shutdown of the Kafka consumer
	if s.consumer != nil {
		consumerErr = s.consumer.Close()
	}

	// Start a graceful shutdown of the Function instance
	if i, ok := s.f.(Stopper); ok {
		ctx, cancel := context.WithTimeout(context.Background(), InstanceStopTimeout)
		defer cancel()
		instanceErr = i.Stop(ctx)
	}

	return collapseErrors("shutdown error", sourceErr, instanceErr, consumerErr)
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
