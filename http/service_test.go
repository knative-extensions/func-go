package http

import (
	"context"
	"testing"
	"time"

	"github.com/lkingland/func-runtime-go/http/mock"
)

// TestStart ensures that the Start method of a function is invoked
// if it is implemented by the function instance.
func TestStart(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		stopCh      = make(chan any)
		onStart     = func(_ context.Context, _ map[string]string) error {
			startCh <- true
			return nil
		}
		onStop = func(_ context.Context) error {
			stopCh <- true
			return nil
		}
	)
	defer cancel()

	f := &mock.Function{OnStart: onStart, OnStop: onStop}

	go func() {
		if err := New(f).Start(ctx); err != nil {
			t.Fatal(err)
		}
	}()

	select {
	case <-time.After(500 * time.Millisecond):
		t.Fatal("function failed to notify of start")
	case <-startCh:
		t.Log("start signal received")
	}
	t.Log("waiting for stop channel to return")
	cancel()
	<-stopCh
}

// TestCfg ensures that pertinent environmental state for the process is
// communicated to the Function instance via the config map on start.
func TestCfg(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		onStart     = func(_ context.Context, cfg map[string]string) error {
			v := cfg["TEST_ENV"]
			if v != "example_value" {
				t.Fatalf("did not receive TEST_ENV.  got %v", cfg["TEST_ENV"])
			} else {
				t.Log("expected value received")
			}
			startCh <- true
			return nil
		}
	)
	defer cancel()

	f := &mock.Function{OnStart: onStart}

	t.Setenv("TEST_ENV", "example_value")

	go func() {
		if err := New(f).Start(ctx); err != nil {
			t.Fatal(err)
		}
	}()

	select {
	case <-time.After(500 * time.Millisecond):
		t.Fatal("function failed to notify of start")
	case <-startCh:
		t.Log("start signal received")
	}

}
