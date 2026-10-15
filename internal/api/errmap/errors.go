// Package errmap сопоставляет ошибки приложения с публичными HTTP-ответами.
package errmap

import (
	"errors"
	"net/http"

	"github.com/76Parker/metrico/internal/api/handlers"
	"github.com/76Parker/metrico/internal/domain/metrics"
)

type registry map[error]Error

// Registry сопоставляет ошибки обработчиков и метрик с публичными HTTP-ошибками.
// Реестр используется только для чтения во время обработки запросов.
var Registry = registry{
	// Transport-specific errors
	handlers.ErrFieldTooLong:       newError("metric value is too long", CodeInvalidRequest, http.StatusBadRequest),
	handlers.ErrInvalidJSON:        newError("invalid input json", CodeInvalidJSON, http.StatusBadRequest),
	handlers.ErrMetricTypeNotFound: newError("metric type not found", CodeNotFound, http.StatusNotFound),

	// Metrics domain-specific errors
	metrics.ErrMetricNotFound:         newError("metric not found", CodeNotFound, http.StatusNotFound),
	metrics.ErrMetricNameIsEmpty:      newError("metric name is empty", CodeEmpty, http.StatusNotFound),
	metrics.ErrInvalidMetricType:      newError("invalid metric type", CodeInvalidMetricType, http.StatusBadRequest),
	metrics.ErrInvalidValueForCounter: newError("invalid value for counter metric", CodeInvalidMetricValue, http.StatusBadRequest),
	metrics.ErrInvalidValueForGauge:   newError("invalid value for gauge metric", CodeInvalidMetricValue, http.StatusBadRequest),
	metrics.ErrGaugeValueIsNil:        newError("gauge value is nil", CodeInvalidMetricValue, http.StatusBadRequest),
	metrics.ErrCounterValueIsNil:      newError("counter value is nil", CodeInvalidMetricValue, http.StatusBadRequest),
}

// UnexpectedError описывает ответ HTTP 500 для ошибки, отсутствующей в Registry.
var UnexpectedError = newError("unexpected error", CodeInternal, http.StatusInternalServerError)

// Resolve находит HTTP-ошибку через errors.Is, включая обёрнутые ошибки.
// Второе значение равно false, если в реестре нет соответствия.
func (r registry) Resolve(err error) (Error, bool) {
	for target, resolved := range r {
		if errors.Is(err, target) {
			return resolved, true
		}
	}
	return Error{}, false
}

// Error описывает JSON-ответ об ошибке с публичным сообщением, кодом и HTTP-статусом.
type Error struct {
	// Message содержит публичное описание ошибки для клиента.
	Message string `json:"message"`
	// Code содержит машиночитаемый код ошибки.
	Code string `json:"code"`
	// Status содержит HTTP-статус ответа.
	Status int `json:"status"`
}

func newError(message, code string, status int) Error {
	return Error{
		Message: message,
		Code:    code,
		Status:  status,
	}
}
