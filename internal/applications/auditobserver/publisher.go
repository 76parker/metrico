package auditobserver

import (
	"context"
	"errors"
	"slices"
)

// Observer принимает события аудита; реализация должна допускать конкурентные вызовы Notify.
type Observer interface {
	// Notify доставляет событие приёмнику и возвращает ошибку доставки.
	Notify(ctx context.Context, event Event) error
}

type errorLogger interface {
	Error(msg string, keysAndValues ...any)
}

// Publisher последовательно доставляет событие всем настроенным приёмникам аудита.
type Publisher struct {
	observers []Observer
	log       errorLogger
}

// NewPublisher создаёт издателя с копией списка приёмников и отклоняет nil-приёмники.
func NewPublisher(log errorLogger, observers ...Observer) (*Publisher, error) {
	for _, observer := range observers {
		if observer == nil {
			return nil, errors.New("audit observer is required")
		}
	}
	return &Publisher{observers: slices.Clone(observers), log: log}, nil
}

// Notify доставляет событие каждому приёмнику, журналируя ошибки без остановки доставки.
func (p *Publisher) Notify(ctx context.Context, event Event) {
	for _, observer := range p.observers {
		if err := observer.Notify(ctx, event); err != nil {
			p.log.Error("audit delivery failed", "error", err)
		}
	}
}
