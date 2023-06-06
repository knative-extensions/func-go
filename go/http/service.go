// Package http implements a Functions HTTP middleware for use by
// scaffolding which exposes a function as a network service which handles
// http requests.
package http

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

const Version = "v0.2.1"

const DefaultLogLevel = LogDebug

func init() {
	log.Debug().Str("version", Version).Msg("func runtime initializing")
}

const (
	DefaultServicePort    = "8080"
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
	done chan error
	f    any
}

// New Service which serves the given instance.
func New(f any) *Service {
	svc := &Service{
		f:    f,
		done: make(chan error),
		Server: http.Server{
			Addr:              ":" + port(),
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
func (s *Service) Start(ctx context.Context) (err error) {
	log.Debug().Msg("func runtime starting function")
	ee := make(chan error)
	if i, ok := s.f.(Starter); ok {
		go func() {
			if err = i.Start(ctx, allEnvs()); err != nil {
				ee <- err
			}
		}()
	} else {
		log.Debug().Msg("function does not implement Start. Skipping")
	}
	s.handleRequests()
	s.handleSignals()
	select {
	case err = <-s.done:
		log.Debug().Err(err).Msg("func runtime received done notification")
	case err = <-ee:
		log.Debug().Err(err).Msg("func runtime received start error notification")
	}
	return
}

// Stop serving
func (s *Service) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), ServerShutdownTimeout)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Warn().Err(err).Msg("error during shutdown")
	}
	ctx, cancel = context.WithTimeout(context.Background(), InstanceStopTimeout)
	defer cancel()

	if i, ok := s.f.(Stopper); ok {
		s.done <- i.Stop(ctx)
	} else {
		s.done <- nil
	}
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
			e := fmt.Sprintf("error determinging liveness.  %v\n", err)
			fmt.Fprintf(os.Stderr, e)
			w.WriteHeader(500)
			w.Write([]byte(e))
			return
		}
		if !alive {
			w.WriteHeader(503)
			w.Write([]byte("Function not live"))
			return
		}
	}
	fmt.Fprintf(w, "ALIVE")
}

func (s *Service) handleRequests() {
	go func() {
		if err := s.Server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("http server exited with unexpected error: %v", err)
			s.done <- err
		}
	}()
	log.Printf("Listening on port %v", port())
}

func (s *Service) handleSignals() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs)
	go func() {
		for {
			sig := <-sigs
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				log.Printf("Signal '%v' received. Stopping.", sig)
				s.Stop()
			} else {
				log.Printf("Signal '%v' ignored.", sig)
			}
		}
	}()
}

func port() (p string) {
	if os.Getenv("PORT") == "" {
		return DefaultServicePort
	}
	return os.Getenv("PORT")
}

func allEnvs() (envs map[string]string) {
	envs = make(map[string]string, len(os.Environ()))
	for _, e := range os.Environ() {
		envs[e] = os.Getenv(e)
	}
	return
}
