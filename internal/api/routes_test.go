package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/76Parker/metrico/internal/api/handlers"
	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
	"github.com/76Parker/metrico/internal/config"
	domainmetrics "github.com/76Parker/metrico/internal/domain/metrics"
	"github.com/76Parker/metrico/internal/signature"
	"github.com/76Parker/metrico/pkg/logger"
	"github.com/gin-gonic/gin"
)

func TestNewHTTPServer_WithHashKey(t *testing.T) {
	const key = "test-key"
	requestBody := []byte(`{"id":"Alloc"}`)
	responseBody := []byte(`{"ok":true}`)
	router := gin.New()
	router.POST("/updates", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", responseBody)
	})
	server := newHTTPServer(config.HTTP{HashKey: key}, router)
	request := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(requestBody))
	request.Header.Set(signature.HeaderName, signature.Sum(requestBody, key))
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get(signature.HeaderName); got != signature.Sum(responseBody, key) {
		t.Fatalf("response hash = %q, want %q", got, signature.Sum(responseBody, key))
	}
}

func TestMetricsJSONRoutes_AcceptTrailingSlash(t *testing.T) {
	const key = "test-key"
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatalf("change to project root: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(workingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	router := gin.New()
	registerMetricsRoutes(router, handlers.NewMetricsHandler(testMetricsApplication{}), logger.NewMockLogger())
	server := newHTTPServer(config.HTTP{HashKey: key}, router)

	for _, tt := range []struct {
		name string
		path string
		body string
	}{
		{name: "update", path: "/update/", body: `{"id":"test","type":"gauge","value":1}`},
		{name: "get", path: "/value/", body: `{"id":"test","type":"gauge"}`},
		{name: "batch update", path: "/updates/", body: `[{"id":"test","type":"gauge","value":1}]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(tt.body))
			request.Header.Set("Accept-Encoding", "gzip")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			server.Handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if location := response.Header().Get("Location"); location != "" {
				t.Fatalf("unexpected redirect to %q", location)
			}
		})
	}
}

type testMetricsApplication struct{}

func (testMetricsApplication) Update(context.Context, metricsapp.UpdateCommand) error {
	return nil
}

func (testMetricsApplication) BatchUpdate(context.Context, metricsapp.BatchUpdateCommand) error {
	return nil
}

func (testMetricsApplication) GetByName(context.Context, string) (domainmetrics.Metrics, error) {
	value := 1.0
	return domainmetrics.Metrics{
		ID:    "test",
		Type:  domainmetrics.MetricTypeGauge,
		Value: &value,
	}, nil
}

func (testMetricsApplication) GetAll(context.Context) ([]domainmetrics.Metrics, error) {
	return nil, nil
}
