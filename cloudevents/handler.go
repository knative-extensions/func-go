package cloudevents

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/cloudevents/sdk-go/v2/protocol"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/rs/zerolog/log"
)

// ceHandler is a concurrent http.Handler that decodes each request into a
// CloudEvent, invokes the user function, and writes the response inline.
//
// It intentionally replaces the CloudEvents SDK's NewHTTPReceiveHandler. That
// receiver routes every request through a single unbuffered, process-shared
// channel (Protocol.incoming) using a non-cancellable send in
// Protocol.ServeHTTP. When a client cancels a request mid-flight — for example
// a per-attempt delivery timeout from the standalone Kafka runtime — the SDK's
// per-request receiver goroutine exits via ctx.Done() WITHOUT draining the
// channel, orphaning a concurrent request's queued send. Under sustained
// concurrent delivery with occasional cancellations those orphaned sends
// accumulate and the endpoint progressively stalls: healthy requests block for
// seconds to their full deadline even though the function and network are fine.
//
// Decoding and dispatching inline, with no shared channel and no cross-request
// handoff, makes every request fully independent and cancellation-safe.
type ceHandler struct {
	fn *receiverFn
}

func (h *ceHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Match the SDK receiver: it rejects the GET-style methods with 405 before
	// attempting to decode a CloudEvent (its OPTIONS/GET/DELETE handler hooks are
	// unset on this handler). Without this filter these methods would fall
	// through and invoke a no-argument function, silently changing the HTTP
	// contract the SDK receiver enforced.
	switch req.Method {
	case http.MethodOptions, http.MethodGet, http.MethodDelete:
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	msg := cehttp.NewMessageFromHttpRequest(req)
	if msg == nil {
		http.Error(rw, "could not read message from request", http.StatusBadRequest)
		return
	}
	defer func() { _ = msg.Finish(nil) }()

	e, decodeErr := binding.ToEvent(ctx, msg)
	var validateErr error
	if decodeErr == nil && e != nil {
		validateErr = e.Validate()
	}

	// Derive the outcome exactly as the SDK's receive invoker does, so the HTTP
	// status contract stays identical to NewHTTPReceiveHandler. This matters
	// because #189 tracks reverting to the SDK receiver: the revert must not flip
	// any status codes. A malformed event the function needs, or an invalid
	// event, becomes a NACK receipt whose status is derived below (mirroring
	// respFn(ctx, nil, NewReceipt(false, ...))); otherwise the user function runs
	// under panic recovery. A decode failure for a function that takes no event
	// argument is ignored — that function is invoked regardless.
	var resp *event.Event
	var result protocol.Result
	switch {
	case decodeErr != nil && h.fn.hasEventIn:
		result = protocol.NewReceipt(false, "failed to convert request to event: %w", decodeErr)
	case validateErr != nil:
		result = protocol.NewReceipt(false, "validation error in incoming event: %w", validateErr)
	default:
		// A panic becomes a non-ACK error result (mapped to 500 below) and is
		// logged, rather than escaping to net/http — which would drop the
		// connection without a response and lose the logged error.
		func() {
			defer func() {
				if r := recover(); r != nil {
					result = fmt.Errorf("call to receiver function has panicked: %v", r)
					log.Error().Interface("panic", r).Msg("cloudevent receiver function panicked")
				}
			}()
			resp, result = h.fn.invoke(ctx, e)
		}()
	}

	// Map the outcome to an HTTP status and body, mirroring the SDK's ResponseFn
	// exactly: a nil result and an ACK both mean success (200); an explicit HTTP
	// result carries its own code and message body; a validation error is 400
	// with the message as a text/plain body; an unknown encoding is 415; any
	// other non-ACK error is 500 with no body.
	status := http.StatusOK
	var errMsg string
	textPlain := false
	if result != nil {
		var httpResult *cehttp.Result
		switch {
		case cloudevents.ResultAs(result, &httpResult):
			if httpResult.StatusCode > 100 && httpResult.StatusCode < 600 {
				status = httpResult.StatusCode
			}
			errMsg = fmt.Errorf(httpResult.Format, httpResult.Args...).Error()
		case !protocol.IsACK(result):
			validationErr := event.ValidationError{}
			switch {
			case errors.As(result, &validationErr):
				status = http.StatusBadRequest
				errMsg = validationErr.Error()
				textPlain = true
			case errors.Is(result, binding.ErrUnknownEncoding):
				status = http.StatusUnsupportedMediaType
			default:
				status = http.StatusInternalServerError
			}
		}
	}

	// resp is non-nil only when the user function ran and returned an event; the
	// decode/validation NACK paths above leave it nil. So a response event and a
	// validation-error body are mutually exclusive here, and this ordering
	// matches the SDK's ResponseFn (which writes the validation-error text and
	// returns before its respMsg check).
	if resp != nil {
		if werr := cehttp.WriteResponseWriter(ctx, (*binding.EventMessage)(resp), status, rw); werr != nil {
			log.Error().Err(werr).Msg("failed to write cloudevent response")
		}
		return
	}
	if textPlain {
		rw.Header().Set("content-type", "text/plain")
	}
	rw.WriteHeader(status)
	// Preserve the SDK behavior of writing the result's message as the response
	// body (HTTP results and validation errors) so clients still see the text.
	if errMsg != "" {
		if _, werr := rw.Write([]byte(errMsg)); werr != nil {
			log.Error().Err(werr).Msg("failed to write cloudevent error response body")
		}
	}
}

// receiverFn validates and invokes a user function of one of the CloudEvents
// SDK's supported signatures. It is a faithful port of the SDK's internal
// (unexported) client.receiverFn, reproduced here so the function can be
// dispatched directly from a plain, concurrent http.Handler rather than through
// the SDK's channel-based receiver. See ceHandler for why. Supported signatures:
//
//	func()
//	func() protocol.Result
//	func(context.Context)
//	func(context.Context) protocol.Result
//	func(event.Event)
//	func(event.Event) protocol.Result
//	func(context.Context, event.Event)
//	func(context.Context, event.Event) protocol.Result
//	func(event.Event) *event.Event
//	func(event.Event) (*event.Event, protocol.Result)
//	func(context.Context, event.Event) *event.Event
//	func(context.Context, event.Event) (*event.Event, protocol.Result)
type receiverFn struct {
	numIn  int
	numOut int

	fnValue reflect.Value

	hasContextIn bool
	hasEventIn   bool

	hasEventOut  bool
	hasResultOut bool
}

const (
	inParamUsage  = "expected a function taking either no parameters, one or more of (context.Context, event.Event) ordered"
	outParamUsage = "expected a function returning one or more of (*event.Event, protocol.Result) ordered"
)

var (
	contextType  = reflect.TypeOf((*context.Context)(nil)).Elem()
	eventType    = reflect.TypeOf((*event.Event)(nil)).Elem()
	eventPtrType = reflect.TypeOf((*event.Event)(nil))
	resultType   = reflect.TypeOf((*protocol.Result)(nil)).Elem()
)

func newReceiverFn(fn any) (*receiverFn, error) {
	fnType := reflect.TypeOf(fn)
	if fnType == nil || fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("must pass a function to handle events")
	}
	r := &receiverFn{
		fnValue: reflect.ValueOf(fn),
		numIn:   fnType.NumIn(),
		numOut:  fnType.NumOut(),
	}
	if err := r.validate(fnType); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *receiverFn) invoke(ctx context.Context, e *event.Event) (*event.Event, protocol.Result) {
	args := make([]reflect.Value, 0, r.numIn)
	if r.numIn > 0 {
		if r.hasContextIn {
			args = append(args, reflect.ValueOf(ctx))
		}
		if r.hasEventIn {
			args = append(args, reflect.ValueOf(*e))
		}
	}
	v := r.fnValue.Call(args)

	var respOut protocol.Result
	var eOut *event.Event
	if r.numOut > 0 {
		i := 0
		if r.hasEventOut {
			if eo, ok := v[i].Interface().(*event.Event); ok {
				eOut = eo
			}
			i++
		}
		if r.hasResultOut {
			if resp, ok := v[i].Interface().(protocol.Result); ok {
				respOut = resp
			}
		}
	}
	return eOut, respOut
}

