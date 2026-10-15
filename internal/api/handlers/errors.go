package handlers

import "errors"

var (
	// ErrFieldTooLong означает превышение допустимой длины параметра метрики.
	ErrFieldTooLong = errors.New("field is too long")
	// ErrInvalidJSON означает ошибку декодирования JSON-тела запроса.
	ErrInvalidJSON = errors.New("invalid input json")
	// ErrMetricTypeNotFound означает неизвестный тип метрики в маршруте чтения.
	ErrMetricTypeNotFound = errors.New("metric type not found")
)
