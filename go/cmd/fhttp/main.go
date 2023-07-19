package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	fn "github.com/lkingland/func-runtimes/go/http"
)

func main() {
	i := fn.DefaultHandler{Handler: Handle}
	err := fn.Start(i)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

// Example Static Handler
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	fmt.Println("Hello World")
}
