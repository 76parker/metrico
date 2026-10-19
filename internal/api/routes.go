// Package api собирает HTTP-маршруты сервера метрик и диагностики.
package api

import (
	"net/http"

	"github.com/76Parker/metrico/internal/api/handlers"
	"github.com/76Parker/metrico/internal/api/middleware"
	"github.com/76Parker/metrico/internal/config"
	"github.com/76Parker/metrico/pkg/logger"
	"github.com/gin-gonic/gin"
)

// NewRouter создаёт HTTP-сервер с маршрутами метрик и проверки доступности.
// Сервер ещё не слушает порт. Шаблоны загружаются из templates/* относительно
// текущего каталога; при отсутствии шаблонов Gin вызывает panic.
func NewRouter(
	metricsHandler *handlers.MetricsHandler,
	healthHandler *handlers.HealthHandler,
	httpCfg config.HTTP,
	log logger.Logger,
) *http.Server {
	router := registerHTTPRoutes(metricsHandler, healthHandler, log)
	return newHTTPServer(httpCfg, router)
}

func newHTTPServer(cfg config.HTTP, router *gin.Engine) *http.Server {
	return &http.Server{
		Addr:              cfg.Address,
		Handler:           middleware.VerifyAndSign(router, cfg.HashKey),
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
		IdleTimeout:       cfg.IdleTimeout,
	}
}

func registerHTTPRoutes(
	metricsHandler *handlers.MetricsHandler,
	healthHandler *handlers.HealthHandler,
	log logger.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	registerMetricsRoutes(router, metricsHandler, log)
	registerHealthRoutes(router, healthHandler)
	return router
}

func registerMetricsRoutes(router *gin.Engine, handler *handlers.MetricsHandler, log logger.Logger) {
	logMW := middleware.WithLogging(log)
	compressMW := middleware.Compress()
	handlerName := middleware.HandlerName
	errHandlerMW := middleware.ErrorHandler()

	router.LoadHTMLGlob("templates/*")
	router.POST("/update/:metricType/:metricName/:metricValue", logMW, compressMW, errHandlerMW, handlerName("update_metric"), handler.Update)
	registerPostRoutes(router, []string{"/value", "/value/"}, logMW, compressMW, errHandlerMW, handlerName("get_metric_json"), handler.GetFromJSON)
	registerPostRoutes(router, []string{"/update", "/update/"}, logMW, compressMW, errHandlerMW, handlerName("update_metric_json"), handler.UpdateFromJSON)
	registerPostRoutes(router, []string{"/updates", "/updates/"}, logMW, compressMW, errHandlerMW, handlerName("batch_update_metrics"), handler.BatchUpdateFromJSON)
	router.GET("/value/:metricType/:metricName", logMW, compressMW, errHandlerMW, handlerName("get_metric"), handler.GetByName)
	router.GET("/", logMW, compressMW, errHandlerMW, handlerName("get_all_metrics"), handler.GetAll)
}

func registerPostRoutes(router *gin.Engine, paths []string, handlers ...gin.HandlerFunc) {
	for _, path := range paths {
		router.POST(path, handlers...)
	}
}

func registerHealthRoutes(router *gin.Engine, handler *handlers.HealthHandler) {
	router.GET("/ping", handler.CheckAvailability)
}
