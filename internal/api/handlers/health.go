package handlers

import (
	"context"

	"github.com/76Parker/metrico/internal/applications/health"
	"github.com/76Parker/metrico/pkg/logger"
	"github.com/gin-gonic/gin"
)

type healthService interface {
	CheckAvailability(ctx context.Context) health.AvailabilityResult
}

// HealthHandler преобразует результат проверки доступности приложения в HTTP-статус.
type HealthHandler struct {
	log     logger.Logger
	service healthService
}

// NewHealthHandler создаёт обработчик с проверкой доступности и журналом ошибок.
func NewHealthHandler(log logger.Logger, service healthService) *HealthHandler {
	return &HealthHandler{log: log, service: service}
}

// CheckAvailability обрабатывает GET /ping: возвращает HTTP 200 при готовности
// приложения или HTTP 500 при недоступности зависимости. Тело ответа пустое.
func (h *HealthHandler) CheckAvailability(c *gin.Context) {
	ctx := c.Request.Context()
	result := h.service.CheckAvailability(ctx)
	if result.Ready {
		c.Status(200)
		return
	}
	for _, check := range result.AvailabilityResults {
		if !check.IsAvailable {
			h.log.Error("System is not available", "system", check.System, "error", check.Error)
		}
	}
	c.Status(500)
}