func (r *receiverFn) validate(fnType reflect.Type) error {
	if err := r.validateInParamSignature(fnType); err != nil {
		return err
	}
	return r.validateOutParamSignature(fnType)
}

// validateInParamSignature verifies the inputs are [0, all] of
// (context.Context, event.Event) in that order.
func (r *receiverFn) validateInParamSignature(fnType reflect.Type) error {
	r.hasContextIn = false
	r.hasEventIn = false

	switch fnType.NumIn() {
	case 2:
		if !eventType.ConvertibleTo(fnType.In(1)) {
			return fmt.Errorf("%s; cannot convert parameter 2 to %s from event.Event", inParamUsage, fnType.In(1))
		}
		r.hasEventIn = true
		fallthrough
	case 1:
		if !contextType.ConvertibleTo(fnType.In(0)) {
			if !eventType.ConvertibleTo(fnType.In(0)) {
				return fmt.Errorf("%s; cannot convert parameter 1 to %s from context.Context or event.Event", inParamUsage, fnType.In(0))
			} else if r.hasEventIn {
				return fmt.Errorf("%s; duplicate parameter of type event.Event", inParamUsage)
			} else {
				r.hasEventIn = true
			}
		} else {
			r.hasContextIn = true
		}
		fallthrough
	case 0:
		return nil
	default:
		return fmt.Errorf("%s; function has too many parameters (%d)", inParamUsage, fnType.NumIn())
	}
}

// validateOutParamSignature verifies the outputs are [0, all] of
// (*event.Event, protocol.Result) in that order.
func (r *receiverFn) validateOutParamSignature(fnType reflect.Type) error {
	r.hasEventOut = false
	r.hasResultOut = false

	switch fnType.NumOut() {
	case 2:
		if !fnType.Out(1).ConvertibleTo(resultType) {
			return fmt.Errorf("%s; cannot convert parameter 2 from %s to event.Response", outParamUsage, fnType.Out(1))
		}
		r.hasResultOut = true
		fallthrough
	case 1:
		if !fnType.Out(0).ConvertibleTo(resultType) {
			if !fnType.Out(0).ConvertibleTo(eventPtrType) {
				return fmt.Errorf("%s; cannot convert parameter 1 from %s to *event.Event or transport.Result", outParamUsage, fnType.Out(0))
			}
			r.hasEventOut = true
		} else if r.hasResultOut {
			return fmt.Errorf("%s; duplicate parameter of type event.Response", outParamUsage)
		} else {
			r.hasResultOut = true
		}
		fallthrough
	case 0:
		return nil
	default:
		return fmt.Errorf("%s; function has too many return types (%d)", outParamUsage, fnType.NumOut())
	}
}
