package kafka

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/rs/zerolog/log"

	ce "knative.dev/func-go/cloudevents"
)

// Starter is a function which defines a method to be called on function start.
type Starter interface {
	Start(context.Context, map[string]string) error
}

// Stopper is a function which defines a method to be called on function stop.
type Stopper interface {
	Stop(context.Context) error
}

// ReadinessReporter is a function which defines a method to be used to
// determine readiness.
type ReadinessReporter interface {
	Ready(context.Context) (bool, error)
}

// LivenessReporter is a function which defines a method to be used to
// determine liveness.
type LivenessReporter interface {
	Alive(context.Context) (bool, error)
}

var responseEventOnce sync.Once

func validateHandler(f any) (err error) {
	var fn any
	switch dh := f.(type) {
	case ce.DefaultHandler:
		fn = dh.Handler
	case *ce.DefaultHandler:
		fn = dh.Handler
	default:
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("handler function does not match any supported CloudEvents signature")
				}
			}()
			fn = ce.GetReceiverFn(f)
		}()
		if err != nil {
			return
		}
	}
	if fn == nil {
		return fmt.Errorf("handler function is nil")
	}
	switch fn.(type) {
	case func(),
		func() error,
		func(context.Context),
		func(context.Context) error,
		func(event.Event),
		func(event.Event) error,
		func(context.Context, event.Event),
		func(context.Context, event.Event) error,
		func(event.Event) *event.Event,
		func(event.Event) (*event.Event, error),
		func(context.Context, event.Event) *event.Event,
		func(context.Context, event.Event) (*event.Event, error):
		return nil
	default:
		return fmt.Errorf("handler function does not match any supported CloudEvents signature")
	}
}

// invokeHandler calls a CloudEvents handler directly with a constructed event.
func invokeHandler(ctx context.Context, f any, e event.Event) error {
	var fn any
	switch dh := f.(type) {
	case ce.DefaultHandler:
		fn = dh.Handler
	case *ce.DefaultHandler:
		fn = dh.Handler
	default:
		fn = ce.GetReceiverFn(f)
	}
	return invokeHandlerFn(ctx, fn, e)
}

func invokeHandlerFn(ctx context.Context, fn any, e event.Event) error {
	switch h := fn.(type) {
	case func():
		h()
		return nil
	case func() error:
		return h()
	case func(context.Context):
		h(ctx)
		return nil
	case func(context.Context) error:
		return h(ctx)
	case func(event.Event):
		h(e)
		return nil
	case func(event.Event) error:
		return h(e)
	case func(context.Context, event.Event):
		h(ctx, e)
		return nil
	case func(context.Context, event.Event) error:
		return h(ctx, e)
	case func(event.Event) *event.Event:
		resp := h(e)
		warnResponseEvent(resp)
		return nil
	case func(event.Event) (*event.Event, error):
		resp, err := h(e)
		warnResponseEvent(resp)
		return err
	case func(context.Context, event.Event) *event.Event:
		resp := h(ctx, e)
		warnResponseEvent(resp)
		return nil
	case func(context.Context, event.Event) (*event.Event, error):
		resp, err := h(ctx, e)
		warnResponseEvent(resp)
		return err
	default:
		panic("handler function does not match any supported CloudEvents signature")
	}
}

func warnResponseEvent(resp *event.Event) {
	if resp != nil {
		responseEventOnce.Do(func() {
			log.Warn().Msg("handler returned a response event, but response events are ignored when consuming from Kafka")
		})
	}
}
