package kafka

import "time"

// Message represents a Kafka message delivered to the function's handler.
type Message struct {
	Key       []byte
	Value     []byte
	Headers   []Header
	Topic     string
	Partition int32
	Offset    int64
	Timestamp time.Time
}

// Header is a key-value pair attached to a Kafka message.
type Header struct {
	Key   string
	Value []byte
}
