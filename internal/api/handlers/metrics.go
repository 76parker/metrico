// Package handlers реализует HTTP-обработчики метрик и проверки доступности.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	goccyjson "github.com/goccy/go-json"

	"github.com/76Parker/metrico/internal/applications/audit"
	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
	"github.com/76Parker/metrico/internal/domain/metrics"
)

const batchReservationSize = 50

type metricsApplication interface {
	Update(ctx context.Context, cmd metricsapp.UpdateCommand) error
	BatchUpdate(ctx context.Context, cmd metricsapp.BatchUpdateCommand) error
	GetByName(ctx context.Context, name string) (metrics.Metrics, error)
	GetAll(ctx context.Context) ([]metrics.Metrics, error)
}

// MetricsHandler обслуживает HTTP-запросы чтения и изменения метрик.
// Создавайте обработчик через NewMetricsHandler и подключайте ErrorHandler
// для преобразования ошибок приложения в HTTP-ответы.
type MetricsHandler struct {
	metricSvc       metricsApplication
	maxPathParamLen int
	auditPublisher  auditPublisher
}

type auditPublisher interface {
	Notify(ctx context.Context, event audit.Event)
}

// MetricsHandlerOption configures optional metrics handler behavior.
type MetricsHandlerOption func(*MetricsHandler)

// WithAuditPublisher enables notifications about successfully received metrics.
func WithAuditPublisher(publisher auditPublisher) MetricsHandlerOption {
	return func(handler *MetricsHandler) {
		handler.auditPublisher = publisher
	}
}

// NewMetricsHandler creates a metrics handler with optional audit publishing.
func NewMetricsHandler(metricSvc metricsApplication, options ...MetricsHandlerOption) *MetricsHandler {
	handler := &MetricsHandler{
		metricSvc:       metricSvc,
		maxPathParamLen: 64,
	}
	for _, option := range options {
		option(handler)
	}
	return handler
}

// Update обрабатывает POST /update/:metricType/:metricName/:metricValue.
// Gauge заменяет сохранённое значение, counter прибавляет переданное приращение.
// При успехе возвращает HTTP 200 без тела; ошибки передаёт middleware ErrorHandler.
func (h *MetricsHandler) Update(c *gin.Context) {
	params := [3]string{
		strings.TrimSpace(c.Param("metricType")),
		strings.TrimSpace(c.Param("metricName")),
		strings.TrimSpace(c.Param("metricValue")),
	}
	for _, param := range params {
		if utf8.RuneCountInString(param) > h.maxPathParamLen {
			c.Error(ErrFieldTooLong)
			return
		}
	}
	cmd := metricsapp.UpdateCommand{
		Name:       params[1],
		MetricType: params[0],
		Value:      params[2],
	}
	if err := h.metricSvc.Update(c.Request.Context(), cmd); err != nil {
		c.Error(err)
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	h.notifyAudit(c, []string{cmd.Name})
	c.Status(http.StatusOK)
}

type updateRequest struct {
	Name       string          `json:"id"`
	MetricType string          `json:"type"`
	Value      json.RawMessage `json:"value"`
	Delta      json.RawMessage `json:"delta"`
}

func (r updateRequest) toCommand() metricsapp.UpdateCommand {
	rawValue := r.Value
	if r.MetricType == string(metrics.MetricTypeCounter) {
		rawValue = r.Delta
	}

	return metricsapp.UpdateCommand{
		Name:       r.Name,
		MetricType: r.MetricType,
		Value:      string(rawValue),
	}
}

// UpdateFromJSON обрабатывает POST /update/ с JSON-описанием одной метрики.
// Для gauge используется поле value, для counter — delta; успешный ответ
// содержит обновлённую метрику в JSON. Тело запроса ограничено 1 MiB.
func (h *MetricsHandler) UpdateFromJSON(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := goccyjson.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var req updateRequest
	if err := decoder.Decode(&req); err != nil {
		c.Error(ErrInvalidJSON)
		return
	}
	cmd := req.toCommand()
	if err := h.metricSvc.Update(c.Request.Context(), cmd); err != nil {
		c.Error(err)
		return
	}
	updatedMetric, err := h.metricSvc.GetByName(c.Request.Context(), cmd.Name)
	if err != nil {
		c.Error(err)
		return
	}

	response, err := goccyjson.Marshal(updatedMetric)
	if err != nil {
		c.Error(fmt.Errorf("marshal updated metric: %w", err))
		return
	}
	h.notifyAudit(c, []string{cmd.Name})
	c.Data(http.StatusOK, "application/json", response)
}

// BatchUpdateFromJSON обрабатывает POST /updates/ с JSON-массивом метрик.
// При успехе возвращает HTTP 200 без тела. Неизвестные поля JSON отклоняются,
// размер тела ограничен 1 MiB; ошибки передаются middleware ErrorHandler.
func (h *MetricsHandler) BatchUpdateFromJSON(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := goccyjson.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()

	batchRequest := make([]updateRequest, 0, batchReservationSize)
	if err := decoder.Decode(&batchRequest); err != nil {
		c.Error(ErrInvalidJSON)
		return
	}
	batchCommand := make(metricsapp.BatchUpdateCommand, 0, len(batchRequest))
	for _, req := range batchRequest {
		batchCommand = append(batchCommand, req.toCommand())
	}
	if err := h.metricSvc.BatchUpdate(c.Request.Context(), batchCommand); err != nil {
		c.Error(err)
		return
	}
	if h.auditPublisher != nil && len(batchCommand) > 0 {
		metricNames := make([]string, 0, len(batchCommand))
		for _, command := range batchCommand {
			metricNames = append(metricNames, command.Name)
		}
		h.notifyAudit(c, metricNames)
	}
	c.Status(http.StatusOK)
}

func (h *MetricsHandler) notifyAudit(c *gin.Context, metricNames []string) {
	if h.auditPublisher == nil {
		return
	}
	ipAddress, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		ipAddress = c.Request.RemoteAddr
	}
	h.auditPublisher.Notify(c.Request.Context(), audit.Event{
		TS:        time.Now().Unix(),
		Metrics:   metricNames,
		IPAddress: ipAddress,
	})
}

