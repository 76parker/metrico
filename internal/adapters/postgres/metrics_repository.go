package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/76Parker/metrico/internal/adapters/postgres/pgen"
	metricapp "github.com/76Parker/metrico/internal/applications/metrics"
	"github.com/76Parker/metrico/internal/domain/metrics"
	goccyjson "github.com/goccy/go-json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type metricsRepo struct {
	q *pgen.Queries
}

// NewMetricsRepository создаёт адаптер метрик поверх подготовленных sqlc-запросов.
func NewMetricsRepository(q *pgen.Queries) *metricsRepo {
	return &metricsRepo{q: q}
}

// Apply преобразует изменения в один SQL-батч и выполняет его без повторных попыток.
// Повтор counter-записи мог бы повторно применить приращение.
func (r *metricsRepo) Apply(ctx context.Context, changes []metricapp.Change) error {
	batch, err := makeBatch(changes)
	if err != nil {
		return err
	}

	batchByte, err := goccyjson.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}
	// Retry-вставка небезопасна - вызываем без retry
	if err := r.q.BatchUpsert(ctx, batchByte); err != nil {
		return fmt.Errorf("batch upsert: %w", err)
	}
	return nil
}

// makeBatch преобразует список изменений в батч для вставки в базу данных
func makeBatch(changes []metricapp.Change) (pgen.BatchUpsertParam, error) {
	batch := make(pgen.BatchUpsertParam, 0, len(changes))
	indices := make(map[string]int, len(changes))
	for _, change := range changes {
		batchElem := pgen.BatchUpsertMetric{
			Name: change.Name(),
			Type: string(change.MetricType()),
		}
		switch change.MetricType() {
		case metrics.MetricTypeGauge:
			value := change.Value()
			batchElem.Value = &value
		case metrics.MetricTypeCounter:
			delta := change.Delta()
			batchElem.Delta = &delta
		}

		if index, ok := indices[change.Name()]; ok {
			if batch[index].Type != batchElem.Type {
				batch[index] = batchElem
				continue
			}

			switch change.MetricType() {
			case metrics.MetricTypeGauge:
				batch[index] = batchElem
			case metrics.MetricTypeCounter:
				delta := *batch[index].Delta + *batchElem.Delta
				batch[index].Delta = &delta
			}
			continue
		}
		batch = append(batch, batchElem)
		indices[change.Name()] = len(batch) - 1
	}

	return batch, nil
}

// Get возвращает метрику по имени; отсутствие строки преобразует в [metrics.ErrMetricNotFound].
func (r *metricsRepo) Get(ctx context.Context, metricName string) (metrics.Metrics, error) {

	var metric pgen.MetricMetric

	err := withRetry(func() error {
		m, err := r.q.GetByName(ctx, metricName)
		if err != nil {
			return err
		}
		metric = m
		return nil
	})
	if err != nil {
		return metrics.Metrics{}, newGetMetricError(err)
	}
	return metrics.Metrics{
		ID:    metric.Name,
		Type:  metrics.MetricType(metric.Type),
		Value: nullableFloat64(metric.Value),
		Delta: nullableInt64(metric.Delta),
	}, nil
}

func newGetMetricError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return metrics.ErrMetricNotFound
	}

	return fmt.Errorf("get metric by name: %w", err)
}

// GetAll возвращает все сохранённые метрики без гарантии порядка.
func (r *metricsRepo) GetAll(ctx context.Context) ([]metrics.Metrics, error) {

	var listedMetrics []pgen.MetricMetric
	err := withRetry(func() error {
		lm, err := r.q.List(ctx)
		if err != nil {
			return err
		}
		listedMetrics = lm
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list metrics: %w", err)
	}
	result := make([]metrics.Metrics, 0, len(listedMetrics))
	for _, metric := range listedMetrics {
		result = append(result, metrics.Metrics{
			ID:    metric.Name,
			Type:  metrics.MetricType(metric.Type),
			Value: nullableFloat64(metric.Value),
			Delta: nullableInt64(metric.Delta),
		})
	}
	return result, nil

}

func nullableFloat64(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}

	return &value.Float64
}

func nullableInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}

	return &value.Int64
}
