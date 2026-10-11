package handlers_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/76Parker/metrico/internal/api/handlers"
	"github.com/76Parker/metrico/internal/api/middleware"
	"github.com/76Parker/metrico/internal/applications/audit"
	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
	domainmetrics "github.com/76Parker/metrico/internal/domain/metrics"
	"github.com/76Parker/metrico/pkg/logger"
)

type auditPublisherFunc func(context.Context, audit.Event)

func (publish auditPublisherFunc) Notify(ctx context.Context, event audit.Event) {
	publish(ctx, event)
}

func createAuditTestRouter(handler *handlers.MetricsHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	errHandler := middleware.ErrorHandler()
	router.POST("/update/:metricType/:metricName/:metricValue", errHandler, handler.Update)
	for _, path := range []string{"/update", "/update/"} {
		router.POST(path, errHandler, handler.UpdateFromJSON)
	}
	for _, path := range []string{"/updates", "/updates/"} {
		router.POST(path, errHandler, handler.BatchUpdateFromJSON)
	}
	router.POST("/value", errHandler, handler.GetFromJSON)
	return router
}

func TestMetricsHandler_Audit_ReportsAcceptedMetrics(t *testing.T) {
	type auditCase struct {
		name       string
		path       string
		body       string
		remoteAddr string
		expectedIP string
		metrics    []string
		configure  func(*handlers.MockMetricsApplication)
	}
	cases := []auditCase{
		{
			name: "path update trims metric name",
			path: "/update/gauge/%20Alloc%20/1.5", remoteAddr: "192.168.0.42:1234", expectedIP: "192.168.0.42",
			metrics: []string{"Alloc"},
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().Update(gomock.Any(), metricsapp.UpdateCommand{
					Name: "Alloc", MetricType: "gauge", Value: "1.5",
				}).Return(nil)
			},
		},
		{
			name: "single json with ipv6",
			path: "/update", body: `{"id":"Alloc","type":"gauge","value":1.5}`,
			remoteAddr: "[2001:db8::42]:1234", expectedIP: "2001:db8::42", metrics: []string{"Alloc"},
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().Update(gomock.Any(), metricsapp.UpdateCommand{
					Name: "Alloc", MetricType: "gauge", Value: "1.5",
				}).Return(nil)
				service.EXPECT().GetByName(gomock.Any(), "Alloc").Return(domainmetrics.Metrics{
					ID: "Alloc", Type: domainmetrics.MetricTypeGauge, Value: float64Pointer(1.5),
				}, nil)
			},
		},
		{
			name: "single json with trailing slash",
			path: "/update/", body: `{"id":"Frees","type":"counter","delta":2}`,
			remoteAddr: "127.0.0.1:1234", expectedIP: "127.0.0.1", metrics: []string{"Frees"},
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().Update(gomock.Any(), metricsapp.UpdateCommand{
					Name: "Frees", MetricType: "counter", Value: "2",
				}).Return(nil)
				service.EXPECT().GetByName(gomock.Any(), "Frees").Return(domainmetrics.Metrics{
					ID: "Frees", Type: domainmetrics.MetricTypeCounter, Delta: int64Pointer(2),
				}, nil)
			},
		},
	}
	for _, path := range []string{"/updates", "/updates/"} {
		cases = append(cases, auditCase{
			name: "batch " + path, path: path,
			body: `[{"id":"Alloc","type":"gauge","value":1.5},` +
				`{"id":"Frees","type":"counter","delta":2},{"id":"Frees","type":"counter","delta":3}]`,
			remoteAddr: "192.168.0.42:1234", expectedIP: "192.168.0.42", metrics: []string{"Alloc", "Frees", "Frees"},
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().BatchUpdate(gomock.Any(), metricsapp.BatchUpdateCommand{
					{Name: "Alloc", MetricType: "gauge", Value: "1.5"},
					{Name: "Frees", MetricType: "counter", Value: "2"},
					{Name: "Frees", MetricType: "counter", Value: "3"},
				}).Return(nil)
			},
		})
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service := newMockMetricsApplication(t)
			testCase.configure(service)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var events []audit.Event
			publisher := auditPublisherFunc(func(notifyCtx context.Context, event audit.Event) {
				require.Equal(t, ctx, notifyCtx)
				events = append(events, event)
			})
			handler := handlers.NewMetricsHandler(service, handlers.WithAuditPublisher(publisher))
			request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body)).WithContext(ctx)
			request.RemoteAddr = testCase.remoteAddr
			request.Header.Set("X-Forwarded-For", "203.0.113.99")
			request.Header.Set("X-Real-IP", "203.0.113.98")
			response := httptest.NewRecorder()
			before := time.Now().Unix()
			createAuditTestRouter(handler).ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			require.Len(t, events, 1)
			require.Equal(t, testCase.metrics, events[0].Metrics)
			require.Equal(t, testCase.expectedIP, events[0].IPAddress)
			require.GreaterOrEqual(t, events[0].TS, before)
			require.LessOrEqual(t, events[0].TS, time.Now().Unix())
		})
	}
}

