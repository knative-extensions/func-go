package cloudevents

import (
	"context"
	"knative.dev/func-go/cloudevents/mock"
	"testing"
	"time"
)

// TestStart_Invoked ensures that the Start method of a function is invoked
// if it is implemented by the function instance.
func TestStart_Invoked(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		errCh       = make(chan error)
		onStart     = func(_ context.Context, _ map[string]string) error {
			startCh <- true
			return nil
		}
	)
	defer cancel()

	f := &mock.Function{OnStart: onStart}

	go func() {
		if err := New(f).Start(ctx); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-time.After(500 * time.Millisecond):
		t.Fatal("function failed to notify of start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		t.Log("start signal received")
	}
	cancel()
}

// TestStart_Static checks that static method Start(f) is a convenience method
// for New(f).Start()
func TestStart_Static(t *testing.T) {
	var (
		startCh = make(chan any)
		errCh   = make(chan error)
		onStart = func(_ context.Context, _ map[string]string) error {
			startCh <- true
			return nil
		}
	)

	f := &mock.Function{OnStart: onStart}

	go func() {
		if err := Start(f); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-time.After(500 * time.Millisecond):
		t.Fatal("function failed to notify of start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		t.Log("start signal received")
	}

}
