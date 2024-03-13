package main

import (
	"fmt"
	"net/http"
	"os"

	fn "knative.dev/func-go/http"
)

// Main illustrates how scaffolding works to wrap a user's function.
func main() {
	// Instanced example (in scaffolding, 'New()' will be in module 'f')
	if err := fn.Start(New()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	// Static example (in scaffolding 'Handle' will be in module f
	// if err := fn.Start(fn.DefaultHandler{Handle}); err != nil {
	// 	fmt.Fprintln(os.Stderr, err.Error())
	// 	os.Exit(1)
	// }
}

// Example Static HTTP Handler implementation.
func Handle(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Static HTTP handler invoked")
	fmt.Fprintln(w, "Static HTTP Handler invoked")
}

// MyFunction is an example instanced HTTP function implementation.
type MyFunction struct{}

func New() *MyFunction {
	return &MyFunction{}
}

func (f *MyFunction) Handle(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Instanced HTTP handler invoked")
	fmt.Fprintln(w, "Instanced HTTP Handler invoked")
}
