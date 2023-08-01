package http

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/knative-sandbox/func-go/http/mock"
)

// TestStart ensures that the Start method of a function is invoked
// if it is implemented by the function instance.
func TestStart(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		stopCh      = make(chan any)
		errCh       = make(chan error)
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
	t.Log("waiting for stop channel to return")
	cancel()
	<-stopCh
}

// TestCfg_Envs ensures that the function's Start method receives a map
// containing all available environment variables as a parameter.
//
// All environment variables are stored in a map which becomes the
// single argument 'cfg' passed to the Function's Start method.  This ensures
// that Functions can run in any context and are not coupled to os environment
// variables.
func TestCfg_Envs(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		errCh       = make(chan error)
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

// TestCfg_Static ensures that additional static "environment variables"
// built into the container as cfg.  The format is one variable per line,
// [key]=[value].
//
// This file is used by `func` to build metadata about a function for use
// at runtime such as the function's version (if using git), the version of
// func used to scaffold the function, etc.
func TestCfg_Static(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		errCh       = make(chan error)
	)
	defer cancel()

	// Run test from within a temp dir
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Write an example `cfg` file
	if err := os.WriteFile("cfg", []byte(`FUNC_VERSION="v1.2.3"`), os.ModePerm); err != nil {
		t.Fatal(err)
	}

	// Function which verifies it received the value
	f := &mock.Function{OnStart: func(_ context.Context, cfg map[string]string) error {
		v := cfg["FUNC_VERSION"]
		if v != "v1.2.3" {
			t.Fatalf("FUNC_VERSION not received.  Expected 'v1.2.3', got '%v'",
				cfg["FUNC_VERSION"])

		} else {
			t.Log("expected value received")
		}
		startCh <- true
		return nil
	}}

	// Run the function
	go func() {
		if err := New(f).Start(ctx); err != nil {
			errCh <- err
		}
	}()

	// Wait for a signal the onStart indicatig the function executed
	select {
	case <-time.After(500 * time.Millisecond):
		t.Fatal("function failed to notify of start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		t.Log("start signal received")
	}
}
