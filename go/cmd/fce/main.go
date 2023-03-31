package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudevents/sdk-go/v2/event"

	fn "github.com/lkingland/func-runtimes/go/cloudevents"
)

func main() {
	// Static:
	i := fn.DefaultHandler{Handle}
	err := fn.Start(i)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	// Instanced

}

// Handle an event
func Handle(ctx context.Context, e event.Event) (*event.Event, error) {
	/*
	 * YOUR CODE HERE
	 *
	 * Try running `go test`.  Add more test as you code in `handle_test.go`.
	 */

	fmt.Println("Received event")
	fmt.Println(e) // echo to local output
	return &e, nil // echo to caller
}

/*
Other supported function signatures:

  Handle()
  Handle() error
  Handle(context.Context)
  Handle(context.Context) error
  Handle(event.Event)
  Handle(event.Event) error
  Handle(context.Context, event.Event)
  Handle(context.Context, event.Event) error
  Handle(event.Event) *event.Event
  Handle(event.Event) (*event.Event, error)
  Handle(context.Context, event.Event) *event.Event
  Handle(context.Context, event.Event) (*event.Event, error)

*/
