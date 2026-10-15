package metrics

import (
	"context"

	"github.com/76Parker/metrico/internal/domain/metrics"
)

type service interface {
	update(ctx context.Context, cmd UpdateCommand) error
	batchUpdate(ctx context.Context, cmd []UpdateCommand) error
	getAll(ctx context.Context) ([]metrics.Metrics, error)
	getByName(ctx context.Context, name string) (metrics.Metrics, error)
}

type noopSnapshotter struct{}

func (n *noopSnapshotter) onMetricsChanged(ctx context.Context) error {
	return nil
}

type changeNotifier interface {
	onMetricsChanged(ctx context.Context) error
}

// Application объединяет операции с метриками и уведомление о необходимости снимка.
type Application struct {
	metricsService        service
	metricsChangeNotifier changeNotifier
}

// NewApplication создаёт приложение метрик; без опций сохранение снимков отключено.
func NewApplication(metricsService service, opts ...ApplicationOption) *Application {
	app := &Application{
		metricsService:        metricsService,
		metricsChangeNotifier: &noopSnapshotter{}, // По умолчанию без применения опций - metricsChangeNotifier не используется
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

// Update применяет одну команду и уведомляет механизм сохранения об изменении.
// Ошибка сохранения может возвращаться после успешного обновления хранилища.
func (s *Application) Update(ctx context.Context, cmd UpdateCommand) error {
	err := s.metricsService.update(ctx, cmd)
	if err != nil {
		return err
	}
	return s.metricsChangeNotifier.onMetricsChanged(ctx)
}

// BatchUpdate проверяет все команды до передачи единого пакета изменений хранилищу.
// После успешной записи уведомляет механизм сохранения снимков.
func (s *Application) BatchUpdate(ctx context.Context, cmd []UpdateCommand) error {
	err := s.metricsService.batchUpdate(ctx, cmd)
	if err != nil {
		return err
	}
	return s.metricsChangeNotifier.onMetricsChanged(ctx)
}

// GetByName возвращает метрику по имени или ошибку выбранного хранилища.
func (s *Application) GetByName(ctx context.Context, name string) (metrics.Metrics, error) {
	return s.metricsService.getByName(ctx, name)
}

// GetAll возвращает текущие метрики без гарантии порядка.
func (s *Application) GetAll(ctx context.Context) ([]metrics.Metrics, error) {
	return s.metricsService.getAll(ctx)
}
