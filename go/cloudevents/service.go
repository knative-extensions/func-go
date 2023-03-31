// Package ce implements a Functions CloudEvent middleware for use by
// scaffolding which exposes a function as a network service which handles
// Cloud Events.
package cloudevents

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const (
	DefaultServicePort    = "8080"
	ServerShutdownTimeout = 30 * time.Second
	InstanceStopTimeout   = 30 * time.Second
)

// Start an intance using a new Service
func Start(i Handler) error {
	return New(i).Start(context.Background())
}

// Service exposes a Function Instance as a an HTTP service.
type Service struct {
	http.Server
	f    Handler
	done chan error
}

// New Service which service the given instance.
func New(f Handler) *Service {
	svc := &Service{
		f:    f,
		done: make(chan error),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health/readiness", svc.Ready)
	mux.HandleFunc("/health/liveness", svc.Alive)
	svc.Server.Handler = mux
	return svc
}

// Start serving
func (s *Service) Start(ctx context.Context) (err error) {
	protocol, err := cloudevents.NewHTTP()
	if err != nil {
		return
	}
	cloudeventReceiver, err := cloudevents.NewHTTPReceiveHandler(ctx, protocol, s.f.Handler())
	if err != nil {
		return
	}
	s.Server.Handler.(*http.ServeMux).Handle("/", cloudeventReceiver)
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

	ctx, cancel = context.WithTimeout(context.Background(), InstanceStopTimeout)
	defer cancel()
	if i, ok := s.f.(Stopper); ok {
		s.done <- i.Stop(ctx)
	} else {
		s.done <- nil
	}
}

// Ready handles readiness checks.
func (s *Service) Ready(res http.ResponseWriter, req *http.Request) {
	if i, ok := s.f.(ReadinessReporter); ok {
		ready, err := i.Ready(req.Context())
		if err != nil {
			e := fmt.Sprintf("error determinging readiness.  %v\n", err)
			fmt.Fprintf(os.Stderr, e)
			res.WriteHeader(500)
			res.Write([]byte(e))
			return
		}
		if !ready {
			res.WriteHeader(503)
			res.Write([]byte("Function not yet available"))
			return
		}
	}
	fmt.Fprintf(res, "READY")
}

// Alive handles liveness checks.
func (s *Service) Alive(res http.ResponseWriter, req *http.Request) {
	if i, ok := s.f.(LivenessReporter); ok {
		alive, err := i.Alive(req.Context())
		if err != nil {
			e := fmt.Sprintf("error determinging liveness.  %v\n", err)
			fmt.Fprintf(os.Stderr, e)
			res.WriteHeader(500)
			res.Write([]byte(e))
			return
		}
		if !alive {
			res.WriteHeader(503)
			res.Write([]byte("Function not live"))
			return
		}
	}
	fmt.Fprintf(res, "ALIVE")
}

func (s *Service) handleRequests() {
	// TODO
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
