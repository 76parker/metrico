// Package reporter подготавливает и отправляет пакеты метрик HTTP-серверу.
package reporter

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/76Parker/metrico/internal/domain/metrics"
	"github.com/76Parker/metrico/internal/signature"
	goccyjson "github.com/goccy/go-json"
)

// gaugeMetricNames содержит список имен Gauge-метрик которые необходимо отправлять на сервер
var gaugeMetricNames = []string{
	"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys", "HeapAlloc",
	"HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased", "HeapSys", "LastGC",
	"Lookups", "MCacheInuse", "MCacheSys", "MSpanInuse", "MSpanSys", "Mallocs",
	"NextGC", "NumForcedGC", "NumGC", "OtherSys", "PauseTotalNs", "StackInuse",
	"StackSys", "Sys", "TotalAlloc", "RandomValue",
}

type metricProvider interface {
	Metrics() (gaugeMetrics runtime.MemStats, pollCount int64)
}

type systemMetricProvider interface {
	Metrics() []metrics.Metrics
}

// MetricReporter периодически отправляет runtime- и системные метрики пакетами по HTTP.
type MetricReporter struct {
	client         *http.Client
	key            string
	reportInterval time.Duration
	url            *url.URL
	provider       metricProvider
	systemProvider systemMetricProvider
	rateLimit      int
}

// Option конфигурирует MetricReporter.
type Option func(*MetricReporter)

// WithHTTPClient устанавливает HTTP-клиент.
func WithHTTPClient(client *http.Client) Option {
	return func(reporter *MetricReporter) {
		reporter.client = client
	}
}

// WithMetricProvider устанавливает источник runtime-метрик.
func WithMetricProvider(provider metricProvider) Option {
	return func(reporter *MetricReporter) {
		reporter.provider = provider
	}
}

// WithReportInterval устанавливает интервал отправки метрик.
func WithReportInterval(reportInterval time.Duration) Option {
	return func(reporter *MetricReporter) {
		reporter.reportInterval = reportInterval
	}
}

// WithKey устанавливает ключ подписи запросов.
func WithKey(key string) Option {
	return func(reporter *MetricReporter) {
		reporter.key = key
	}
}

// WithRateLimit устанавливает число параллельных отправителей.
func WithRateLimit(rateLimit int) Option {
	return func(reporter *MetricReporter) {
		reporter.rateLimit = rateLimit
	}
}

// WithSystemMetricProvider устанавливает источник системных метрик.
func WithSystemMetricProvider(provider systemMetricProvider) Option {
	return func(reporter *MetricReporter) {
		reporter.systemProvider = provider
	}
}

// NewMetricReporter разбирает адрес сервера и применяет опции отправки.
// При ошибке разбора URL возвращает nil. Для Run задайте HTTP-клиент, источник
// runtime-метрик и положительный интервал соответствующими опциями.
func NewMetricReporter(
	serverAddr string,
	options ...Option,
) *MetricReporter {
	baseURL, err := url.Parse(serverAddr)
	if err != nil {
		return nil
	}
	reporter := &MetricReporter{
		url:       baseURL,
		rateLimit: 1,
	}
	for _, option := range options {
		option(reporter)
	}
	if reporter.rateLimit <= 0 {
		reporter.rateLimit = 1
	}
	return reporter
}

// Run отправляет метрики до отмены ctx и ожидает завершения отправителей.
// Возвращает ошибку контекста при остановке; ошибки отдельных отправок журналируются.
// Для ограничения ожидания текущих HTTP-запросов настройте таймаут клиента.
func (r *MetricReporter) Run(ctx context.Context) error {
	reportTicker := time.NewTicker(r.reportInterval)
	defer reportTicker.Stop()
	jobs := make(chan []metrics.Metrics, r.rateLimit)
	var workers sync.WaitGroup
	for i := 0; i < r.rateLimit; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for batch := range jobs {
				if err := r.sendBatch(batch); err != nil {
					log.Print(err)
				}
			}
		}()
	}
	defer func() {
		close(jobs)
		workers.Wait()
	}()

	for {
		select {
		case <-reportTicker.C:
			runtimeMetrics, pollCount := r.provider.Metrics()
			batch := r.metricsBatch(runtimeMetrics, pollCount)
			if r.systemProvider != nil {
				batch = append(batch, r.systemProvider.Metrics()...)
			}
			select {
			case jobs <- batch:
			case <-ctx.Done():
				return ctx.Err()
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (r *MetricReporter) sendMetrics(runtimeMetrics runtime.MemStats, pollCount int64) error {
	return r.sendBatch(r.metricsBatch(runtimeMetrics, pollCount))
}

func (r *MetricReporter) metricsBatch(runtimeMetrics runtime.MemStats, pollCount int64) []metrics.Metrics {
	v := reflect.ValueOf(runtimeMetrics)
	t := v.Type()
	metricsBatch := make([]metrics.Metrics, 0, len(gaugeMetricNames)+1)

	// Собирает runtime-метрики из MemStats
	for i := 0; i < v.NumField(); i++ {
		fieldName := t.Field(i).Name // Имя поля берем из типа
		fieldVal := v.Field(i)       // Значение поля берем из value
		for _, gaugeMetricName := range gaugeMetricNames {
			if fieldName == gaugeMetricName {
				var metricValue float64
				switch fieldVal.Kind() {
				case reflect.Uint64, reflect.Uint32:
					metricValue = float64(fieldVal.Uint())
				case reflect.Float64:
					metricValue = fieldVal.Float()
				}
				metricsBatch = append(metricsBatch, newGaugeMetric(metricValue, fieldName))
			}
		}
	}
	// Добавляем одну Counter-метрику - PollCount
	metricsBatch = append(metricsBatch, newCounterMetric(pollCount, "PollCount"))
	// Добавляем кастомную Gauge-метрику - RandomValue
	metricsBatch = append(metricsBatch, newGaugeMetric(rand.Float64(), "RandomValue"))

	return metricsBatch
}

func newGaugeMetric(metricValue float64, metricName string) metrics.Metrics {
	return metrics.Metrics{
		ID:    metricName,
		Type:  metrics.MetricTypeGauge,
		Value: &metricValue,
	}
}

func newCounterMetric(metricValue int64, metricName string) metrics.Metrics {
	return metrics.Metrics{
		ID:    metricName,
		Type:  metrics.MetricTypeCounter,
		Delta: &metricValue,
	}
}

func (r *MetricReporter) sendBatch(metricsBatch []metrics.Metrics) error {
	body, err := goccyjson.Marshal(metricsBatch)
	if err != nil {
		return fmt.Errorf("marshal metric batch: %w", err)
	}

	var response *http.Response
	defer func() {
		if response != nil {
			response.Body.Close()
		}
	}()
	err = withRetry(func() error {
		request, err := http.NewRequest(http.MethodPost, r.batchUpdateURL(), bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create batch update request: %w", err)
		}
		request.Header.Set("Content-Type", "application/json")
		if r.key != "" {
			request.Header.Set(signature.HeaderName, signature.Sum(body, r.key))
		}

		resp, err := r.client.Do(request)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		response = resp
		return nil
	})
	if err != nil {
		return err
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return fmt.Errorf("parse response content type: %w", err)
	}
	if contentType != "application/json" {
		return fmt.Errorf("unexpected response content type: %s", contentType)
	}

	return nil
}

func (r *MetricReporter) batchUpdateURL() string {
	requestURL := *r.url
	requestURL.Path = "/updates"
	requestURL.RawQuery = ""
	return requestURL.String()
}
