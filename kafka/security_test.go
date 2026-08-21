package kafka

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/IBM/sarama"
)

func TestConfigureSecurity_Plaintext(t *testing.T) {
	t.Setenv("KAFKA_SECURITY_PROTOCOL", "PLAINTEXT")
	config := sarama.NewConfig()
	if err := configureSecurity(config); err != nil {
		t.Fatal(err)
	}
	if config.Net.TLS.Enable {
		t.Error("TLS should not be enabled for PLAINTEXT")
	}
	if config.Net.SASL.Enable {
		t.Error("SASL should not be enabled for PLAINTEXT")
	}
}

func TestConfigureSecurity_DefaultIsPlaintext(t *testing.T) {
	t.Setenv("KAFKA_SECURITY_PROTOCOL", "")
	config := sarama.NewConfig()
	if err := configureSecurity(config); err != nil {
		t.Fatal(err)
	}
	if config.Net.TLS.Enable {
		t.Error("TLS should not be enabled when protocol is empty")
	}
	if config.Net.SASL.Enable {
		t.Error("SASL should not be enabled when protocol is empty")
	}
}

func TestConfigureSecurity_SSL(t *testing.T) {
	t.Setenv("KAFKA_SECURITY_PROTOCOL", "SSL")
	config := sarama.NewConfig()
	if err := configureSecurity(config); err != nil {
		t.Fatal(err)
	}
	if !config.Net.TLS.Enable {
		t.Error("TLS should be enabled for SSL")
	}
	if config.Net.SASL.Enable {
		t.Error("SASL should not be enabled for SSL")
	}
}

func TestConfigureSecurity_SASL_PLAINTEXT(t *testing.T) {
	t.Setenv("KAFKA_SECURITY_PROTOCOL", "SASL_PLAINTEXT")
	t.Setenv("KAFKA_SASL_MECHANISM", "PLAIN")
	t.Setenv("KAFKA_SASL_USER", "alice")
	t.Setenv("KAFKA_SASL_PASSWORD", "s3cret")

	config := sarama.NewConfig()
	if err := configureSecurity(config); err != nil {
		t.Fatal(err)
	}
	if config.Net.TLS.Enable {
		t.Error("TLS should not be enabled for SASL_PLAINTEXT")
	}
	if !config.Net.SASL.Enable {
		t.Error("SASL should be enabled")
	}
	if config.Net.SASL.Mechanism != sarama.SASLTypePlaintext {
		t.Errorf("mechanism = %v, want %v", config.Net.SASL.Mechanism, sarama.SASLTypePlaintext)
	}
	if config.Net.SASL.User != "alice" {
		t.Errorf("user = %q, want alice", config.Net.SASL.User)
	}
	if config.Net.SASL.Password != "s3cret" {
		t.Errorf("password = %q, want s3cret", config.Net.SASL.Password)
	}
}

func TestConfigureSecurity_SASL_SSL(t *testing.T) {
	t.Setenv("KAFKA_SECURITY_PROTOCOL", "SASL_SSL")
	t.Setenv("KAFKA_SASL_MECHANISM", "PLAIN")
	t.Setenv("KAFKA_SASL_USER", "bob")
	t.Setenv("KAFKA_SASL_PASSWORD", "pw")

	config := sarama.NewConfig()
	if err := configureSecurity(config); err != nil {
		t.Fatal(err)
	}
	if !config.Net.TLS.Enable {
		t.Error("TLS should be enabled for SASL_SSL")
	}
	if !config.Net.SASL.Enable {
		t.Error("SASL should be enabled for SASL_SSL")
	}
}

func TestConfigureSecurity_InvalidProtocol(t *testing.T) {
	t.Setenv("KAFKA_SECURITY_PROTOCOL", "BOGUS")
	config := sarama.NewConfig()
	err := configureSecurity(config)
	if err == nil {
		t.Fatal("expected error for invalid protocol")
	}
}

func TestConfigureSASL_DefaultMechanism(t *testing.T) {
	t.Setenv("KAFKA_SASL_MECHANISM", "")
	t.Setenv("KAFKA_SASL_USER", "u")
	t.Setenv("KAFKA_SASL_PASSWORD", "p")

	config := sarama.NewConfig()
	if err := configureSASL(config); err != nil {
		t.Fatal(err)
	}
	if config.Net.SASL.Mechanism != sarama.SASLTypePlaintext {
		t.Errorf("default mechanism = %v, want PLAIN", config.Net.SASL.Mechanism)
	}
}

