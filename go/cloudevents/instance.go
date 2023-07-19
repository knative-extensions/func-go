package cloudevents

import (
	"context"
	"log"

	"github.com/cloudevents/sdk-go/v2/event"
)

// Handler returns is a struct which returns a reference to the target to
// send events.  This target must implement one of the method signatures
// supported by the CloudEvents SDK.
type Handler interface {
	Handler() any
}

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
// need only define a single exported function named Handle which must be
// of a signature understood by the CloudEvents SDK.
type DefaultHandler struct {
	EventReceiver any
}

// Handler is an accessor to the underlying event receiver which must implement
// one of the cloudevents SDK event receiver sgnatures.
func (f DefaultHandler) Handler() any {
	if f.EventReceiver == nil {
		f.EventReceiver = defaultEventReceiver
	}
	return f.EventReceiver
}

// defaultEventReceiver is a cloudevent handler implementation which simply
// warns that the default handler instance was not properly initialized.
var defaultEventReceiver = func(ctx context.Context, e event.Event) (*event.Event, error) {
	log.Printf("Handler was not instantiated with a handle function. Echoing received event.")
	return &e, nil
}
