package metrics

// ApplicationOption используется для конфигурации Application
type ApplicationOption func(*Application)

// WithSnapshotter включает уведомление переданного координатора после успешной записи метрик.
func WithSnapshotter(notifier changeNotifier) ApplicationOption {
	return func(app *Application) {
		app.metricsChangeNotifier = notifier
	}
}