func TestMetricsHandler_Audit_SkipsUnsuccessfulAndEmptyRequests(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	cases := []struct {
		name      string
		path      string
		body      string
		status    int
		configure func(*handlers.MockMetricsApplication)
	}{
		{name: "field too long", path: "/update/gauge/" + strings.Repeat("a", 65) + "/1", status: http.StatusBadRequest},
		{
			name: "path update storage error", path: "/update/gauge/Alloc/1", status: http.StatusInternalServerError,
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().Update(gomock.Any(), gomock.Any()).Return(storageErr)
			},
		},
		{
			name: "invalid metric value", path: "/update/gauge/Alloc/invalid", status: http.StatusBadRequest,
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().Update(gomock.Any(), gomock.Any()).Return(domainmetrics.ErrInvalidValueForGauge)
			},
		},
		{name: "invalid single json", path: "/update", body: "{", status: http.StatusBadRequest},
		{name: "invalid batch json", path: "/updates", body: "{", status: http.StatusBadRequest},
		{
			name: "json update storage error", path: "/update", body: `{"id":"Alloc","type":"gauge","value":1}`,
			status: http.StatusInternalServerError,
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().Update(gomock.Any(), gomock.Any()).Return(storageErr)
			},
		},
		{
			name: "json response read error", path: "/update", body: `{"id":"Alloc","type":"gauge","value":1}`,
			status: http.StatusInternalServerError,
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
				service.EXPECT().GetByName(gomock.Any(), "Alloc").Return(domainmetrics.Metrics{}, storageErr)
			},
		},
		{
			name: "json response marshal error", path: "/update", body: `{"id":"Alloc","type":"gauge","value":1}`,
			status: http.StatusInternalServerError,
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
				service.EXPECT().GetByName(gomock.Any(), "Alloc").Return(domainmetrics.Metrics{
					ID: "Alloc", Type: domainmetrics.MetricTypeGauge, Value: float64Pointer(math.NaN()),
				}, nil)
			},
		},
		{
			name: "batch storage error", path: "/updates", body: `[{"id":"Alloc","type":"gauge","value":1}]`,
			status: http.StatusInternalServerError,
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().BatchUpdate(gomock.Any(), gomock.Any()).Return(storageErr)
			},
		},
		{
			name: "empty batch", path: "/updates", body: "[]", status: http.StatusOK,
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().BatchUpdate(gomock.Any(), metricsapp.BatchUpdateCommand{}).Return(nil)
			},
		},
		{
			name: "metric read", path: "/value", body: `{"id":"Alloc","type":"gauge"}`, status: http.StatusOK,
			configure: func(service *handlers.MockMetricsApplication) {
				service.EXPECT().GetByName(gomock.Any(), "Alloc").Return(domainmetrics.Metrics{
					ID: "Alloc", Type: domainmetrics.MetricTypeGauge, Value: float64Pointer(1),
				}, nil)
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service := newMockMetricsApplication(t)
			if testCase.configure != nil {
				testCase.configure(service)
			}
			var events []audit.Event
			publisher := auditPublisherFunc(func(_ context.Context, event audit.Event) {
				events = append(events, event)
			})
			handler := handlers.NewMetricsHandler(service, handlers.WithAuditPublisher(publisher))
			response := serveRequest(t, createAuditTestRouter(handler), http.MethodPost, testCase.path, testCase.body)
			require.Equal(t, testCase.status, response.Code)
			require.Empty(t, events)
		})
	}
}

type auditObserverFunc func(context.Context, audit.Event) error

func (notify auditObserverFunc) Notify(ctx context.Context, event audit.Event) error {
	return notify(ctx, event)
}

func TestMetricsHandler_Audit_DeliveryFailurePreservesSuccessfulResponse(t *testing.T) {
	service := newMockMetricsApplication(t)
	service.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
	var events []audit.Event
	publisher, err := audit.NewPublisher(
		logger.NewMockLogger(),
		auditObserverFunc(func(context.Context, audit.Event) error { return errors.New("receiver unavailable") }),
		auditObserverFunc(func(_ context.Context, event audit.Event) error {
			events = append(events, event)
			return nil
		}),
	)
	require.NoError(t, err)
	handler := handlers.NewMetricsHandler(service, handlers.WithAuditPublisher(publisher))
	response := serveRequest(t, createAuditTestRouter(handler), http.MethodPost, "/update/counter/Frees/1", "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Len(t, events, 1)
	require.Equal(t, []string{"Frees"}, events[0].Metrics)
}
