package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudevents/sdk-go/v2/event"

	ce "github.com/lkingland/func-runtimes/go/cloudevents"
)

// Main illustrates how scaffolding works to wrap a user's function.
func main() {
	// Instanced Example
	// (in scaffolding 'New()' will be in module 'f')
	if err := ce.Start(New()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	// Static Example
	// (in scaffolding 'Handle' will be in the module 'f')
	// if err := ce.Start(ce.DefaultHandler{Handle}); err != nil {
	//   fmt.Fprintln(os.Stderr, err.Error())
	//   os.Exit(1)
	// }
}

// Handle is an example static function implementation.
func Handle(ctx context.Context, e event.Event) (*event.Event, error) {
	fmt.Println("Static CE Handler invoked")
	return &e, nil // echo to caller
}

// MyFunction is an example instanced CloudEvents function implementation.
type MyFunction struct{}

func New() *MyFunction {
	return &MyFunction{}
}

/*
Supported method signatures:

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
func (f *MyFunction) Handle() {
	fmt.Println("Instanced CloudEvents handler invoked")
}
