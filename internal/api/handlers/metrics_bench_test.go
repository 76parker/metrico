package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
)

type benchmarkMetricsApplication struct {
	metricsApplication
	size int
}

func (a benchmarkMetricsApplication) BatchUpdate(_ context.Context, command metricsapp.BatchUpdateCommand) error {
	if len(command) != a.size {
		return fmt.Errorf("batch size = %d, want %d", len(command), a.size)
	}
	return nil
}

func BenchmarkBatchUpdateFromJSON(b *testing.B) {
	gin.SetMode(gin.TestMode)
	for _, size := range []int{1, 50, 500} {
		b.Run(fmt.Sprintf("metrics=%d", size), func(b *testing.B) {
			batch := make([]updateRequest, size)
			for i := range batch {
				batch[i] = updateRequest{Name: fmt.Sprintf("metric_%04d", i), MetricType: "gauge", Value: json.RawMessage(`42.5`)}
			}
			body, err := json.Marshal(batch)
			if err != nil {
				b.Fatal(err)
			}
			router := gin.New()
			handler := NewMetricsHandler(benchmarkMetricsApplication{size: size})
			router.POST("/updates/", handler.BatchUpdateFromJSON)
			request := httptest.NewRequest(http.MethodPost, "/updates/", nil)
			request.Header.Set("Content-Type", "application/json")
			reader := bytes.NewReader(body)
			requestBody := io.NopCloser(reader)
			b.ReportAllocs()
			for b.Loop() {
				reader.Reset(body)
				request.Body = requestBody
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					b.Fatalf("status = %d", response.Code)
				}
			}
		})
	}
}
