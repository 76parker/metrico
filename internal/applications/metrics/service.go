// Package metrics реализует сценарии чтения, изменения и сохранения метрик.
package metrics

import (
	"context"
	"strconv"

	"github.com/76Parker/metrico/internal/domain/metrics"
)

// Repository задаёт операции хранения, необходимые сервису метрик.
type Repository interface {
	// Apply применяет список изменений к хранилищу метрик (создает либо обновляет),
	// является единой логикой обновления для single-update и batch-update.
	Apply(ctx context.Context, mutations []Change) error
	// Get возвращает метрику по имени или ошибку, если она отсутствует.
	Get(ctx context.Context, metricName string) (metrics.Metrics, error)
	// GetAll возвращает текущий набор метрик без гарантии порядка.
	GetAll(ctx context.Context) ([]metrics.Metrics, error)
}

// Service проверяет команды и преобразует их в операции выбранного хранилища.
type Service struct {
	storage Repository
}

// NewService создаёт сервис, использующий переданное хранилище для всех операций.
func NewService(storage Repository) *Service {
	return &Service{
		storage: storage,
	}
}
func (s *Service) update(ctx context.Context, cmd UpdateCommand) error {
	mutation, err := buildChangeOperation(cmd)
	if err != nil {
		return err
	}
	if err := s.storage.Apply(ctx, []Change{mutation}); err != nil {
		return err
	}
	return nil
}

func (s *Service) batchUpdate(ctx context.Context, cmd []UpdateCommand) error {
	changes := make([]Change, 0, len(cmd))
	for _, c := range cmd {
		mutation, err := buildChangeOperation(c)
		if err != nil {
			return err
		}
		changes = append(changes, mutation)
	}
	if err := s.storage.Apply(ctx, changes); err != nil {
		return err
	}
	return nil
}

func (s *Service) getByName(ctx context.Context, name string) (metrics.Metrics, error) {
	metric, err := s.storage.Get(ctx, name)
	if err != nil {
		return metrics.Metrics{}, err
	}
	return metric, nil
}
func (s *Service) getAll(ctx context.Context) ([]metrics.Metrics, error) {
	return s.storage.GetAll(ctx)
}

func buildChangeOperation(cmd UpdateCommand) (Change, error) {
	if cmd.Name == "" {
		return Change{}, metrics.ErrMetricNameIsEmpty
	}
	if cmd.MetricType == "" {
		return Change{}, metrics.ErrMetricTypeIsEmpty
	}
	switch metrics.MetricType(cmd.MetricType) {
	case metrics.MetricTypeGauge:
		value, err := strconv.ParseFloat(cmd.Value, 64)
		if err != nil {
			return Change{}, metrics.ErrInvalidValueForGauge
		}
		return Change{
			name:  cmd.Name,
			kind:  ChangeKindSetGauge,
			value: value,
		}, nil
	case metrics.MetricTypeCounter:
		delta, err := strconv.ParseInt(cmd.Value, 10, 64)
		if err != nil {
			return Change{}, metrics.ErrInvalidValueForCounter
		}
		return Change{
			name:  cmd.Name,
			kind:  ChangeKindAddCounter,
			delta: delta,
		}, nil
	default:
		return Change{}, metrics.ErrInvalidMetricType
	}
}
