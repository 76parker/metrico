package metrics

import "errors"

var (
	// ErrMetricNotFound означает, что метрики с указанным именем нет в хранилище.
	ErrMetricNotFound = errors.New("metric not found")
	// ErrMetricNameIsEmpty означает, что имя метрики не задано.
	ErrMetricNameIsEmpty = errors.New("metric name is empty")
	// ErrMetricTypeIsEmpty означает, что тип метрики не задан.
	ErrMetricTypeIsEmpty = errors.New("metric type is empty")
	// ErrInvalidMetricType означает, что тип метрики не поддерживается.
	ErrInvalidMetricType = errors.New("invalid metric type")
	// ErrInvalidValueForCounter означает, что значение counter не разбирается как int64.
	ErrInvalidValueForCounter = errors.New("invalid value for counter metric")
	// ErrInvalidValueForGauge означает, что значение gauge не разбирается как float64.
	ErrInvalidValueForGauge = errors.New("invalid value for gauge metric")

	// ErrGaugeValueIsNil означает отсутствие значения Value у gauge-метрики.
	ErrGaugeValueIsNil = errors.New("gauge value is nil")
	// ErrCounterValueIsNil означает отсутствие значения Delta у counter-метрики.
	ErrCounterValueIsNil = errors.New("counter value is nil")
)
