package cloudevents

import (
	"context"

	"github.com/cloudevents/sdk-go/v2/event"
)

// Handler is a CloudEvent function Handler, which is invoked when it
// receives a cloud event.  It must implement one of the following methods:
//
//	Handle()
//	Handle() error
//	Handle(context.Context)
//	Handle(context.Context) error
//	Handle(event.Event)
//	Handle(event.Event) error
//	Handle(context.Context, event.Event)
//	Handle(context.Context, event.Event) error
//	Handle(event.Event) *event.Event
//	Handle(event.Event) (*event.Event, error)
//	Handle(context.Context, event.Event) *event.Event
//	Handle(context.Context, event.Event) (*event.Event, error)
//
// It can optionaly implement any of Start, Stop, Ready, and Alive.
type Handler any

// Starter is a function which defines a method to be called on function start.
type Starter interface {
	// Start instance event hook.
	Start(context.Context, map[string]string) error
}

// Stopper is function which defines a method to be called on function stop.
type Stopper interface {
	// Stop instance event hook.
	Stop(context.Context) error
}

// ReadinessReporter is a function which defines a method to be used to
// determine readiness.
type ReadinessReporter interface {
	// Ready to be invoked or not.
	Ready(context.Context) (bool, error)
}

// LivenessReporter is a function which defines a method to be used to
// determine liveness.
type LivenessReporter interface {
	// Alive allows the instance to report it's liveness status.
	Alive(context.Context) (bool, error)
}

// DefaultHandler is used for simple static function implementations which
// need only define a single exported function named Handle which must be
// of a signature understood by the CloudEvents SDK.
type DefaultHandler struct {
	Handler any
}

// NOTE on the implementation of the various Handle methods:
//
// Since Go does not consider differing arguments and returns as indicating
// a new method signature, each of the possible implementations supported
// by the underlying CloudEvents SDK are enumerated as separate types.
//
// The cloudevents SDK wants us to pass a reference to a _function_ which is
// of one of its accepted method signatures.  It uses reflection to determine
// which signature has been provided and then invokes.
// We, however, want to simply pass a structure which implements a
// CloudEvent handler interface of a known and compile-time safe type.
//
// To bridge these two approaches, herein is created a new interface called
// a CloudeventsHandlerProducer which, as the name suggests, produces
// a CloudeventsHandler when requested.  This is of type "any" because
// the cloudevents SDK supports several signatures, so we can not validate
// this at compile-time without becoming coupled to the cloudevents SDK
// implementation.
// So here we accept any structure which exports a mamber named "Handle", and
// then leave it up to the cloudevents SDK to fail if the arguments and
// return variables are not one of the supported signature.
// It is important to the function developer's usage to be able to simply
// declare their function has a method Handle... without needing themselves
// to implement the ProduceHandler method, because that's ugly and confusing.
// The way this is accomplished is by decorating the passed instance in a
// struct which uses reflection to implement the ProduceHandler method.

// newReceiver satisfies the expectations of
// cloudevents.NewHTTPReceiveHandler's handler argument the hard way.
//
// First attempt: use a reference to the function which should handle
// the request via reflection.   This appears to fal because it becomes
// a function with first argument the receiver, rather than a method where
// that is implied, and thus fails the conversion to one of the CE
// library's known set of method signatures.
//
//			t := reflect.TypeOf(f)
//			m, ok := t.MethodByName("Handle")
//	   if !ok {
//	     panic("unable to find a 'Handle' method on the function.")
//	   }
//	   h = m.Func.Interface()
//
// Second attempt:  wrap the target in a closure with an entirely generic
// method signature and invoke it directly.  This fails for basically the
// same reason: the CE library checks and sees that []interface{} is not
// one of its known signatures.
//
//	h = func(args ...interface{}) {
//	  inputs := make([]reflect.Value, len(args))
//	  for i, _ := range args {
//	    inputs[i] = reflect.ValueOf(args[i])
//	  }
//	  reflect.ValueOf(f).MethodByName("Handle").Call(inputs)
//	}
//
// Third Attempt: just give the CE library _exactly_ the method
// signature it wants by enumerating them one at a time with the receiver
// in the resultant closure.
//
// TODO: Can probably simplify this at least one level:  from
// TODO: Should check the CloudEvents SDK to ensure passing a handling
// struct is not already supported some other way, and if not,
// contribute this to the codebase.  Also could check that the two earlier
// reflection-based attempts are not in error.
// TODO: One benefit of enumerating the types is that a user's function could
// be type-checked at compile time with a littl extra introspection.
type handler interface {
	Handle()
}
type handlerErr interface {
	Handle() error
}
type handlerCtx interface {
	Handle(context.Context)
}
type handlerCtxErr interface {
	Handle(context.Context) error
}
type handlerEvt interface {
	Handle(event.Event)
}
type handlerEvtErr interface {
	Handle(event.Event) error
}
type handlerCtxEvt interface {
	Handle(context.Context, event.Event)
}
type handlerCtxEvtErr interface {
	Handle(context.Context, event.Event) error
}
type handlerEvtEvt interface {
	Handle(event.Event) *event.Event
}
type handlerEvtEvtErr interface {
	Handle(event.Event) (*event.Event, error)
}
type handlerCtxEvtEvt interface {
	Handle(context.Context, event.Event) *event.Event
}
type handlerCtxEvtEvtErr interface {
	Handle(context.Context, event.Event) (*event.Event, error)
}

func getReceiverFn(f any) any {
	switch h := f.(type) {
	case handler:
		return h.Handle
	case handlerErr:
		return h.Handle
	case handlerCtx:
		return h.Handle
	case handlerCtxErr:
		return h.Handle
	case handlerEvt:
		return h.Handle
	case handlerEvtErr:
		return h.Handle
	case handlerCtxEvt:
		return h.Handle
	case handlerCtxEvtErr:
		return h.Handle
	case handlerEvtEvt:
		return h.Handle
	case handlerEvtEvtErr:
		return h.Handle
	case handlerCtxEvtEvt:
		return h.Handle
	case handlerCtxEvtEvtErr:
		return h.Handle
	default:
		panic("Please see the supported function signatures list.")
	}
}
