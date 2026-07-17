package kafka

import (
	"context"
	"sync"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"

	ce "knative.dev/func-go/cloudevents"
)

func TestInvokeHandler_AllSignatures(t *testing.T) {
	tests := []struct {
		name string
		f    any
	}{
		{"Handle()", &hNoArgs{}},
		{"Handle() error", &hErr{}},
		{"Handle(ctx)", &hCtx{}},
		{"Handle(ctx) error", &hCtxErr{}},
		{"Handle(event)", &hEvt{}},
		{"Handle(event) error", &hEvtErr{}},
		{"Handle(ctx, event)", &hCtxEvt{}},
		{"Handle(ctx, event) error", &hCtxEvtErr{}},
		{"Handle(event) *event", &hEvtEvt{}},
		{"Handle(event) (*event, error)", &hEvtEvtErr{}},
		{"Handle(ctx, event) *event", &hCtxEvtEvt{}},
		{"Handle(ctx, event) (*event, error)", &hCtxEvtEvtErr{}},
	}

	e := event.New()
	e.SetType("test")
	e.SetSource("test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := invokeHandler(context.Background(), tt.f, e)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestInvokeHandler_DefaultHandler(t *testing.T) {
	called := false
	dh := ce.DefaultHandler{
		Handler: func(ctx context.Context, e event.Event) error {
			called = true
			return nil
		},
	}

	e := event.New()
	e.SetType("test")
	e.SetSource("test")

	err := invokeHandler(context.Background(), dh, e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("DefaultHandler's function was not called")
	}
}

func TestInvokeHandler_DefaultHandlerPointer(t *testing.T) {
	called := false
	dh := &ce.DefaultHandler{
		Handler: func(ctx context.Context, e event.Event) error {
			called = true
			return nil
		},
	}

	e := event.New()
	e.SetType("test")
	e.SetSource("test")

	err := invokeHandler(context.Background(), dh, e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("*DefaultHandler's function was not called")
	}
}

func TestValidateHandler_InvalidSignature(t *testing.T) {
	type badHandler struct{}
	err := validateHandler(&badHandler{})
	if err == nil {
		t.Fatal("expected error for handler without Handle method")
	}
}

func TestValidateHandler_NilDefaultHandler(t *testing.T) {
	dh := ce.DefaultHandler{Handler: nil}
	err := validateHandler(dh)
	if err == nil {
		t.Fatal("expected error for DefaultHandler with nil Handler")
	}
}

func TestInvokeHandler_ResponseEventIgnored(t *testing.T) {
	responseEventOnce = sync.Once{}

	resp := event.New()
	resp.SetType("response")
	resp.SetSource("test")

	f := &hCtxEvtEvtErrReturnsEvent{resp: &resp}

	e := event.New()
	e.SetType("test")
	e.SetSource("test")

	err := invokeHandler(context.Background(), f, e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Test handler types for all 12 supported CloudEvents signatures.
type hNoArgs struct{}

func (h *hNoArgs) Handle() {}

type hErr struct{}

func (h *hErr) Handle() error { return nil }

type hCtx struct{}

func (h *hCtx) Handle(context.Context) {}

type hCtxErr struct{}

func (h *hCtxErr) Handle(context.Context) error { return nil }

type hEvt struct{}

func (h *hEvt) Handle(event.Event) {}

type hEvtErr struct{}

func (h *hEvtErr) Handle(event.Event) error { return nil }

type hCtxEvt struct{}

func (h *hCtxEvt) Handle(context.Context, event.Event) {}

type hCtxEvtErr struct{}

func (h *hCtxEvtErr) Handle(context.Context, event.Event) error { return nil }

type hEvtEvt struct{}

func (h *hEvtEvt) Handle(event.Event) *event.Event { return nil }

type hEvtEvtErr struct{}

func (h *hEvtEvtErr) Handle(event.Event) (*event.Event, error) { return nil, nil }

type hCtxEvtEvt struct{}

func (h *hCtxEvtEvt) Handle(context.Context, event.Event) *event.Event { return nil }

type hCtxEvtEvtErr struct{}

func (h *hCtxEvtEvtErr) Handle(context.Context, event.Event) (*event.Event, error) { return nil, nil }

type hCtxEvtEvtErrReturnsEvent struct {
	resp *event.Event
}

func (h *hCtxEvtEvtErrReturnsEvent) Handle(_ context.Context, _ event.Event) (*event.Event, error) {
	return h.resp, nil
}
