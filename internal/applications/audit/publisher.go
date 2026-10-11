package audit

import (
	"context"
	"errors"
	"slices"
)

type Observer interface {
	Notify(ctx context.Context, event Event) error
}

type errorLogger interface {
	Error(msg string, keysAndValues ...any)
}

type Publisher struct {
	observers []Observer
	log       errorLogger
}

func NewPublisher(log errorLogger, observers ...Observer) (*Publisher, error) {
	for _, observer := range observers {
		if observer == nil {
			return nil, errors.New("audit observer is required")
		}
	}
	return &Publisher{observers: slices.Clone(observers), log: log}, nil
}

func (p *Publisher) Notify(ctx context.Context, event Event) {
	for _, observer := range p.observers {
		if err := observer.Notify(ctx, event); err != nil {
			p.log.Error("audit delivery failed", "error", err)
		}
	}
}
