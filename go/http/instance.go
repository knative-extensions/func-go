package http

import (
	"context"
	"log"
	"net/http"
)

// Handler is a function instance which can handle a request.
//
// This is of course specific to Go functions, with other languages using types
// of their own (see language-specific runtime middleware), but the conceptual
// framework is the same.
type Handler interface {
	// Handle a request.
	Handle(context.Context, http.ResponseWriter, *http.Request)
}

// Starter is an instance which has defined the Start hook
type Starter interface {
	// Start instance event hook.
	Start(map[string]string) error
}

// Stopper is an instance which has defined the  Stop hook
type Stopper interface {
	// Stop instance event hook.
	Stop(context.Context) error
}

// ReadinessReporter is an instance which reports its readiness.
type ReadinessReporter interface {
	// Ready to be invoked or not.
	Ready(context.Context) (bool, error)
}

// LivenessReporter is an instance which reports it is alive.
type LivenessReporter interface {
	// Alive allows the instance to report it's liveness status.
	Alive(context.Context) (bool, error)
}

// HandleFunc defines the function signature expected of static Functions.
type HandleFunc func(context.Context, http.ResponseWriter, *http.Request)

// DefaultHandler is used for simple static function implementations which
// need only define a single exported function named Handle of type HandleFunc.
type DefaultHandler struct {
	Handler HandleFunc
}

// Handle a request by passing to the handler function.
func (f DefaultHandler) Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	if f.Handle == nil {
		f.Handler = defaultHandler
	}
	f.Handler(ctx, res, req)
}

// DefaultHandler is a Handler implementation which simply warns the user that
// the default handler instance was not properly initialized.
var defaultHandler = func(_ context.Context, res http.ResponseWriter, req *http.Request) {
	log.Printf("Handler was not instantiated with a handle function.")
	http.NotFound(res, req)
}
