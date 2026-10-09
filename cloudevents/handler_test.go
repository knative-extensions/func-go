package cloudevents

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
)

// ceRequest builds a minimal valid binary-mode CloudEvent POST request.
func ceRequest(t *testing.T, method, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Ce-Specversion", "1.0")
	req.Header.Set("Ce-Id", "id")
	req.Header.Set("Ce-Source", "example/uri")
	req.Header.Set("Ce-Type", "example.type")
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestHandler_MethodNotAllowed verifies the handler rejects the GET-style
// methods with 405 before decoding, matching the SDK receiver instead of
// invoking a no-argument function for them.
func TestHandler_MethodNotAllowed(t *testing.T) {
	var invoked int64
	h := newCloudeventHandler(DefaultHandler{Handler: func() {
		atomic.AddInt64(&invoked, 1)
	}})
	srv := httptest.NewServer(h)
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodOptions, http.MethodDelete} {
		req := ceRequest(t, method, srv.URL)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: request failed: %v", method, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s: status = %d, want 405", method, resp.StatusCode)
		}
	}
	if n := atomic.LoadInt64(&invoked); n != 0 {
		t.Fatalf("function was invoked %d times for GET-style methods, want 0", n)
	}
}

// TestHandler_UnknownEncodingStatus verifies that a POST that is not a valid
// CloudEvent (no CE headers) delivered to an event-taking function yields 415,
// matching the SDK receiver — not a blanket 400.
func TestHandler_UnknownEncodingStatus(t *testing.T) {
	h := newCloudeventHandler(DefaultHandler{Handler: func(_ context.Context, _ event.Event) error {
		return nil
	}})
	srv := httptest.NewServer(h)
	defer srv.Close()

	// No Ce-* headers: neither binary nor structured CE encoding.
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte(`{"not":"a cloudevent"}`)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415 for an undecodable event", resp.StatusCode)
	}
}

// TestHandler_InvalidEventStatus verifies that a decodable but invalid event
// (missing the required "type" attribute) yields 400 with the validation
// message as the body, matching the SDK receiver.
func TestHandler_InvalidEventStatus(t *testing.T) {
	h := newCloudeventHandler(DefaultHandler{Handler: func(_ context.Context, _ event.Event) error {
		return nil
	}})
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Valid binary-mode headers except the required Ce-Type is omitted, so the
	// event decodes but fails validation.
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Ce-Specversion", "1.0")
	req.Header.Set("Ce-Id", "id")
	req.Header.Set("Ce-Source", "example/uri")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an invalid event", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 {
		t.Fatal("expected a validation-error body, got empty")
	}
}

