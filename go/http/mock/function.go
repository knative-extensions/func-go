package mock

import "context"

type Function struct {
	OnStart func(context.Context, map[string]string) error
}

func (f *Function) Start(ctx context.Context, cfg map[string]string) error {
	if f.OnStart != nil {
		return f.OnStart(ctx, cfg)
	}
	return nil
}
