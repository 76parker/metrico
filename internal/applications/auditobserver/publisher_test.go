package auditobserver

import (
	"context"
	"errors"
	"testing"
)

type observerFunc func(context.Context, Event) error

func (notify observerFunc) Notify(ctx context.Context, event Event) error { return notify(ctx, event) }

type loggerStub struct {
	calls   int
	message string
	values  []any
}

func (logger *loggerStub) Error(message string, values ...any) {
	logger.calls++
	logger.message = message
	logger.values = values
}

func TestPublisher_DeliversToEveryObserverAndLogsFailures(t *testing.T) {
	ctx := t.Context()
	event := Event{TS: 42, Metrics: []string{"load"}, IPAddress: "127.0.0.1"}
	log := &loggerStub{}
	var delivered []Event
	observers := []Observer{
		observerFunc(func(actual context.Context, got Event) error {
			if actual != ctx {
				t.Error("context was not preserved")
			}
			delivered = append(delivered, got)
			return errors.New("offline")
		}),
		observerFunc(func(_ context.Context, got Event) error { delivered = append(delivered, got); return nil }),
	}
	publisher, err := NewPublisher(log, observers...)
	if err != nil {
		t.Fatal(err)
	}
	observers[0] = observerFunc(func(context.Context, Event) error { t.Fatal("publisher retained caller's observer slice"); return nil })
	publisher.Notify(ctx, event)
	if len(delivered) != 2 || delivered[0].TS != event.TS || delivered[1].IPAddress != event.IPAddress {
		t.Errorf("delivered events = %+v", delivered)
	}
	if log.calls != 1 || log.message != "audit delivery failed" || len(log.values) != 2 || log.values[0] != "error" {
		t.Errorf("logged failures = %+v", log)
	}
}

func TestNewPublisher_RejectsNilObserver(t *testing.T) {
	if _, err := NewPublisher(&loggerStub{}, observerFunc(nil), nil); err == nil {
		t.Fatal("nil observer accepted")
	}
}
