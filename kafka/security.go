package kafka

import (
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"hash"
	"os"
	"strings"

	"github.com/IBM/sarama"
	"github.com/rs/zerolog/log"
	"github.com/xdg-go/scram"
)

func configureSecurity(config *sarama.Config) error {
	protocol := strings.TrimSpace(os.Getenv("KAFKA_SECURITY_PROTOCOL"))
	if protocol == "" {
		protocol = "PLAINTEXT"
	}

	switch protocol {
	case "PLAINTEXT":
		return nil
	case "SSL":
		return configureTLS(config)
	case "SASL_PLAINTEXT":
		return configureSASL(config)
	case "SASL_SSL":
		if err := configureTLS(config); err != nil {
			return err
		}
		return configureSASL(config)
	default:
		return fmt.Errorf("unsupported KAFKA_SECURITY_PROTOCOL: %s (expected PLAINTEXT, SSL, SASL_PLAINTEXT, or SASL_SSL)", protocol)
	}
}

func configureTLS(config *sarama.Config) error {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if caPath := os.Getenv("KAFKA_TLS_CA_CERT"); caPath != "" {
		caCert, err := os.ReadFile(caPath)
		if err != nil {
			return fmt.Errorf("reading CA certificate %s: %w", caPath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("failed to parse CA certificate from %s", caPath)
		}
		tlsConfig.RootCAs = pool
		log.Debug().Str("path", caPath).Msg("loaded kafka CA certificate")
	}

	clientCert := os.Getenv("KAFKA_TLS_CLIENT_CERT")
	clientKey := os.Getenv("KAFKA_TLS_CLIENT_KEY")
	if (clientCert != "") != (clientKey != "") {
		return fmt.Errorf("both KAFKA_TLS_CLIENT_CERT and KAFKA_TLS_CLIENT_KEY must be set for mutual TLS (got cert=%q, key=%q)", clientCert, clientKey)
	}
	if clientCert != "" && clientKey != "" {
		cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
		if err != nil {
			return fmt.Errorf("loading client certificate/key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
		log.Debug().Str("cert", clientCert).Msg("loaded kafka client certificate for mutual TLS")
	}

	if strings.EqualFold(os.Getenv("KAFKA_TLS_SKIP_VERIFY"), "true") {
		tlsConfig.InsecureSkipVerify = true
		log.Warn().Msg("kafka TLS: broker certificate verification is disabled")
	}

	config.Net.TLS.Enable = true
	config.Net.TLS.Config = tlsConfig
	return nil
}

func configureSASL(config *sarama.Config) error {
	config.Net.SASL.Enable = true
	config.Net.SASL.User = os.Getenv("KAFKA_SASL_USER")
	config.Net.SASL.Password = os.Getenv("KAFKA_SASL_PASSWORD")
	if config.Net.SASL.User == "" || config.Net.SASL.Password == "" {
		return fmt.Errorf("KAFKA_SASL_USER and KAFKA_SASL_PASSWORD must both be set when SASL is enabled")
	}

	mechanism := strings.TrimSpace(os.Getenv("KAFKA_SASL_MECHANISM"))
	if mechanism == "" {
		mechanism = "PLAIN"
	}

	switch mechanism {
	case "PLAIN":
		config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
	case "SCRAM-SHA-256":
		config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
		config.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
			return &scramClient{hashGen: sha256.New}
		}
	case "SCRAM-SHA-512":
		config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
		config.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
			return &scramClient{hashGen: sha512.New}
		}
	default:
		return fmt.Errorf("unsupported KAFKA_SASL_MECHANISM: %s (expected PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512)", mechanism)
	}

	log.Debug().Str("mechanism", mechanism).Msg("kafka SASL configured")
	return nil
}

type scramClient struct {
	conv    *scram.ClientConversation
	hashGen func() hash.Hash
}

func (c *scramClient) Begin(userName, password, authzID string) error {
	client, err := scram.HashGeneratorFcn(c.hashGen).NewClient(userName, password, authzID)
	if err != nil {
		return fmt.Errorf("SCRAM client configuration failed (check KAFKA_SASL_USER and KAFKA_SASL_PASSWORD for invalid characters)")
	}
	c.conv = client.NewConversation()
	return nil
}

func (c *scramClient) Step(challenge string) (string, error) {
	return c.conv.Step(challenge)
}

func (c *scramClient) Done() bool {
	return c.conv.Done()
}