func TestConfigureSASL_SCRAM256(t *testing.T) {
	t.Setenv("KAFKA_SASL_MECHANISM", "SCRAM-SHA-256")
	t.Setenv("KAFKA_SASL_USER", "u")
	t.Setenv("KAFKA_SASL_PASSWORD", "p")

	config := sarama.NewConfig()
	if err := configureSASL(config); err != nil {
		t.Fatal(err)
	}
	if config.Net.SASL.Mechanism != sarama.SASLTypeSCRAMSHA256 {
		t.Errorf("mechanism = %v, want SCRAM-SHA-256", config.Net.SASL.Mechanism)
	}
	if config.Net.SASL.SCRAMClientGeneratorFunc == nil {
		t.Error("SCRAMClientGeneratorFunc should be set")
	}
}

func TestConfigureSASL_SCRAM512(t *testing.T) {
	t.Setenv("KAFKA_SASL_MECHANISM", "SCRAM-SHA-512")
	t.Setenv("KAFKA_SASL_USER", "u")
	t.Setenv("KAFKA_SASL_PASSWORD", "p")

	config := sarama.NewConfig()
	if err := configureSASL(config); err != nil {
		t.Fatal(err)
	}
	if config.Net.SASL.Mechanism != sarama.SASLTypeSCRAMSHA512 {
		t.Errorf("mechanism = %v, want SCRAM-SHA-512", config.Net.SASL.Mechanism)
	}
	if config.Net.SASL.SCRAMClientGeneratorFunc == nil {
		t.Error("SCRAMClientGeneratorFunc should be set")
	}
}

func TestConfigureSASL_InvalidMechanism(t *testing.T) {
	t.Setenv("KAFKA_SASL_MECHANISM", "OAUTHBEARER")
	t.Setenv("KAFKA_SASL_USER", "u")
	t.Setenv("KAFKA_SASL_PASSWORD", "p")
	config := sarama.NewConfig()
	err := configureSASL(config)
	if err == nil {
		t.Fatal("expected error for unsupported mechanism")
	}
}

func TestConfigureSASL_MissingCredentials(t *testing.T) {
	t.Setenv("KAFKA_SASL_MECHANISM", "PLAIN")

	t.Setenv("KAFKA_SASL_USER", "")
	t.Setenv("KAFKA_SASL_PASSWORD", "p")
	config := sarama.NewConfig()
	if err := configureSASL(config); err == nil {
		t.Fatal("expected error when SASL user is empty")
	}

	t.Setenv("KAFKA_SASL_USER", "u")
	t.Setenv("KAFKA_SASL_PASSWORD", "")
	config = sarama.NewConfig()
	if err := configureSASL(config); err == nil {
		t.Fatal("expected error when SASL password is empty")
	}
}

func TestConfigureTLS_CACert(t *testing.T) {
	caPath := writeTestCACert(t)
	t.Setenv("KAFKA_TLS_CA_CERT", caPath)
	t.Setenv("KAFKA_TLS_CLIENT_CERT", "")
	t.Setenv("KAFKA_TLS_CLIENT_KEY", "")
	t.Setenv("KAFKA_TLS_SKIP_VERIFY", "")

	config := sarama.NewConfig()
	if err := configureTLS(config); err != nil {
		t.Fatal(err)
	}
	if !config.Net.TLS.Enable {
		t.Error("TLS should be enabled")
	}
	if config.Net.TLS.Config.RootCAs == nil {
		t.Error("RootCAs should be set")
	}
	if config.Net.TLS.Config.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false")
	}
}

func TestConfigureTLS_MutualTLS(t *testing.T) {
	caPath, certPath, keyPath := writeTestCerts(t)
	t.Setenv("KAFKA_TLS_CA_CERT", caPath)
	t.Setenv("KAFKA_TLS_CLIENT_CERT", certPath)
	t.Setenv("KAFKA_TLS_CLIENT_KEY", keyPath)
	t.Setenv("KAFKA_TLS_SKIP_VERIFY", "")

	config := sarama.NewConfig()
	if err := configureTLS(config); err != nil {
		t.Fatal(err)
	}
	if len(config.Net.TLS.Config.Certificates) != 1 {
		t.Errorf("expected 1 client certificate, got %d", len(config.Net.TLS.Config.Certificates))
	}
}

