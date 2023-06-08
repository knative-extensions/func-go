package http

import (
	"context"
	"testing"
	"time"

	"github.com/lkingland/func-runtimes/go/http/mock"
)

// TestStart ensures that the Start method of a function is invoked
// if it is implemented by the function instance.
func TestStart(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		onStart     = func(_ context.Context, _ map[string]string) error {
			startCh <- true
			return nil
		}
	)
	defer cancel()

	f := &mock.Function{OnStart: onStart}

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
