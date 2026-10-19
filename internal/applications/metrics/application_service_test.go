package metrics

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	domain "github.com/76Parker/metrico/internal/domain/metrics"
)

type changeNotifierFunc func(context.Context) error

func (notify changeNotifierFunc) onMetricsChanged(ctx context.Context) error { return notify(ctx) }

func TestApplication_UpdatesAndNotifiesAfterSuccessfulWrite(t *testing.T) {
	storageErr, snapshotErr := errors.New("storage"), errors.New("snapshot")
	for _, batch := range []bool{false, true} {
		name := "single"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			for _, test := range []struct {
				name                             string
				writeErr, notifyErr, expectedErr error
			}{
				{name: "success"},
				{name: "storage failure", writeErr: storageErr, expectedErr: storageErr},
				{name: "snapshot failure", notifyErr: snapshotErr, expectedErr: snapshotErr},
			} {
				t.Run(test.name, func(t *testing.T) {
					repository := NewMockRepository(gomock.NewController(t))
					commands := []UpdateCommand{{Name: "temperature", MetricType: "gauge", Value: "1.5"}}
					changes := []Change{{name: "temperature", kind: ChangeKindSetGauge, value: 1.5}}
					if batch {
						commands = append(commands, UpdateCommand{Name: "requests", MetricType: "counter", Value: "3"})
						changes = append(changes, Change{name: "requests", kind: ChangeKindAddCounter, delta: 3})
					}
					ctx := t.Context()
					hasWritten, notifications := false, 0
					repository.EXPECT().Apply(ctx, changes).DoAndReturn(func(context.Context, []Change) error { hasWritten = true; return test.writeErr })
					application := NewApplication(NewService(repository), WithSnapshotter(changeNotifierFunc(func(actualCtx context.Context) error {
						if !hasWritten || actualCtx != ctx {
							t.Error("notification must follow the write and preserve context")
						}
						notifications++
						return test.notifyErr
					})))
					var err error
					if batch {
						err = application.BatchUpdate(ctx, commands)
					} else {
						err = application.Update(ctx, commands[0])
					}
					if !errors.Is(err, test.expectedErr) {
						t.Fatalf("update error = %v, want %v", err, test.expectedErr)
					}
					wantNotifications := 1
					if test.writeErr != nil {
						wantNotifications = 0
					}
					if notifications != wantNotifications {
						t.Errorf("notifications = %d, want %d", notifications, wantNotifications)
					}
				})
			}
		})
	}
}

func TestApplication_RejectsInvalidCommandsBeforeWriting(t *testing.T) {
	for _, test := range []struct {
		name        string
		command     UpdateCommand
		expectedErr error
	}{
		{name: "empty name", command: UpdateCommand{MetricType: "gauge", Value: "1"}, expectedErr: domain.ErrMetricNameIsEmpty},
		{name: "empty type", command: UpdateCommand{Name: "metric", Value: "1"}, expectedErr: domain.ErrMetricTypeIsEmpty},
		{name: "unknown type", command: UpdateCommand{Name: "metric", MetricType: "unknown", Value: "1"}, expectedErr: domain.ErrInvalidMetricType},
		{name: "invalid gauge", command: UpdateCommand{Name: "metric", MetricType: "gauge", Value: "bad"}, expectedErr: domain.ErrInvalidValueForGauge},
		{name: "invalid counter", command: UpdateCommand{Name: "metric", MetricType: "counter", Value: "1.5"}, expectedErr: domain.ErrInvalidValueForCounter},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := NewMockRepository(gomock.NewController(t))
			application := NewApplication(NewService(repository))
			if err := application.Update(t.Context(), test.command); !errors.Is(err, test.expectedErr) {
				t.Errorf("Update error = %v, want %v", err, test.expectedErr)
			}
			commands := []UpdateCommand{{Name: "valid", MetricType: "counter", Value: "1"}, test.command}
			if err := application.BatchUpdate(t.Context(), commands); !errors.Is(err, test.expectedErr) {
				t.Errorf("BatchUpdate error = %v, want %v", err, test.expectedErr)
			}
		})
	}
}

func TestApplication_ReadsAndPreservesStorageErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		storageErr error
	}{{name: "success"}, {name: "storage failure", storageErr: errors.New("read failed")}} {
		t.Run(test.name, func(t *testing.T) {
			repository := NewMockRepository(gomock.NewController(t))
			value := int64(7)
			metric := domain.Metrics{ID: "requests", Type: domain.MetricTypeCounter, Delta: &value}
			metrics := []domain.Metrics{metric}
			if test.storageErr != nil {
				metric = domain.Metrics{}
				metrics = nil
			}
			repository.EXPECT().Get(t.Context(), "requests").Return(metric, test.storageErr)
			repository.EXPECT().GetAll(t.Context()).Return(metrics, test.storageErr)
			application := NewApplication(NewService(repository))
			got, err := application.GetByName(t.Context(), "requests")
			if !errors.Is(err, test.storageErr) || got != metric {
				t.Errorf("GetByName = %v, %v", got, err)
			}
			gotAll, err := application.GetAll(t.Context())
			if !errors.Is(err, test.storageErr) || len(gotAll) != len(metrics) {
				t.Errorf("GetAll = %v, %v", gotAll, err)
			}
		})
	}
}
