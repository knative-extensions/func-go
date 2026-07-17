package kafka

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	DefaultListenAddress  = "[::]:8080"
	ServerShutdownTimeout = 30 * time.Second
	InstanceStopTimeout   = 30 * time.Second
)

// Start a Kafka consumer service using the given CloudEvents handler.
// The handler must implement one of the supported CloudEvents Handle signatures.
func Start(f any) error {
	log.Debug().Msg("func runtime creating kafka consumer instance")
	return New(f).Start(context.Background())
}

// Service runs a Kafka consumer that delivers messages as CloudEvents to
// the function handler. An HTTP server runs alongside for health probes only.
type Service struct {
	http.Server
	listener      net.Listener
	f             any
	stop          chan error
	ready         atomic.Bool
	cancelConsume context.CancelFunc
}

// New creates a Service for the given handler.
func New(f any) *Service {
	svc := &Service{
		f:    f,
		stop: make(chan error, 1),
		Server: http.Server{
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       30 * time.Second,
			MaxHeaderBytes:    1 << 20,
			ReadHeaderTimeout: 2 * time.Second,
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health/readiness", svc.Ready)
	mux.HandleFunc("/health/liveness", svc.Alive)
	svc.Handler = mux
	return svc
}

// Start the Kafka consumer and health HTTP server.
func (s *Service) Start(ctx context.Context) (err error) {
	if err = validateHandler(s.f); err != nil {
		return
	}

	addr := listenAddress()
	log.Debug().Str("address", addr).Msg("kafka service starting")

	if s.listener, err = net.Listen("tcp", addr); err != nil {
		return
	}

	if err = s.startInstance(ctx); err != nil {
		s.listener.Close()
		return
	}

	s.handleSignals()

	go func() {
		if err := s.Serve(s.listener); err != http.ErrServerClosed {
			log.Error().Err(err).Msg("http server exited with unexpected error")
			s.sendStop(err)
		}
	}()

	consumerCtx, cancelConsume := context.WithCancel(ctx)
	s.cancelConsume = cancelConsume
	go func() {
		if err := consumeLoop(consumerCtx, s.f, &s.ready); err != nil {
			log.Error().Err(err).Msg("kafka consumer exited with error")
			s.sendStop(err)
		}
	}()

	log.Debug().Msg("waiting for stop signals or errors")
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

// Addr returns the address upon which the health server is listening.
func (s *Service) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Ready handles readiness checks.
func (s *Service) Ready(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, "kafka consumer not yet ready")
		return
	}
	if i, ok := s.f.(ReadinessReporter); ok {
		ready, err := i.Ready(r.Context())
		if err != nil {
			log.Debug().Err(err).Msg("error checking readiness")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "error checking readiness: ", err.Error())
			return
		}
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "function not yet ready")
			return
		}
	}
	fmt.Fprintf(w, "READY")
}

// Alive handles liveness checks.
func (s *Service) Alive(w http.ResponseWriter, r *http.Request) {
	if i, ok := s.f.(LivenessReporter); ok {
		alive, err := i.Alive(r.Context())
		if err != nil {
			log.Err(err).Msg("error checking liveness")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "error checking liveness: ", err.Error())
			return
		}
		if !alive {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("function not alive"))
			return
		}
	}
	fmt.Fprintf(w, "ALIVE")
}

func (s *Service) startInstance(ctx context.Context) error {
	if i, ok := s.f.(Starter); ok {
		cfg, err := newCfg()
		if err != nil {
			return err
		}
		go func() {
			if err := i.Start(ctx, cfg); err != nil {
				s.sendStop(err)
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
				s.sendStop(nil)
			} else if runtime.GOOS == "linux" && sig == syscall.Signal(0x17) {
				// Ignore SIGURG
			}
		}
	}()
}

func (s *Service) sendStop(err error) {
	select {
	case s.stop <- err:
	default:
	}
}

func (s *Service) shutdown(sourceErr error) (err error) {
	log.Debug().Msg("function stopping")
	if s.cancelConsume != nil {
		s.cancelConsume()
	}
	var runtimeErr, instanceErr error

	ctx, cancel := context.WithTimeout(context.Background(), ServerShutdownTimeout)
	defer cancel()
	runtimeErr = s.Shutdown(ctx)

	if i, ok := s.f.(Stopper); ok {
		ctx, cancel = context.WithTimeout(context.Background(), InstanceStopTimeout)
		defer cancel()
		instanceErr = i.Stop(ctx)
	}

	return collapseErrors("shutdown error", sourceErr, instanceErr, runtimeErr)
}

func listenAddress() string {
	if v := os.Getenv("LISTEN_ADDRESS"); v != "" {
		return v
	}
	return DefaultListenAddress
}

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