func TestConfigureTLS_SkipVerify(t *testing.T) {
	t.Setenv("KAFKA_TLS_CA_CERT", "")
	t.Setenv("KAFKA_TLS_CLIENT_CERT", "")
	t.Setenv("KAFKA_TLS_CLIENT_KEY", "")
	t.Setenv("KAFKA_TLS_SKIP_VERIFY", "true")

	config := sarama.NewConfig()
	if err := configureTLS(config); err != nil {
		t.Fatal(err)
	}
	if !config.Net.TLS.Config.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestConfigureTLS_MismatchedCertKey(t *testing.T) {
	caPath := writeTestCACert(t)
	t.Setenv("KAFKA_TLS_CA_CERT", caPath)
	t.Setenv("KAFKA_TLS_CLIENT_CERT", "/some/cert.pem")
	t.Setenv("KAFKA_TLS_CLIENT_KEY", "")
	t.Setenv("KAFKA_TLS_SKIP_VERIFY", "")

	config := sarama.NewConfig()
	err := configureTLS(config)
	if err == nil {
		t.Fatal("expected error when only client cert is set without key")
	}

	t.Setenv("KAFKA_TLS_CLIENT_CERT", "")
	t.Setenv("KAFKA_TLS_CLIENT_KEY", "/some/key.pem")

	config = sarama.NewConfig()
	err = configureTLS(config)
	if err == nil {
		t.Fatal("expected error when only client key is set without cert")
	}
}

func TestConfigureTLS_BadCACert(t *testing.T) {
	t.Setenv("KAFKA_TLS_CA_CERT", "/nonexistent/ca.crt")
	t.Setenv("KAFKA_TLS_CLIENT_CERT", "")
	t.Setenv("KAFKA_TLS_CLIENT_KEY", "")
	t.Setenv("KAFKA_TLS_SKIP_VERIFY", "")

	config := sarama.NewConfig()
	err := configureTLS(config)
	if err == nil {
		t.Fatal("expected error for missing CA cert file")
	}
}

func TestConfigureTLS_InvalidCAPEM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-ca.crt")
	if err := os.WriteFile(path, []byte("not a pem"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("KAFKA_TLS_CA_CERT", path)
	t.Setenv("KAFKA_TLS_CLIENT_CERT", "")
	t.Setenv("KAFKA_TLS_CLIENT_KEY", "")
	t.Setenv("KAFKA_TLS_SKIP_VERIFY", "")

	config := sarama.NewConfig()
	err := configureTLS(config)
	if err == nil {
		t.Fatal("expected error for invalid PEM content")
	}
}

func TestConfigureTLS_MinVersion(t *testing.T) {
	t.Setenv("KAFKA_TLS_CA_CERT", "")
	t.Setenv("KAFKA_TLS_CLIENT_CERT", "")
	t.Setenv("KAFKA_TLS_CLIENT_KEY", "")
	t.Setenv("KAFKA_TLS_SKIP_VERIFY", "")

	config := sarama.NewConfig()
	if err := configureTLS(config); err != nil {
		t.Fatal(err)
	}
	// TLS 1.2 = 0x0303
	if config.Net.TLS.Config.MinVersion != 0x0303 {
		t.Errorf("MinVersion = %x, want TLS 1.2 (0x0303)", config.Net.TLS.Config.MinVersion)
	}
}

func TestScramClient_Begin(t *testing.T) {
	sc := &scramClient{hashGen: sha256.New}
	if err := sc.Begin("user", "pass", ""); err != nil {
		t.Fatalf("Begin failed: %v", err)
	}
	if sc.Done() {
		t.Error("should not be done before any steps")
	}
}

// --- test helpers ---

func writeTestCACert(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ca.crt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return path
}

func writeTestCerts(t *testing.T) (caPath, certPath, keyPath string) {
	t.Helper()
	dir := t.TempDir()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}

	caPath = filepath.Join(dir, "ca.crt")
	caF, err := os.Create(caPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = pem.Encode(caF, &pem.Block{Type: "CERTIFICATE", Bytes: caDER}); err != nil {
		t.Fatal(err)
	}
	caF.Close()

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Test Client"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTmpl, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	certPath = filepath.Join(dir, "client.crt")
	certF, err := os.Create(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = pem.Encode(certF, &pem.Block{Type: "CERTIFICATE", Bytes: clientDER}); err != nil {
		t.Fatal(err)
	}
	certF.Close()

	keyDER, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPath = filepath.Join(dir, "client.key")
	keyF, err := os.Create(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = pem.Encode(keyF, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatal(err)
	}
	keyF.Close()

	return
}
