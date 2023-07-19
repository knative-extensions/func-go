// Package http implements a Functions HTTP middleware for use by
// scaffolding which exposes a function as a network service which handles
// http requests.
package http

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	DefaultServicePort    = "8080"
	ServerShutdownTimeout = 30 * time.Second
	InstanceStopTimeout   = 30 * time.Second
)

// Start an intance using a new Service
func Start(i Instance) error {
	return New(i).Start()
}

// Service exposes a Function Instance as a an HTTP service.
type Service struct {
	http.Server
	f    Instance
	done chan error
}

// New Service which service the given instance.
func New(f Instance) *Service {
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

// Start serving
func (s *Service) Start() (err error) {
	if err = s.f.Start(allEnvs()); err != nil {
		return
	}
	s.handleRequests()
	s.handleSignals()
	return <-s.done
}

// Stop serving
func (s *Service) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), ServerShutdownTimeout)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Printf("warning: error during shutdown. %s", err)
	}
	ctx, _ = context.WithTimeout(context.Background(), InstanceStopTimeout)
	s.done <- s.f.Stop(ctx)
}

// Handle requests for the instance
func (s *Service) Handle(res http.ResponseWriter, req *http.Request) {
	log.Println("Service.Handle")
	s.f.Handle(req.Context(), res, req)
}

// Ready handles readiness checks.
func (s *Service) Ready(res http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(res, "OK")
}

// Alive handles liveness checks.
func (s *Service) Alive(res http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(res, "OK")
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
				log.Printf("Signal %v received. Stopping.", sig)
				s.Stop()
			} else {
				log.Printf("Signal %v ignored.", sig)
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
