package http

import (
	"context"
	"log"
	"net/http"
)

// Instance of a Function as it exists during the runtime (for Go functions.)
//
// Other languages use types of their own lange (see language-specific
// runtime middleware).
type Instance interface {
	// Start instance event hook.
	Start(map[string]string) error

	// Handle a request.
	Handle(context.Context, http.ResponseWriter, *http.Request)

	// Stop instance event hook.
	Stop(context.Context) error
}

// HandleFunc defines the function signature expected of static Functions.
type HandleFunc func(context.Context, http.ResponseWriter, *http.Request)

// DefaultInstance is used for simple static function implementations which
// need only define a single exported function named Handle of type HandleFunc.
type DefaultInstance struct {
	Handler HandleFunc
}

// Start a default instance.
func (f DefaultInstance) Start(_ map[string]string) error {
	return nil
}

// Handle a request by passing to the handler function.
func (f DefaultInstance) Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	if f.Handle == nil {
		f.Handler = defaultHandler
	}
	f.Handler(ctx, res, req)
}

// Stop a default instance.
func (f DefaultInstance) Stop(_ context.Context) error {
	return nil
}

// DefaultHandler is a Handler implementation which simply warns the user that
// the default handler instance was not properly initialized.
var defaultHandler = func(_ context.Context, res http.ResponseWriter, req *http.Request) {
	log.Printf("Handler was not instantiated with a handle function.")
	http.NotFound(res, req)
}