// GetFromJSON обрабатывает POST /value/ и возвращает метрику по полю id в JSON.
// Размер тела ограничен 1 MiB; ошибки чтения и отсутствия метрики передаются
// middleware ErrorHandler.
func (h *MetricsHandler) GetFromJSON(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := goccyjson.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()

	var request metrics.Metrics
	if err := decoder.Decode(&request); err != nil {
		c.Error(ErrInvalidJSON)
		return
	}
	request.ID = strings.TrimSpace(request.ID)
	metric, err := h.metricSvc.GetByName(c.Request.Context(), request.ID)
	if err != nil {
		c.Error(err)
		return
	}
	response, err := goccyjson.Marshal(metric)
	if err != nil {
		c.Error(fmt.Errorf("marshal metric: %w", err))
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", response)
}

// GetByName обрабатывает GET /value/:metricType/:metricName.
// Возвращает числовое значение метрики как text/plain; ошибки передаёт ErrorHandler.
func (h *MetricsHandler) GetByName(c *gin.Context) {
	metricName := strings.TrimSpace(c.Param("metricName"))
	metricType := strings.TrimSpace(c.Param("metricType"))
	metric, err := h.metricSvc.GetByName(c.Request.Context(), metricName)
	if err != nil {
		c.Error(err)
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	switch metricType {
	case "counter":
		c.String(http.StatusOK, "%d", *metric.Delta)
	case "gauge":
		c.String(http.StatusOK, "%g", *metric.Value)
	default:
		c.Error(ErrMetricTypeNotFound)
	}
}

type metricResponse struct {
	Name  string
	Value any
}

// GetAll обрабатывает GET / и формирует HTML-список метрик, отсортированный по имени.
// Для отображения использует предварительно загруженный шаблон index.tmpl.
func (h *MetricsHandler) GetAll(c *gin.Context) {
	metricsSnapshot, err := h.metricSvc.GetAll(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	resp := make([]metricResponse, 0, len(metricsSnapshot))
	for _, metric := range metricsSnapshot {
		switch metric.Type {
		case metrics.MetricTypeGauge:
			resp = append(resp, metricResponse{
				Name:  metric.ID,
				Value: *metric.Value,
			})
		case metrics.MetricTypeCounter:
			resp = append(resp, metricResponse{
				Name:  metric.ID,
				Value: *metric.Delta,
			})
		}
	}
	// Так как map не гарантирует порядок, сортируем срез по имени для одинаково ответа
	// Без сортировки значения всегда в разном порядке
	sort.Slice(resp, func(i, j int) bool {
		return resp[i].Name < resp[j].Name
	})
	c.HTML(http.StatusOK, "index.tmpl", resp)
}
