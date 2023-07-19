# knative.dev/runtime

This repository contains the function invocation and runtime framework for
Knative Go functions.

Invoked via the Scaffolding Middleware when a Function is either run (locally)
or deployed to turn a function (signature) into a service for the host OS.

## Overview

Knative Go functions are a way to run Go code in a serverless environment. The
functions are invoked by the Knative runtime, which provides the function with
an event and a context. The function then processes the event and returns a
result.

## Getting started

Typically, this runtime is used in the context of a Knative Go function. You
can create a new function using the Knative Go function template.

```
func create -l go --builder=host
```

This will create a new Go project that uses the Knative Go function invocation
framework. You can then build and deploy the function using the Knative
Functions CLI.

```
func deploy
```

## More information

For more information about Knative Go functions, see the
[Knative Functions documentation](https://knative.dev/docs/functions/).
