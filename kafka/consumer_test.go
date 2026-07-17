package kafka

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestKafkaMessageToEvent(t *testing.T) {
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	msg := Message{
		Key:       []byte("my-key"),
		Value:     []byte(`{"hello":"world"}`),
		Topic:     "my-topic",
		Partition: 2,
		Offset:    42,
		Timestamp: ts,
	}

	e := kafkaMessageToEvent(msg, "broker1:9092,broker2:9092")

	if e.SpecVersion() != "1.0" {
		t.Errorf("specversion = %q, want 1.0", e.SpecVersion())
	}
	if e.ID() != "partition:2/offset:42" {
		t.Errorf("id = %q, want partition:2/offset:42", e.ID())
	}
	if e.Source() != "kafka://broker1:9092,broker2:9092/my-topic" {
		t.Errorf("source = %q", e.Source())
	}
	if e.Type() != "dev.knative.kafka.event" {
		t.Errorf("type = %q", e.Type())
	}
	if e.Subject() != "my-key" {
		t.Errorf("subject = %q, want my-key", e.Subject())
	}
	if !e.Time().Equal(ts) {
		t.Errorf("time = %v, want %v", e.Time(), ts)
	}
	if string(e.Data()) != `{"hello":"world"}` {
		t.Errorf("data = %q", string(e.Data()))
	}

	exts := e.Extensions()
	if exts["kafkatopic"] != "my-topic" {
		t.Errorf("kafkatopic = %v", exts["kafkatopic"])
	}
	if fmt.Sprintf("%v", exts["kafkapartition"]) != "2" {
		t.Errorf("kafkapartition = %v", exts["kafkapartition"])
	}
	if fmt.Sprintf("%v", exts["kafkaoffset"]) != "42" {
		t.Errorf("kafkaoffset = %v", exts["kafkaoffset"])
	}
	if exts["kafkakey"] != "my-key" {
		t.Errorf("kafkakey = %v", exts["kafkakey"])
	}
}

func TestKafkaMessageToEvent_NoKey(t *testing.T) {
	msg := Message{
		Value:     []byte("data"),
		Topic:     "t",
		Partition: 0,
		Offset:    0,
		Timestamp: time.Now(),
	}

	e := kafkaMessageToEvent(msg, "b:9092")

	if e.Subject() != "" {
		t.Errorf("subject should be empty when key is nil, got %q", e.Subject())
	}
	if _, ok := e.Extensions()["kafkakey"]; ok {
		t.Error("kafkakey extension should not be set when key is nil")
	}
}

func TestKafkaMessageToEvent_CEPassThrough(t *testing.T) {
	msg := Message{
		Value: []byte(`{"temperature":22}`),
		Topic: "events",
		Headers: []Header{
			{Key: "ce_specversion", Value: []byte("1.0")},
			{Key: "ce_id", Value: []byte("abc-123")},
			{Key: "ce_source", Value: []byte("//my-sensor")},
			{Key: "ce_type", Value: []byte("sensor.reading")},
			{Key: "ce_subject", Value: []byte("temp")},
			{Key: "ce_time", Value: []byte("2025-06-15T12:00:00Z")},
			{Key: "ce_datacontenttype", Value: []byte("application/json")},
			{Key: "ce_dataschema", Value: []byte("https://example.com/schema/sensor.json")},
			{Key: "ce_customext", Value: []byte("custom-value")},
		},
		Partition: 1,
		Offset:    99,
	}

	e := kafkaMessageToEvent(msg, "b:9092")

	if e.SpecVersion() != "1.0" {
		t.Errorf("specversion = %q", e.SpecVersion())
	}
	if e.ID() != "abc-123" {
		t.Errorf("id = %q, want abc-123", e.ID())
	}
	if e.Source() != "//my-sensor" {
		t.Errorf("source = %q", e.Source())
	}
	if e.Type() != "sensor.reading" {
		t.Errorf("type = %q", e.Type())
	}
	if e.Subject() != "temp" {
		t.Errorf("subject = %q", e.Subject())
	}
	expectedTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	if !e.Time().Equal(expectedTime) {
		t.Errorf("time = %v, want %v", e.Time(), expectedTime)
	}
	if e.DataContentType() != "application/json" {
		t.Errorf("datacontenttype = %q", e.DataContentType())
	}
	if string(e.Data()) != `{"temperature":22}` {
		t.Errorf("data = %q", string(e.Data()))
	}
	if e.DataSchema() != "https://example.com/schema/sensor.json" {
		t.Errorf("dataschema = %q, want https://example.com/schema/sensor.json", e.DataSchema())
	}
	if v, ok := e.Extensions()["customext"]; !ok || v != "custom-value" {
		t.Errorf("customext = %v", v)
	}
}

