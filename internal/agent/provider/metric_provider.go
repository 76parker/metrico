// Package provider собирает runtime- и системные метрики агента.
package provider

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// MetricProvider собирает runtime-метрики и число опросов с синхронизацией доступа.
// Содержит мьютекс и не должен копироваться после использования.
type MetricProvider struct {
	metrics           runtime.MemStats
	mu                sync.Mutex
	pollCount         int64
	previousPollCount int64
	pollInterval      time.Duration
}

// NewMetricProvider запускает сбор runtime-метрик с заданным положительным интервалом.
// Сбор работает до завершения процесса; для управления остановкой используйте
// NewMetricProviderWithContext.
func NewMetricProvider(pollInterval time.Duration) *MetricProvider {
	return NewMetricProviderWithContext(context.Background(), pollInterval)
}

// NewMetricProviderWithContext запускает сбор runtime-метрик до отмены ctx.
// pollInterval должен быть положительным.
func NewMetricProviderWithContext(ctx context.Context, pollInterval time.Duration) *MetricProvider {

	p := &MetricProvider{
		pollInterval: pollInterval,
	}
	p.Start(ctx)
	return p
}

// Start запускает неблокирующий цикл сбора до отмены ctx.
// Конструктор уже запускает цикл; повторный вызов создаёт дополнительный цикл.
func (mp *MetricProvider) Start(ctx context.Context) {
	timer := time.NewTicker(mp.pollInterval)
	go func() {
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				mp.mu.Lock()
				mp.pollCount++
				runtime.ReadMemStats(&mp.metrics)
				mp.mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Metrics возвращает последний снимок runtime-метрик и число опросов с прошлого вызова.
// Повторное чтение без нового сбора возвращает нулевое приращение PollCount.
func (mp *MetricProvider) Metrics() (runtime.MemStats, int64) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if mp.pollCount == mp.previousPollCount {
		return mp.metrics, 0
	}
	pollCount := mp.pollCount - mp.previousPollCount
	mp.previousPollCount = mp.pollCount
	return mp.metrics, pollCount
}
