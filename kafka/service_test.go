package kafka

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// readyHandler implements ReadinessReporter.
type readyHandler struct {
	readyVal bool
	readyErr error
}

func (h *readyHandler) Handle() {}
func (h *readyHandler) Ready(_ context.Context) (bool, error) {
	return h.readyVal, h.readyErr
}

// aliveHandler implements LivenessReporter.
type aliveHandler struct {
	aliveVal bool
	aliveErr error
}

func (h *aliveHandler) Handle() {}
func (h *aliveHandler) Alive(_ context.Context) (bool, error) {
	return h.aliveVal, h.aliveErr
}

// plainHandler implements neither ReadinessReporter nor LivenessReporter.
type plainHandler struct{}

func (h *plainHandler) Handle() {}

func TestReady_NotReady(t *testing.T) {
	svc := New(&plainHandler{})
	// ready defaults to false

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/readiness", nil)
	svc.Ready(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestReady_Ready(t *testing.T) {
	svc := New(&plainHandler{})
	svc.ready.Store(true)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/readiness", nil)
	svc.Ready(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if w.Body.String() != "READY" {
		t.Errorf("expected body %q, got %q", "READY", w.Body.String())
	}
}

func TestReady_UserReporterNotReady(t *testing.T) {
	h := &readyHandler{readyVal: false, readyErr: nil}
	svc := New(h)
	svc.ready.Store(true)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/readiness", nil)
	svc.Ready(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestReady_UserReporterError(t *testing.T) {
	h := &readyHandler{readyVal: false, readyErr: fmt.Errorf("db down")}
	svc := New(h)
	svc.ready.Store(true)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/readiness", nil)
	svc.Ready(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestAlive_Default(t *testing.T) {
	svc := New(&plainHandler{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/liveness", nil)
	svc.Alive(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if w.Body.String() != "ALIVE" {
		t.Errorf("expected body %q, got %q", "ALIVE", w.Body.String())
	}
}

func TestAlive_NotAlive(t *testing.T) {
	h := &aliveHandler{aliveVal: false, aliveErr: nil}
	svc := New(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/liveness", nil)
	svc.Alive(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestAlive_Error(t *testing.T) {
	h := &aliveHandler{aliveVal: false, aliveErr: fmt.Errorf("check failed")}
	svc := New(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health/liveness", nil)
	svc.Alive(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}
