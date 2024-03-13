package http

import (
	"context"
	"net/http"
)

// Handler is a function instance which can handle a request.
//
// This is of course specific to Go functions, with other languages using types
// of their own (see language-specific runtime middleware), but the conceptual
// framework is the same.
type Handler interface {
	// Handle a request.
	Handle(http.ResponseWriter, *http.Request)
}

type HandleFunc func(http.ResponseWriter, *http.Request)

// Starter is an instance which has defined the Start hook
type Starter interface {
	// Start instance event hook.
	Start(context.Context, map[string]string) error
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

// DefaultHandler is used for simple static function implementations which
// need only define a single exported function named Handle of type HandleFunc.
type DefaultHandler struct {
	Handler HandleFunc
}

func (f DefaultHandler) Handle(w http.ResponseWriter, r *http.Request) {
	f.Handler(w, r)
}
