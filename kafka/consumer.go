package kafka

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/rs/zerolog/log"
)

func kafkaBrokers() []string {
	v := os.Getenv("KAFKA_BROKERS")
	if v == "" {
		return nil
	}
	return splitAndTrim(v)
}

func kafkaTopic() string {
	return strings.TrimSpace(os.Getenv("KAFKA_TOPIC"))
}

func kafkaConsumerGroup() string {
	return strings.TrimSpace(os.Getenv("KAFKA_CONSUMER_GROUP"))
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func consumeLoop(ctx context.Context, f any, ready *atomic.Bool) error {
	defer ready.Store(false)

	brokers := kafkaBrokers()
	topic := kafkaTopic()
	group := kafkaConsumerGroup()

	if len(brokers) == 0 {
		return fmt.Errorf("KAFKA_BROKERS environment variable is required")
	}
	if topic == "" {
		return fmt.Errorf("KAFKA_TOPIC environment variable is required")
	}
	if group == "" {
		return fmt.Errorf("KAFKA_CONSUMER_GROUP environment variable is required")
	}

	log.Info().
		Strs("brokers", brokers).
		Str("topic", topic).
		Str("group", group).
		Msg("connecting to kafka")

	config := sarama.NewConfig()
	config.Version = sarama.V2_0_0_0
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	config.Consumer.Offsets.Initial = sarama.OffsetNewest

	client, err := sarama.NewConsumerGroup(brokers, group, config)
	if err != nil {
		return fmt.Errorf("creating consumer group: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Error().Err(err).Msg("error closing kafka consumer group")
		}
	}()

	handler := &consumerGroupHandler{
		f:       f,
		ready:   ready,
		brokers: strings.Join(brokers, ","),
	}

	for {
		if err := client.Consume(ctx, []string{topic}, handler); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("consumer error: %w", err)
		}
		if ctx.Err() != nil {
			return nil
		}
		ready.Store(false)
	}
}

type consumerGroupHandler struct {
	f       any
	ready   *atomic.Bool
	brokers string
}

func (h *consumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error {
	h.ready.Store(true)
	log.Info().Msg("kafka consumer ready (partitions assigned)")
	return nil
}

func (h *consumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
	h.ready.Store(false)
	log.Info().Msg("kafka consumer partitions revoked")
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			m := Message{
				Key:       msg.Key,
				Value:     msg.Value,
				Topic:     msg.Topic,
				Partition: msg.Partition,
				Offset:    msg.Offset,
				Timestamp: msg.Timestamp,
			}
			for _, rh := range msg.Headers {
				if rh != nil {
					m.Headers = append(m.Headers, Header{
						Key:   string(rh.Key),
						Value: rh.Value,
					})
				}
			}

			e := kafkaMessageToEvent(m, h.brokers)

			if err := invokeHandler(session.Context(), h.f, e); err != nil {
				log.Error().Err(err).
					Str("topic", msg.Topic).
					Int32("partition", msg.Partition).
					Int64("offset", msg.Offset).
					Msg("error handling kafka message")
				continue
			}
			session.MarkMessage(msg, "")
		case <-session.Context().Done():
			return nil
		}
	}
}

// kafkaMessageToEvent converts a Kafka message to a CloudEvent.
// If the message already contains CloudEvent headers (ce_ prefix per the
// CloudEvents Kafka Protocol Binding), it is parsed as an existing CloudEvent
// rather than re-wrapped.
func kafkaMessageToEvent(msg Message, brokers string) event.Event {
	if e, ok := parseCEFromHeaders(msg); ok {
		return e
	}

	e := event.New()
	e.SetSpecVersion("1.0")
	e.SetID(fmt.Sprintf("partition:%d/offset:%d", msg.Partition, msg.Offset))
	e.SetSource(fmt.Sprintf("kafka://%s/%s", brokers, msg.Topic))
	e.SetType("dev.knative.kafka.event")
	e.SetTime(msg.Timestamp)
	if len(msg.Key) > 0 {
		e.SetSubject(string(msg.Key))
	}
	_ = e.SetData("application/octet-stream", msg.Value)
	e.SetExtension("kafkatopic", msg.Topic)
	e.SetExtension("kafkapartition", msg.Partition)
	e.SetExtension("kafkaoffset", msg.Offset)
	if len(msg.Key) > 0 {
		e.SetExtension("kafkakey", string(msg.Key))
	}
	return e
}

// parseCEFromHeaders checks if the Kafka message is already a CloudEvent
// (binary content mode with ce_ prefixed headers) and parses it.
func parseCEFromHeaders(msg Message) (event.Event, bool) {
	headers := make(map[string]string)
	for _, h := range msg.Headers {
		headers[strings.ToLower(h.Key)] = string(h.Value)
	}

	if _, ok := headers["ce_specversion"]; !ok {
		return event.Event{}, false
	}

	e := event.New()
	e.SetSpecVersion(headers["ce_specversion"])
	if v, ok := headers["ce_id"]; ok {
		e.SetID(v)
	}
	if v, ok := headers["ce_source"]; ok {
		e.SetSource(v)
	}
	if v, ok := headers["ce_type"]; ok {
		e.SetType(v)
	}
	if v, ok := headers["ce_subject"]; ok {
		e.SetSubject(v)
	}
	if v, ok := headers["ce_time"]; ok {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err == nil {
			e.SetTime(t)
		}
	}
	if v, ok := headers["ce_dataschema"]; ok {
		e.SetDataSchema(v)
	}

	contentType := "application/json"
	if v, ok := headers["content-type"]; ok {
		contentType = v
	}
	if v, ok := headers["ce_datacontenttype"]; ok {
		contentType = v
	}
	_ = e.SetData(contentType, msg.Value)

	// Set non-standard ce_ headers as extensions
	for k, v := range headers {
		if strings.HasPrefix(k, "ce_") {
			attr := strings.TrimPrefix(k, "ce_")
			switch attr {
			case "specversion", "id", "source", "type", "subject", "time", "datacontenttype", "dataschema":
				continue
			default:
				e.SetExtension(attr, v)
			}
		}
	}

	return e, true
}
