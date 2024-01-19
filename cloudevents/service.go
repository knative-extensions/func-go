// Package ce implements a Functions CloudEvent middleware for use by
// scaffolding which exposes a function as a network service which handles
// Cloud Events.
package cloudevents

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/rs/zerolog/log"
)

const (
	DefaultLogLevel       = LogDebug
	DefaultServicePort    = "8080"
	ServerShutdownTimeout = 30 * time.Second
	InstanceStopTimeout   = 30 * time.Second
)

// Start an intance using a new Service
func Start(i Handler) error {
	log.Debug().Msg("func runtime creating function instance")
	return New(i).Start(context.Background())
}

// Service exposes a Function Instance as a an HTTP service.
type Service struct {
	http.Server
	i    Handler
	stop chan error
}

// New Service which service the given instance.
func New(i Handler) *Service {
	svc := &Service{
		i:    i,
		stop: make(chan error),
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
	mux.Handle("/", newCloudeventHandler(i)) // See implementation note
	svc.Server.Handler = mux
	return svc
}

// Start serving
func (s *Service) Start(ctx context.Context) (err error) {
	// Start
	// Starts the function instance in a separate routine, sending any
	// runtime errors on s.stop.
	if err = s.startInstance(ctx); err != nil {
		return
	}

	// Listen and serve
	go func() {
		if err := s.Server.ListenAndServe(); err != http.ErrServerClosed {
			log.Error().Err(err).Msg("http server exited with unexpected error")
			s.stop <- err
		}
	}()
	log.Debug().Str("port", port()).Msg("Listening on port")

	s.handleSignals()
	return <-s.stop
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

	if i, ok := s.i.(Stopper); ok {
		s.stop <- i.Stop(ctx)
	} else {
		s.stop <- nil
	}
}

// NOTE: no Handle on service because of the need to decorate the handler
// at runtime to adapt to the cloudevents sdk's expectation of a polymorphic
// handle method. So instead of a 'func (s *Service) Handle..' we have:
//
// TODO: test when f is not a pointer
// TODO: test when f.Handle does not have a pointer receiver
// TODO: test when f is an interface type
func newCloudeventHandler(f any) http.Handler {
	var h any
	if dh, ok := f.(DefaultHandler); ok {
		// Static Functions use a struct to curry the reference
		h = dh.Handler
	} else {
		// Instanced Functions implement one of the defined interfaces.
		h = getReceiverFn(f)
	}

	port, err := strconv.Atoi(port())
	panicOn(err)
	protocol, err := cloudevents.NewHTTP(
		cloudevents.WithPort(port),
		cloudevents.WithPath("/"),
	)
	panicOn(err)
	ctx := context.Background() // ctx is not used by NewHTTPReceiveHandler
	cloudeventReceiver, err := cloudevents.NewHTTPReceiveHandler(ctx, protocol, h)
	panicOn(err)
	return cloudeventReceiver
}

// Ready handles readiness checks.
func (s *Service) Ready(w http.ResponseWriter, r *http.Request) {
	if i, ok := s.i.(ReadinessReporter); ok {
		ready, err := i.Ready(r.Context())
		if err != nil {
			message := "error checking readiness"
			log.Debug().Err(err).Msg(message)
			w.WriteHeader(500)
			_, _ = w.Write([]byte(message + ". " + err.Error()))
			return
		}
		if !ready {
			message := "function not yet available"
			log.Debug().Msg(message)
			w.WriteHeader(503)
			fmt.Fprintln(w, message)
			return
		}
	}
	fmt.Fprintf(w, "READY")
}

// Alive handles liveness checks.
func (s *Service) Alive(w http.ResponseWriter, r *http.Request) {
	if i, ok := s.i.(LivenessReporter); ok {
		alive, err := i.Alive(r.Context())
		if err != nil {
			message := "error checking liveness"
			log.Err(err).Msg(message)
			w.WriteHeader(500)
			_, _ = w.Write([]byte(message + ". " + err.Error()))
			return
		}
		if !alive {
			message := "function not ready"
			log.Debug().Msg(message)
			w.WriteHeader(503)
			_, _ = w.Write([]byte(message))
			return
		}
	}
	fmt.Fprintf(w, "ALIVE")
}

func (s *Service) startInstance(ctx context.Context) error {
	if i, ok := s.i.(Starter); ok {
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
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs)
	go func() {
		for {
			sig := <-sigs
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				log.Debug().Any("signal", sig).Msg("signal received")
				s.Stop()
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

func port() (p string) {
	if os.Getenv("PORT") == "" {
		return DefaultServicePort
	}
	return os.Getenv("PORT")
}

// CE-specific helpers
func panicOn(err error) {
	if err != nil {
		panic(err)
	}
}
