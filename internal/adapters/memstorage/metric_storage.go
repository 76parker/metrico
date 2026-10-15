// Package memstorage реализует хранилище метрик в памяти процесса.
package memstorage

import (
	"context"
	"maps"
	"sync"

	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
	"github.com/76Parker/metrico/internal/domain/metrics"
)

// MemStorage хранит метрики в памяти и синхронизирует операции с ними.
// Создавайте хранилище через NewMemStorage. Возвращаемые метрики содержат
// указатели на значения, которые вызывающий код не должен изменять.
type MemStorage struct {
	mu      *sync.Mutex
	metrics map[string]metrics.Metrics
}

var _ metricsapp.Repository = (*MemStorage)(nil)

// NewMemStorage создаёт пустое хранилище метрик, готовое к использованию.
func NewMemStorage() *MemStorage {
	reservation := 1024 // Резервируем место в памяти для 1024 метрик, для избежания лишних аллокаций
	return &MemStorage{
		mu:      &sync.Mutex{},
		metrics: make(map[string]metrics.Metrics, reservation),
	}
}

// Apply атомарно применяет пакет изменений: ошибочная операция оставляет хранилище прежним.
func (s *MemStorage) Apply(_ context.Context, changes []metricsapp.Change) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	updated := s.cloneMetrics()
	for _, change := range changes {
		if err := s.applyChange(updated, change); err != nil {
			return err
		}
	}
	s.metrics = updated

	return nil
}

// Get возвращает метрику по имени или [metrics.ErrMetricNotFound], если она отсутствует.
func (s *MemStorage) Get(_ context.Context, metricName string) (metrics.Metrics, error) {
	if metricName == "" {
		return metrics.Metrics{}, metrics.ErrMetricNameIsEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	metric, ok := s.metrics[metricName]
	if !ok {
		return metrics.Metrics{}, metrics.ErrMetricNotFound
	}
	return metric, nil
}

// Load заменяет весь набор метрик переданным снимком без валидации.
// Значения метрик не копируются глубоко: не изменяйте их после загрузки.
func (s *MemStorage) Load(_ context.Context, metricsSlice []metrics.Metrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = make(map[string]metrics.Metrics, len(metricsSlice))
	for _, metric := range metricsSlice {
		s.metrics[metric.ID] = metric
	}
	return nil
}

func (s *MemStorage) applyChange(items map[string]metrics.Metrics, change metricsapp.Change) error {
	metricType := change.MetricType()
	if metricType == metrics.MetricTypeInvalid {
		return metrics.ErrInvalidMetricType
	}
	if change.Name() == "" {
		return metrics.ErrMetricNameIsEmpty
	}
	switch metricType {
	case metrics.MetricTypeGauge:
		s.applyGauge(items, change.Name(), change.Value())
	case metrics.MetricTypeCounter:
		s.applyCounter(items, change.Name(), change.Delta())
	}

	return nil
}

func (s *MemStorage) applyGauge(items map[string]metrics.Metrics, metricName string, value float64) {
	items[metricName] = metrics.Metrics{
		ID:    metricName,
		Type:  metrics.MetricTypeGauge,
		Value: &value,
	}
}

func (s *MemStorage) applyCounter(items map[string]metrics.Metrics, metricName string, delta int64) {
	if metric, ok := items[metricName]; ok && metric.Type == metrics.MetricTypeCounter && metric.Delta != nil {
		delta += *metric.Delta
	}
	items[metricName] = metrics.Metrics{
		ID:    metricName,
		Type:  metrics.MetricTypeCounter,
		Delta: &delta,
	}
}

func (s *MemStorage) cloneMetrics() map[string]metrics.Metrics {
	clone := make(map[string]metrics.Metrics, len(s.metrics))
	maps.Copy(clone, s.metrics)
	return clone
}

// GetAll возвращает снимок набора метрик без гарантии порядка.
// Срез создаётся заново, значения по указателям остаются общими с хранилищем.
func (s *MemStorage) GetAll(ctx context.Context) ([]metrics.Metrics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.createMetricSnapshot()
	return result, nil
}

func (s *MemStorage) createMetricSnapshot() []metrics.Metrics {
	result := make([]metrics.Metrics, 0, len(s.metrics))
	for _, v := range s.metrics {
		result = append(result, v)
	}
	return result
}
