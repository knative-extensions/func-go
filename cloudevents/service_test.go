package cloudevents

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"knative.dev/func-go/cloudevents/mock"
)

// TestStart_Invoked ensures that the Start method of a function is invoked
// if it is implemented by the function instance.
func TestStart_Invoked(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		errCh       = make(chan error)
		timeoutCh   = time.After(500 * time.Millisecond)
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
	case <-timeoutCh:
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
	t.Setenv("LISTEN_ADDRESS", "127.0.0.1:") // use an OS-chosen port
	var (
		startCh   = make(chan any)
		errCh     = make(chan error)
		timeoutCh = time.After(500 * time.Millisecond)
		onStart   = func(_ context.Context, _ map[string]string) error {
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
	case <-timeoutCh:
		t.Fatal("function failed to notify of start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		t.Log("start signal received")
	}
}

// TestStart_CfgEnvs ensures that the function's Start method receives a map
// containing all available environment variables as a parameter.
//
// All environment variables are stored in a map which becomes the
// single argument 'cfg' passed to the Function's Start method.  This ensures
// that Functions can run in any context and are not coupled to os environment
// variables.
func TestStart_CfgEnvs(t *testing.T) {
	t.Setenv("LISTEN_ADDRESS", "127.0.0.1:") // use an OS-chosen port
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		errCh       = make(chan error)
		timeoutCh   = time.After(500 * time.Millisecond)
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
	case <-timeoutCh:
		t.Fatal("function failed to notify of start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		t.Log("start signal received")
	}
}

// TestStart_CfgStatic ensures that additional static "environment variables"
// built into the container as cfg.  The format is one variable per line,
// [key]=[value].
//
// This file is used by `func` to build metadata about a function for use
// at runtime such as the function's version (if using git), the version of
// func used to scaffold the function, etc.
func TestCfg_Static(t *testing.T) {
	t.Setenv("LISTEN_ADDRESS", "127.0.0.1:") // use an OS-chosen port
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		errCh       = make(chan error)
		timeoutCh   = time.After(500 * time.Millisecond)
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
	case <-timeoutCh:
		t.Fatal("function failed to notify of start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		t.Log("start signal received")
	}
}

// TestStop_Invoked ensures the Stop method of a function is invoked on context
// cancellation if it is implemented by the function instance.
func TestStop_Invoked(t *testing.T) {
	t.Setenv("LISTEN_ADDRESS", "127.0.0.1:") // use an OS-chosen port
	var (
		ctx, cancel = context.WithCancel(context.Background())
		startCh     = make(chan any)
		stopCh      = make(chan any)
		errCh       = make(chan error)
		timeoutCh   = time.After(500 * time.Millisecond)
		onStart     = func(_ context.Context, _ map[string]string) error {
			startCh <- true
			return nil
		}
		onStop = func(_ context.Context) error {
			stopCh <- true
			return nil
		}
	)

	f := &mock.Function{OnStart: onStart, OnStop: onStop}

	go func() {
		if err := New(f).Start(ctx); err != nil {
			errCh <- err
		}
	}()

	// Wait for start, error starting or hang
	select {
	case <-timeoutCh:
		t.Fatal("function failed to notify of start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		t.Log("start signal received")
	}

	// Cancel the context (trigger a stop)
	cancel()

	// Wait for stop signal, error stopping, or hang
	select {
	case <-time.After(500 * time.Millisecond):
		t.Fatal("function failed to notify of stop")
	case err := <-errCh:
		t.Fatal(err)
	case <-stopCh:
		t.Log("stop signal received")
	}
}

// TestHandle_Invoked ensures the Handle method of a function is invoked on
// a successful http request.
func TestHandle_Invoked(t *testing.T) {
	t.Setenv("LISTEN_ADDRESS", "127.0.0.1:") // use an OS-chosen port

	var (
		ctx, cancel = context.WithCancel(context.Background())
		errCh       = make(chan error)
		startCh     = make(chan any)
		timeoutCh   = time.After(500 * time.Millisecond)
		onStart     = func(_ context.Context, _ map[string]string) error {
			startCh <- true
			return nil
		}
		onHandle = func(_ context.Context, event event.Event) (*event.Event, error) {
			fmt.Println("Instanced CloudEvents handler invoked")
			fmt.Println(event) // echo to local output
			return nil, nil    // echo to caller
		}
	)
	defer cancel()

	f := &mock.Function{OnStart: onStart, OnHandle: onHandle}
	service := New(f)

	go func() {
		if err := service.Start(ctx); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-timeoutCh:
		t.Fatal("function failed to start")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
	}

	t.Logf("Service address: %v\n", service.Addr())

	// Send a request:
	c, err := cloudevents.NewClientHTTP()
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}

	// Create an Event.
	event := cloudevents.NewEvent()
	event.SetSource("example/uri")
	event.SetType("example.type")
	event.SetData(cloudevents.ApplicationJSON, map[string]string{"hello": "world"})

	// Set a target.
	ctx = cloudevents.ContextWithTarget(context.Background(), "http://"+service.Addr().String())

	// Send that Event.
	if result := c.Send(ctx, event); cloudevents.IsUndelivered(result) {
		log.Fatalf("failed to send, %v", result)
	}
}

// TestReady_Invoked ensures the default Ready Handle method of a function is invoked on
// a successful http request.
func TestReady_Invoked(t *testing.T) {
	t.Setenv("LISTEN_ADDRESS", "127.0.0.1:") // use an OS-chosen port

	var (
		ctx, cancel = context.WithCancel(context.Background())
		errCh       = make(chan error)
		startCh     = make(chan any)
		timeoutCh   = time.After(500 * time.Millisecond)
		onStart     = func(_ context.Context, _ map[string]string) error {
			startCh <- true
			return nil
		}
	)
	defer cancel()

	f := &mock.Function{OnStart: onStart}
	service := New(f)
	go func() {
		if err := service.Start(ctx); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-timeoutCh:
		t.Fatal("Service timed out")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		// Service started successfully
	}

	t.Logf("Service address: %v\n", service.Addr())

	resp, err := http.Get("http://" + service.Addr().String() + "/health/readiness")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected http status code: %v", resp.StatusCode)
	}
}

// TestAlive_Invoked ensures the default Alive Handle method of a function is invoked on
// a successful http request.
func TestAlive_Invoked(t *testing.T) {
	t.Setenv("LISTEN_ADDRESS", "127.0.0.1:") // use an OS-chosen port

	var (
		ctx, cancel = context.WithCancel(context.Background())
		errCh       = make(chan error)
		startCh     = make(chan any)
		timeoutCh   = time.After(500 * time.Millisecond)
		onStart     = func(_ context.Context, _ map[string]string) error {
			startCh <- true
			return nil
		}
	)
	defer cancel()

	f := &mock.Function{OnStart: onStart}
	service := New(f)
	go func() {
		if err := service.Start(ctx); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-timeoutCh:
		t.Fatal("Service timed out")
	case err := <-errCh:
		t.Fatal(err)
	case <-startCh:
		// Service started successfully
	}

	t.Logf("Service address: %v\n", service.Addr())

	resp, err := http.Get("http://" + service.Addr().String() + "/health/liveness")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected http status code: %v", resp.StatusCode)
	}
}
