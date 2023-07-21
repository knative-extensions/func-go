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
	"reflect"
	"runtime"
	"strconv"
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
	i    Handler
	done chan error
}

// New Service which service the given instance.
func New(i Handler) *Service {
	svc := &Service{
		i:    i,
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
	mux.Handle("/", newCloudeventHandler(i)) // See implementation note
	svc.Server.Handler = mux
	return svc
}

// Start serving
func (s *Service) Start(ctx context.Context) (err error) {
	if i, ok := s.i.(Starter); ok {
		if err = i.Start(ctx, allEnvs()); err != nil {
			return
		}
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

	ctx, cancel = context.WithTimeout(context.Background(), InstanceStopTimeout)
	defer cancel()

	if i, ok := s.i.(Stopper); ok {
		s.done <- i.Stop(ctx)
	} else {
		s.done <- nil
	}
}

type MyFunction struct{}

func NewF() *MyFunction {
	return &MyFunction{}
}

func (f *MyFunction) Handle() {
	fmt.Println("Instanced CE Handle called")
}

func debug() any {
	fmt.Println("DEBUG")
	f := NewF()
	h := f.Handle

	// m := reflect.MethodByName("Handle")

	fmt.Printf("typeof h=%v\n", reflect.TypeOf(h))
	fmt.Printf("kindof h=%v\n", reflect.TypeOf(h).Kind())
	fmt.Println("/DEBUG")
	return f.Handle
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
func (s *Service) Ready(res http.ResponseWriter, req *http.Request) {
	if i, ok := s.i.(ReadinessReporter); ok {
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
	if i, ok := s.i.(LivenessReporter); ok {
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
			} else if runtime.GOOS == "linux" && sig == syscall.Signal(0x17) {
				// Ignore SIGURG; signal 23 (0x17)
				// See https://go.googlesource.com/proposal/+/master/design/24543-non-cooperative-preemption.md
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

// CE-specific helpers

func panicOn(err error) {
	if err != nil {
		panic(err)
	}
}
