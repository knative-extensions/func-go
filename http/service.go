// Package http implements a Functions HTTP middleware for use by
// scaffolding which exposes a function as a network service which handles
// http requests.
package http

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
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	DefaultLogLevel      = LogDebug
	DefaultListenAddress = "127.0.0.1:8080"
)

const (
	ServerShutdownTimeout = 30 * time.Second
	InstanceStopTimeout   = 30 * time.Second
)

// Start an intance using a new Service
// Note that for CloudEvent Handlers this effectively accepts ANY because
// the actual type of the handler function is determined later.
func Start(f Handler) error {
	log.Debug().Msg("func runtime creating function instance")
	return New(f).Start(context.Background())
}

// Service exposes a Function Instance as a an HTTP service.
type Service struct {
	http.Server
	listener net.Listener
	stop     chan error
	f        Handler
}

// New Service which serves the given instance.
func New(f Handler) *Service {
	svc := &Service{
		f:    f,
		stop: make(chan error),
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
	mux.HandleFunc("/", svc.Handle)
	svc.Handler = mux

	// Print some helpful information about which interfaces the function
	// is correctly implementing
	logImplements(f)

	return svc
}

// log which interfaces the function implements.
// This could be more verbose for new users:
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

// Start
// Will stop when the context is canceled, a runtime error is encountered,
// or an os interrupt or kill signal is received.
// By default it listens on the default address DefaultListenAddress.
// This can be modified using the environment variable LISTEN_ADDRESS
func (s *Service) Start(ctx context.Context) (err error) {
	// Get the listen address
	// TODO: Currently this is an env var for legacy reasons. Logic should
	// be moved into the generated mainfiles, and this setting be an optional
	// functional option WithListenAddress(os.Getenv("LISTEN_ADDRESS"))
	addr := listenAddress()
	log.Debug().Str("address", addr).Msg("function starting")

	// Listen
	if s.listener, err = net.Listen("tcp", addr); err != nil {
		return
	}

	// Start
	// Starts the function instance in a separate routine, sending any
	// runtime errors on s.stop.
	if err = s.startInstance(ctx); err != nil {
		return
	}

	// Wait for signals
	// Interrupts and Kill signals
	// sending a message on the s.stop channel if either are received.
	s.handleSignals()

	// Listen and serve
	go func() {
		if err := s.Serve(s.listener); err != http.ErrServerClosed {
			log.Error().Err(err).Msg("http server exited with unexpected error")
			s.stop <- err
		}
	}()

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

func listenAddress() string {
	// If they are using the corret LISTEN_ADRESS, use this immediately
	listenAddress := os.Getenv("LISTEN_ADDRESS")
	if listenAddress != "" {
		return listenAddress
	}

	// Legacy logic if ADDRESS or PORT provided
	address := os.Getenv("ADDRESS")
	port := os.Getenv("PORT")
	if address != "" || port != "" {
		if address != "" {
			log.Warn().Msg("Environment variable ADDRESS is deprecated and support will be removed in future versions.  Try rebuilding your Function with the latest version of func to use LISTEN_ADDRESS instead.")
		} else {
			address = "127.0.0.1"
		}
		if port != "" {
			log.Warn().Msg("Environment variable PORT is deprecated and support will be removed in future version.s  Try rebuilding your Function with the latest version of func to use LISTEN_ADDRESS instead.")
		} else {
			port = "8080"
		}
		return address + ":" + port
	}

	return DefaultListenAddress
}

// Addr returns the address upon which the service is listening if started;
// nil otherwise.
func (s *Service) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Handle requests for the instance
func (s *Service) Handle(w http.ResponseWriter, r *http.Request) {
	s.f.Handle(w, r)
}

// Ready handles readiness checks.
func (s *Service) Ready(w http.ResponseWriter, r *http.Request) {
	if i, ok := s.f.(ReadinessReporter); ok {
		ready, err := i.Ready(r.Context())
		if err != nil {
			message := "error checking readiness"
			log.Debug().Err(err).Msg(message)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "error checking readiness: ", err.Error())
			return
		}
		if !ready {
			message := "function not yet ready"
			log.Debug().Msg(message)
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, message)
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
			message := "error checking liveness"
			log.Err(err).Msg(message)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "error checking liveness: ", err.Error())
			return
		}
		if !alive {
			message := "function not alive"
			log.Debug().Msg(message)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(message))
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
// Passed in is the message received on the stop channel, wich is either an
// error in the case of a runtime error, or nil in the case of a context
// cancellation or sigint/sigkill.
func (s *Service) shutdown(sourceErr error) (err error) {
	log.Debug().Msg("function stopping")
	var runtimeErr, instanceErr error

	// Start a graceful shutdown of the HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), ServerShutdownTimeout)
	defer cancel()
	runtimeErr = s.Shutdown(ctx)

	//  Start a graceful shutdown of the Function instance
	if i, ok := s.f.(Stopper); ok {
		ctx, cancel = context.WithTimeout(context.Background(), InstanceStopTimeout)
		defer cancel()
		instanceErr = i.Stop(ctx)
	}

	return collapseErrors("shutdown error", sourceErr, instanceErr, runtimeErr)
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