// TestHandler_PanicRecovered verifies a panic in the user function becomes a
// 500 response rather than escaping to net/http and dropping the connection.
func TestHandler_PanicRecovered(t *testing.T) {
	h := newCloudeventHandler(DefaultHandler{Handler: func(_ context.Context, _ event.Event) error {
		panic("boom")
	}})
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.DefaultClient.Do(ceRequest(t, http.MethodPost, srv.URL))
	if err != nil {
		t.Fatalf("request failed (panic escaped to net/http?): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

// TestHandler_ErrorResultBody verifies that when the function returns an HTTP
// result carrying a message and no response event, the handler writes both the
// result's status code and its message body, preserving the SDK behavior.
func TestHandler_ErrorResultBody(t *testing.T) {
	h := newCloudeventHandler(DefaultHandler{Handler: func(_ context.Context, _ event.Event) error {
		return cehttp.NewResult(http.StatusTeapot, "%s", "brewing failed")
	}})
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.DefaultClient.Do(ceRequest(t, http.MethodPost, srv.URL))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTeapot)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("brewing failed")) {
		t.Fatalf("body = %q, want it to contain the result message", body)
	}
}

// TestHandler_ConcurrentCancellation is a regression test for the CloudEvents
// receiver wedge: the SDK's NewHTTPReceiveHandler routes every request through a
// single unbuffered, process-shared channel with a non-cancellable send, so a
// client that cancels a request mid-flight can orphan a concurrent request's
// send. Under sustained concurrent load with occasional cancellations, healthy
// requests then stall for their full deadline.
//
// ceHandler decodes and dispatches inline with no shared channel, so healthy
// requests must never stall no matter how many concurrent requests are
// cancelled. This test hammers the handler with concurrent clients — a fraction
// forcing mid-flight cancellation — and fails if any healthy, instant-handler
// request takes longer than a generous slack.
func TestHandler_ConcurrentCancellation(t *testing.T) {
	// Instant handler: any multi-hundred-ms latency below is pure stall.
	h := newCloudeventHandler(DefaultHandler{Handler: func(_ context.Context, _ event.Event) (*event.Event, error) {
		return nil, nil
	}})
	srv := httptest.NewServer(h)
	// On the buggy (shared-channel) path the wedge leaves goroutines stuck on a
	// non-cancellable channel send, so this deferred Close blocks and the failure
	// surfaces as a `go test` timeout rather than the assertion message below. On
	// the fixed path Close returns promptly. Either way the test fails on the bug.
	defer srv.Close()

	const (
		workers  = 8
		duration = 2 * time.Second
		// slack is the per-request deadline AND the wedge signal: a genuine wedge
		// blocks forever (a non-cancellable channel send that never drains), so
		// any finite deadline catches it. It is set generously so ordinary
		// scheduling jitter on a loaded CI runner (2 cores, -race) never makes a
		// healthy instant-handler request hit it — a false positive that a
		// tighter deadline produced. On the fixed path healthy requests finish in
		// milliseconds regardless, so a large deadline costs nothing here.
		slack = 15 * time.Second
	)

	// A single client with keep-alives and a bounded pool, so hammering
	// localhost does not exhaust ephemeral ports (which would masquerade as
	// stalls). Genuine wedge stalls surface as context deadline exceeded.
	client := &http.Client{Transport: &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 200,
		MaxConnsPerHost:     200,
	}}
	defer client.CloseIdleConnections()

	post := func(ctx context.Context) error {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader([]byte("x")))
		req.Header.Set("Ce-Specversion", "1.0")
		req.Header.Set("Ce-Id", "id")
		req.Header.Set("Ce-Source", "example/uri")
		req.Header.Set("Ce-Type", "example.type")
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		return nil
	}

	var (
		healthyOK, healthyStalled, connErr, cancelled int64
		wg                                            sync.WaitGroup
	)
	deadline := time.Now().Add(duration)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			i := 0
			for time.Now().Before(deadline) {
				i++
				// A quarter of the workers cancel a quarter of their requests
				// almost immediately, to force mid-flight cancellation.
				if w%4 == 0 && i%4 == 0 {
					ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
					_ = post(ctx)
					cancel()
					atomic.AddInt64(&cancelled, 1)
					time.Sleep(time.Millisecond)
					continue
				}
				// Healthy request: generous deadline against an instant handler.
				ctx, cancel := context.WithTimeout(context.Background(), slack)
				err := post(ctx)
				cancel()
				switch {
				case errors.Is(err, context.DeadlineExceeded):
					// The wedge symptom: an instant handler took ~the full
					// deadline. A genuine wedge always surfaces as a full-deadline
					// stall, so DeadlineExceeded alone detects it without the
					// false positives an elapsed-time threshold would produce on a
					// contended CI runner.
					atomic.AddInt64(&healthyStalled, 1)
				case err != nil:
					// Localhost harness noise (e.g. transient dial errors); tolerated.
					atomic.AddInt64(&connErr, 1)
				default:
					atomic.AddInt64(&healthyOK, 1)
				}
				time.Sleep(time.Millisecond)
			}
		}(w)
	}
	wg.Wait()

	t.Logf("cancelled(forced)=%d healthyOK=%d healthyStalled=%d connErr(tolerated)=%d",
		cancelled, healthyOK, healthyStalled, connErr)
	if cancelled == 0 {
		t.Fatal("test did not exercise any cancellations")
	}
	if healthyStalled > 0 {
		t.Fatalf("%d healthy instant-handler requests stalled under concurrent cancellation "+
			"(the shared-channel receiver wedge has regressed)", healthyStalled)
	}
}

// TestHandler_Signatures verifies the inline dispatcher accepts and correctly
// invokes representative CloudEvents SDK function signatures, including the
// response-returning form.
func TestHandler_Signatures(t *testing.T) {
	respEvent := func() *event.Event { e := event.New(); e.SetType("resp.type"); e.SetSource("resp/uri"); return &e }

	cases := []struct {
		name       string
		fn         any
		wantStatus int
		wantCeResp bool
	}{
		{"no-args", func() {}, http.StatusOK, false},
		{"ctx-only", func(_ context.Context) {}, http.StatusOK, false},
		{"event-err", func(_ event.Event) error { return nil }, http.StatusOK, false},
		{"ctx-event-err", func(_ context.Context, _ event.Event) error { return nil }, http.StatusOK, false},
		{"ctx-event-resp", func(_ context.Context, _ event.Event) (*event.Event, error) { return respEvent(), nil }, http.StatusOK, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newCloudeventHandler(DefaultHandler{Handler: tc.fn})
			srv := httptest.NewServer(h)
			defer srv.Close()

			req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte("{}")))
			req.Header.Set("Ce-Specversion", "1.0")
			req.Header.Set("Ce-Id", "id")
			req.Header.Set("Ce-Source", "example/uri")
			req.Header.Set("Ce-Type", "example.type")
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			gotCeResp := resp.Header.Get("Ce-Type") != ""
			if gotCeResp != tc.wantCeResp {
				t.Fatalf("response CloudEvent present = %v, want %v", gotCeResp, tc.wantCeResp)
			}
		})
	}
}
