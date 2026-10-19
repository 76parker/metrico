// Package health проверяет доступность зависимостей сервера перед приёмом трафика.
package health

import "context"

// AvailabilityResult содержит общую готовность приложения и результаты отдельных проверок.
type AvailabilityResult struct {
	// Ready показывает доступна ли система в целом принимать трафик на основании результатов проверок
	Ready bool
	// AvailabilityResults содержит результаты проверок доступности внешних систем
	AvailabilityResults []CheckResult
}

// CheckResult описывает доступность одной внешней системы и причину отказа.
type CheckResult struct {
	// System содержит название системы, которая была проверена (например, "postgres")
	System string
	// IsAvailable показывает доступность конкретной внешней системы (в нашем случае это только база данных)
	IsAvailable bool
	// Error содержит информацию об ошибке, если проверка не была успешной
	Error error
}

// Application проверяет доступность настроенного соединения с хранилищем.
type Application struct {
	db dbPinger
}

// NewApplication создаёт проверку доступности через переданную операцию Ping.
func NewApplication(db dbPinger) *Application {
	return &Application{db: db}
}

// CheckAvailability выполняет Ping и возвращает готовность вместе с причиной ошибки.
func (s *Application) CheckAvailability(ctx context.Context) AvailabilityResult {
	err := s.db.Ping(ctx)
	result := AvailabilityResult{
		Ready:               err == nil,
		AvailabilityResults: []CheckResult{{System: "postgres", IsAvailable: err == nil, Error: err}},
	}
	return result
}
