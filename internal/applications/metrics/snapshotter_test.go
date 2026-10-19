package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/76Parker/metrico/internal/domain/metrics"
)

type snapshotStoreStub struct {
	saved               chan []domain.Metrics
	restored            []domain.Metrics
	restoreErr, saveErr error
	restoreCalls        int
	saveCalls           int
	savedMetrics        []domain.Metrics
}

func (s *snapshotStoreStub) Save(_ context.Context, metrics []domain.Metrics) error {
	s.saveCalls++
	s.savedMetrics = metrics
	if s.saved != nil {
		s.saved <- metrics
	}
	return s.saveErr
}
func (s *snapshotStoreStub) Restore(context.Context) ([]domain.Metrics, error) {
	s.restoreCalls++
	return s.restored, s.restoreErr
}

type metricsStoreStub struct {
	metrics             []domain.Metrics
	getErr, loadErr     error
	loaded              []domain.Metrics
	getCalls, loadCalls int
}

func (s *metricsStoreStub) GetAll(context.Context) ([]domain.Metrics, error) {
	s.getCalls++
	return s.metrics, s.getErr
}
func (s *metricsStoreStub) Load(_ context.Context, metrics []domain.Metrics) error {
	s.loadCalls++
	s.loaded = metrics
	return s.loadErr
}

func TestSnapshotCoordinator_RestoreAttemptsOnlyOnce(t *testing.T) {
	restoreErr := errors.New("restore failed")
	loadErr := errors.New("load failed")
	for _, test := range []struct {
		name                              string
		storeErr, metricsErr, expectedErr error
	}{
		{name: "restore failure", storeErr: restoreErr, expectedErr: restoreErr},
		{name: "load failure", metricsErr: loadErr, expectedErr: loadErr},
		{name: "success"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &snapshotStoreStub{restored: []domain.Metrics{{ID: "m"}}, restoreErr: test.storeErr}
			metricsStore := &metricsStoreStub{loadErr: test.metricsErr}
			coordinator := NewSnapshotCoordinator(store, metricsStore, 0)
			for attempt := 0; attempt < 2; attempt++ {
				gotErr := coordinator.Restore(t.Context())
				wantErr := test.expectedErr
				if attempt == 1 {
					wantErr = nil
				}
				if !errors.Is(gotErr, wantErr) {
					t.Fatalf("Restore error = %v, want %v", gotErr, wantErr)
				}
			}
			if store.restoreCalls != 1 {
				t.Errorf("restore calls = %d, want 1", store.restoreCalls)
			}
			wantLoads := 1
			if test.storeErr != nil {
				wantLoads = 0
			}
			if metricsStore.loadCalls != wantLoads {
				t.Errorf("load calls = %d, want %d", metricsStore.loadCalls, wantLoads)
			}
		})
	}
}

func TestSnapshotCoordinator_SynchronousSaveAndErrors(t *testing.T) {
	saveErr, getErr := errors.New("save failed"), errors.New("read failed")
	for _, test := range []struct {
		name                         string
		getErr, saveErr, expectedErr error
	}{
		{name: "success"},
		{name: "read failure", getErr: getErr, expectedErr: getErr},
		{name: "save failure", saveErr: saveErr, expectedErr: saveErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &snapshotStoreStub{saveErr: test.saveErr}
			metricsStore := &metricsStoreStub{metrics: []domain.Metrics{{ID: "m"}}, getErr: test.getErr}
			coordinator := NewSnapshotCoordinator(store, metricsStore, 0)
			if err := coordinator.onMetricsChanged(t.Context()); !errors.Is(err, test.expectedErr) {
				t.Fatalf("save error = %v, want %v", err, test.expectedErr)
			}
			if metricsStore.getCalls != 1 {
				t.Errorf("read calls = %d, want 1", metricsStore.getCalls)
			}
			if test.getErr == nil && store.saveCalls != 1 {
				t.Errorf("save calls = %d, want 1", store.saveCalls)
			}
			if test.getErr != nil && store.saveCalls != 0 {
				t.Errorf("save calls = %d after read failure, want 0", store.saveCalls)
			}
			if test.getErr == nil && len(store.savedMetrics) != 1 {
				t.Errorf("saved metrics = %v", store.savedMetrics)
			}
		})
	}
}

func TestSnapshotCoordinator_PeriodicSaveAndCancellation(t *testing.T) {
	saved := make(chan []domain.Metrics, 1)
	store := &snapshotStoreStub{saved: saved, saveErr: errors.New("logged but ignored")}
	metricsStore := &metricsStoreStub{metrics: []domain.Metrics{{ID: "periodic"}}}
	coordinator := NewSnapshotCoordinator(store, metricsStore, time.Millisecond)
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1)
	go func() { finished <- coordinator.Run(ctx) }()
	select {
	case got := <-saved:
		if len(got) != 1 || got[0].ID != "periodic" {
			t.Errorf("saved metrics = %v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("periodic save did not happen")
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Errorf("Run error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
	if metricsStore.getCalls != 1 {
		t.Errorf("periodic reads = %d, want 1", metricsStore.getCalls)
	}
}

func TestSnapshotCoordinator_RunInSyncModeReturnsImmediately(t *testing.T) {
	coordinator := NewSnapshotCoordinator(&snapshotStoreStub{}, &metricsStoreStub{}, 0)
	if err := coordinator.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
}
