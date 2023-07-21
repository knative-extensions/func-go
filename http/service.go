// Package http implements a Functions HTTP middleware for use by
// scaffolding which exposes a function as a network service which handles
// http requests.
package http

import (
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

const DefaultLogLevel = LogDebug

const (
	ServerShutdownTimeout = 30 * time.Second
	InstanceStopTimeout   = 30 * time.Second
)

// Start an intance using a new Service
// Note that for CloudEvent Handlers this effectively accepts ANY because
// the actual type of the handler function is determined later.
func Start(f any) error {
	log.Debug().Msg("func runtime creating function instance")
	return New(f).Start(context.Background())
}

// Service exposes a Function Instance as a an HTTP service.
type Service struct {
	http.Server
	stop chan error
	f    any
}

// listen on ADDRESS:PORT.
// If port is defined only, the default is to listen on the 127.0.0.1 loopback
// interface for security reasons.
// The OS chooses the port if it is empty or zero.
func (s *Service) listen() (lis net.Listener, err error) {
	if lis, err = net.Listen("tcp", listenAddress()); err != nil {
		return
	}
	log.Info().Any("address", listenAddress()).Msg("listening")
	return
}

func listenAddress() string {
	addr := os.Getenv("ADDRESS")
	port := os.Getenv("PORT")
	if addr == "" {
		addr = "127.0.0.1"
	}
	if port == "" {
		port = "8080"
	}
	return addr + ":" + port
}

// New Service which serves the given instance.
func New(f any) *Service {
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
	svc.Server.Handler = mux
	return svc
}

// Start
// Will stop when the context is canceled, a runtime error is encountered,
// or an os interrupt or kill signal is received.
func (s *Service) Start(ctx context.Context) (err error) {
	log.Debug().Msg("function starting")

	// Start the function instance in a separate routine, sending any
	// runtime errors on s.stop.
	s.startInstance(ctx)

	// Start listening for interrupt and kill signals in a separate routine,
	// sending a nil error on the s.stop channel if either are received.
	s.handleSignals()

	// Start the HTTP listener in a separate routine, sending any runtime errors
	// on s.stop.
	s.handleRequests()

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

// Handle requests for the instance
func (s *Service) Handle(w http.ResponseWriter, r *http.Request) {
	if i, ok := s.f.(Handler); ok {
		i.Handle(r.Context(), w, r)
	} else {
		message := "function does not implement Handle. Skipping"
		log.Debug().Msg(message)
		w.Write([]byte(message))
	}
}

// Ready handles readiness checks.
func (s *Service) Ready(w http.ResponseWriter, r *http.Request) {
	if i, ok := s.f.(ReadinessReporter); ok {
		ready, err := i.Ready(r.Context())
		if err != nil {
			message := "error checking readiness"
			log.Debug().Err(err).Msg(message)
			w.WriteHeader(500)
			w.Write([]byte(message + ". " + err.Error()))
			return
		}
		if !ready {
			message := "function not yet available"
			log.Debug().Msg(message)
			w.WriteHeader(503)
			w.Write([]byte(message))
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
			w.WriteHeader(500)
			w.Write([]byte(message + ". " + err.Error()))
			return
		}
		if !alive {
			message := "function not ready"
			log.Debug().Msg(message)
			w.WriteHeader(503)
			w.Write([]byte(message))
			return
		}
	}
	fmt.Fprintf(w, "ALIVE")
}

func (s *Service) startInstance(ctx context.Context) {
	if i, ok := s.f.(Starter); ok {
		go func() {
			if err := i.Start(ctx, allEnvs()); err != nil {
				s.stop <- err
			}
		}()
	} else {
		log.Debug().Msg("function does not implement Start. Skipping")
	}
}

func (s *Service) handleRequests() {
	lis, err := s.listen()
	if err != nil {
		s.stop <- err
		return
	}

	go func() {
		if err = s.Server.Serve(lis); err != http.ErrServerClosed {
			log.Error().Err(err).Msg("http server exited with unexpected error")
			s.stop <- err
		}
	}()
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
			} else {
				log.Debug().Any("signal", sig).Msg("signal ignored")
			}
		}
	}()
}

func allEnvs() (envs map[string]string) {
	envs = make(map[string]string, len(os.Environ()))
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		envs[pair[0]] = pair[1]
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