func TestKafkaMessageToEvent_CEPassThrough_DefaultContentType(t *testing.T) {
	msg := Message{
		Value: []byte(`{}`),
		Headers: []Header{
			{Key: "ce_specversion", Value: []byte("1.0")},
			{Key: "ce_id", Value: []byte("x")},
			{Key: "ce_source", Value: []byte("s")},
			{Key: "ce_type", Value: []byte("t")},
		},
	}

	e := kafkaMessageToEvent(msg, "b:9092")

	if e.DataContentType() != "application/json" {
		t.Errorf("default content type = %q, want application/json", e.DataContentType())
	}
}

func TestKafkaMessageToEvent_CEPassThrough_ContentTypeHeader(t *testing.T) {
	msg := Message{
		Value: []byte(`<xml/>`),
		Headers: []Header{
			{Key: "ce_specversion", Value: []byte("1.0")},
			{Key: "ce_id", Value: []byte("x")},
			{Key: "ce_source", Value: []byte("s")},
			{Key: "ce_type", Value: []byte("t")},
			{Key: "content-type", Value: []byte("application/xml")},
		},
	}

	e := kafkaMessageToEvent(msg, "b:9092")

	if e.DataContentType() != "application/xml" {
		t.Errorf("content type = %q, want application/xml", e.DataContentType())
	}
}

func TestKafkaMessageToEvent_CEPassThrough_CEDataContentTypeOverridesContentType(t *testing.T) {
	msg := Message{
		Value: []byte(`{}`),
		Headers: []Header{
			{Key: "ce_specversion", Value: []byte("1.0")},
			{Key: "ce_id", Value: []byte("x")},
			{Key: "ce_source", Value: []byte("s")},
			{Key: "ce_type", Value: []byte("t")},
			{Key: "content-type", Value: []byte("application/xml")},
			{Key: "ce_datacontenttype", Value: []byte("text/plain")},
		},
	}

	e := kafkaMessageToEvent(msg, "b:9092")

	if e.DataContentType() != "text/plain" {
		t.Errorf("content type = %q, want text/plain (ce_datacontenttype should override content-type)", e.DataContentType())
	}
}

func TestKafkaMessageToEvent_NotCE(t *testing.T) {
	msg := Message{
		Value: []byte("plain data"),
		Headers: []Header{
			{Key: "x-custom", Value: []byte("val")},
		},
		Topic:     "t",
		Partition: 0,
		Offset:    0,
		Timestamp: time.Now(),
	}

	e := kafkaMessageToEvent(msg, "b:9092")

	if e.Type() != "dev.knative.kafka.event" {
		t.Errorf("non-CE message should get type dev.knative.kafka.event, got %q", e.Type())
	}
}

func TestKafkaMessageToEvent_CEPassThrough_MissingAttributes(t *testing.T) {
	// Message has ce_specversion but is missing ce_id, ce_source, ce_type.
	// It should still be treated as a CE pass-through (not wrapped), and the
	// missing required attributes should be empty strings.
	msg := Message{
		Value: []byte(`{"data":"value"}`),
		Topic: "events",
		Headers: []Header{
			{Key: "ce_specversion", Value: []byte("1.0")},
		},
		Partition: 0,
		Offset:    10,
	}

	e := kafkaMessageToEvent(msg, "b:9092")

	// Should be treated as CE pass-through, not the generated wrapper.
	if e.SpecVersion() != "1.0" {
		t.Errorf("specversion = %q, want 1.0", e.SpecVersion())
	}
	// The wrapper would set type to "dev.knative.kafka.event", so verify it is
	// empty (CE pass-through with missing ce_type).
	if e.Type() != "" {
		t.Errorf("type = %q, want empty string", e.Type())
	}
	if e.ID() != "" {
		t.Errorf("id = %q, want empty string", e.ID())
	}
	if e.Source() != "" {
		t.Errorf("source = %q, want empty string", e.Source())
	}
	// Data should still be set.
	if string(e.Data()) != `{"data":"value"}` {
		t.Errorf("data = %q", string(e.Data()))
	}
}

// placeholderHandler is a minimal handler used in consumeLoop tests.
// consumeLoop fails on env-var validation before it tries to invoke the handler.
type placeholderHandler struct{}

func (h *placeholderHandler) Handle() {}

func TestConsumeLoop_MissingEnvVars(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr string
	}{
		{
			name:    "no KAFKA_BROKERS",
			envVars: map[string]string{},
			wantErr: "KAFKA_BROKERS",
		},
		{
			name: "no KAFKA_TOPIC",
			envVars: map[string]string{
				"KAFKA_BROKERS": "localhost:9092",
			},
			wantErr: "KAFKA_TOPIC",
		},
		{
			name: "no KAFKA_CONSUMER_GROUP",
			envVars: map[string]string{
				"KAFKA_BROKERS": "localhost:9092",
				"KAFKA_TOPIC":   "my-topic",
			},
			wantErr: "KAFKA_CONSUMER_GROUP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all three env vars, then set only what the subtest provides.
			t.Setenv("KAFKA_BROKERS", "")
			t.Setenv("KAFKA_TOPIC", "")
			t.Setenv("KAFKA_CONSUMER_GROUP", "")
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			var ready atomic.Bool
			err := consumeLoop(context.Background(), &placeholderHandler{}, &ready)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
