//go:build !debug
// +build !debug

/*
Copyright 2022 The Knative Authors
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package runtime

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
)

func recoverMiddleware(handler http.Handler) http.Handler {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	f := func(rw http.ResponseWriter, req *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				recoverError := fmt.Errorf("user function error: %v", r)
				stack := string(debug.Stack())
				logger.Printf("%v\n%v\n", recoverError, stack)

				rw.WriteHeader(http.StatusInternalServerError)
			}
		}()
		handler.ServeHTTP(rw, req)
	}
	return http.HandlerFunc(f)
}
