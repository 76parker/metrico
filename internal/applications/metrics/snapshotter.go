package metrics

import (
	"context"
	"sync"
	"time"

	"log"

	"github.com/76Parker/metrico/internal/domain/metrics"
)

type snapshotMode uint8

const (
	snapshotModeUnknown snapshotMode = iota
	snapshotModeAsync
	snapshotModeSync
)

type metricsStore interface {
	GetAll(ctx context.Context) ([]metrics.Metrics, error)
	Load(ctx context.Context, metrics []metrics.Metrics) error
}

type snapshotStore interface {
	Save(ctx context.Context, metrics []metrics.Metrics) error
	Restore(ctx context.Context) ([]metrics.Metrics, error)
}

type snapshotCoordinatorState struct {
	restoreOnce sync.Once
}

type snapshotPolicy struct {
	mode         snapshotMode
	saveInterval time.Duration
}

// SnapshotCoordinator восстанавливает метрики и управляет сохранением снимков.
// Содержит состояние синхронизации и не должен копироваться после использования.
type SnapshotCoordinator struct {
	policy       snapshotPolicy
	store        snapshotStore
	metricsStore metricsStore
	snapshotCoordinatorState
}

// NewSnapshotCoordinator создаёт координатор сохранения снимков.
// Нулевой saveInterval сохраняет снимок при изменениях, положительный — в Run.
// Для периодического режима передавайте положительный интервал.
func NewSnapshotCoordinator(store snapshotStore, metricsStore metricsStore, saveInterval time.Duration) *SnapshotCoordinator {
	var mode snapshotMode
	if saveInterval == 0 {
		mode = snapshotModeSync
	} else {
		mode = snapshotModeAsync
	}
	policy := snapshotPolicy{
		mode:         mode,
		saveInterval: saveInterval,
	}
	return &SnapshotCoordinator{
		policy:       policy,
		store:        store,
		metricsStore: metricsStore,
	}
}

// Restore однократно пытается загрузить снимок в хранилище метрик.
// Повторный вызов не выполняет восстановление, даже если первая попытка завершилась ошибкой.
func (s *SnapshotCoordinator) Restore(ctx context.Context) error {
	var restoreErr error
	s.restoreOnce.Do(func() {
		metrics, err := s.store.Restore(ctx)
		if err != nil {
			restoreErr = err
			return
		}
		if err := s.metricsStore.Load(ctx, metrics); err != nil {
			restoreErr = err
			return
		}
	})
	return restoreErr
}

// Run периодически сохраняет снимки до отмены ctx, журналируя ошибки сохранения.
// В синхронном режиме сразу возвращает nil без запуска периодического сохранения.
func (s *SnapshotCoordinator) Run(ctx context.Context) error {
	if s.policy.mode != snapshotModeAsync {
		return nil
	}

	ticker := time.NewTicker(s.policy.saveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.runSync(ctx); err != nil {
				log.Printf("[ASYNC SNAPSHOTTER] Failed to save snapshot: %v", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// onMetricsChanged метод который уведомляет snapshotCoordinator о том что метрики в физическом хранилище изменились
func (s *SnapshotCoordinator) onMetricsChanged(ctx context.Context) error {
	if s.policy.mode == snapshotModeSync {
		return s.runSync(ctx)
	} else {
		return nil
	}
}

// runSync запускает синхронный процесс сохранения снапшота из физического хранилища в хранилище снапшотов
func (s *SnapshotCoordinator) runSync(ctx context.Context) error {
	metrics, err := s.metricsStore.GetAll(ctx)
	if err != nil {
		return err
	}
	return s.store.Save(ctx, metrics)
}
