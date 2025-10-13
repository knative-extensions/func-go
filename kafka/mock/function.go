package mock

import (
	"context"

	"github.com/IBM/sarama"
)

type Function struct {
	OnStart  func(context.Context, map[string]string) error
	OnStop   func(context.Context) error
	OnHandle func(context.Context, *sarama.ConsumerMessage) error
	OnReady  func(context.Context) (bool, error)
	OnAlive  func(context.Context) (bool, error)
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

func (f *Function) Handle(ctx context.Context, msg *sarama.ConsumerMessage) error {
	if f.OnHandle != nil {
		return f.OnHandle(ctx, msg)
	}
	return nil
}

func (f *Function) Ready(ctx context.Context) (bool, error) {
	if f.OnReady != nil {
		return f.OnReady(ctx)
	}
	return true, nil
}

func (f *Function) Alive(ctx context.Context) (bool, error) {
	if f.OnAlive != nil {
		return f.OnAlive(ctx)
	}
	return true, nil
}