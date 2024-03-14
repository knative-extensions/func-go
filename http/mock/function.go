package mock

import (
	"context"
	"net/http"
)

type Function struct {
	OnStart  func(context.Context, map[string]string) error
	OnStop   func(context.Context) error
	OnHandle func(http.ResponseWriter, *http.Request)
}

func (f *Function) Start(ctx context.Context, cfg map[string]string) error {
	if f.OnStart != nil {
		return f.OnStart(ctx, cfg)
	}
	return nil
}

func (f *Function) Stop(ctx context.Context) error {
	if f.OnStop != nil {
		return f.OnStop(ctx)
	}
	return nil
}

func (f *Function) Handle(w http.ResponseWriter, r *http.Request) {
	if f.OnHandle != nil {
		f.OnHandle(w, r)
	}
}
