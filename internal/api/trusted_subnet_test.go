package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/76Parker/metrico/internal/adapters/memstorage"
	"github.com/76Parker/metrico/internal/api/handlers"
	"github.com/76Parker/metrico/internal/applications/audit"
	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
	"github.com/76Parker/metrico/internal/config"
	"github.com/76Parker/metrico/internal/cryptomsg"
	"github.com/76Parker/metrico/internal/signature"
	"github.com/76Parker/metrico/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type recordingAudit struct{ events []audit.Event }

func (a *recordingAudit) Notify(_ context.Context, event audit.Event) {
	a.events = append(a.events, event)
}

func TestTrustedSubnetProtectsMetricUpdates(t *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir("../.."))
	t.Cleanup(func() { require.NoError(t, os.Chdir(workingDirectory)) })

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	_, subnet, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)
	const hashKey = "test-key"
	application := metricsapp.NewApplication(metricsapp.NewService(memstorage.NewMemStorage()))
	auditRecorder := &recordingAudit{}
	router := gin.New()
	registerMetricsRoutes(router, handlers.NewMetricsHandler(application, handlers.WithAuditPublisher(auditRecorder)), logger.NewMockLogger())
	router.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	server := newHTTPServer(config.HTTP{HashKey: hashKey, CryptoKey: key, TrustedSubnet: subnet}, router)

	var expected int64
	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/update/counter/Test/1"},
		{path: "/update", body: `{"id":"Test","type":"counter","delta":1}`},
		{path: "/update/", body: `{"id":"Test","type":"counter","delta":1}`},
		{path: "/updates", body: `[{"id":"Test","type":"counter","delta":1}]`},
		{path: "/updates/", body: `[{"id":"Test","type":"counter","delta":1}]`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			body := []byte(tc.body)
			allowed := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
			allowed.Header.Set("X-Real-IP", "10.1.2.3")
			allowed.Header.Set(signature.HeaderName, signature.Sum(body, hashKey))
			response := httptest.NewRecorder()
			server.Handler.ServeHTTP(response, allowed)
			require.Equal(t, http.StatusOK, response.Code)
			expected++
			require.Len(t, auditRecorder.events, int(expected))

			denied := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
			denied.Header.Set("X-Real-IP", "203.0.113.1")
			denied.Header.Set(signature.HeaderName, "invalid")
			denied.Header.Set(cryptomsg.Header, "invalid")
			response = httptest.NewRecorder()
			server.Handler.ServeHTTP(response, denied)
			require.Equal(t, http.StatusForbidden, response.Code)
			require.Equal(t, signature.Sum(nil, hashKey), response.Header().Get(signature.HeaderName))
			require.Len(t, auditRecorder.events, int(expected))
			metric, err := application.GetByName(context.Background(), "Test")
			require.NoError(t, err)
			require.Equal(t, expected, *metric.Delta)
		})
	}

	t.Run("encrypted and signed update", func(t *testing.T) {
		body, err := cryptomsg.Encrypt(&key.PublicKey, []byte(`[{"id":"Test","type":"counter","delta":1}]`))
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(body))
		request.Header.Set("X-Real-IP", "10.1.2.3")
		request.Header.Set(cryptomsg.Header, cryptomsg.Version)
		request.Header.Set(signature.HeaderName, signature.Sum(body, hashKey))
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, signature.Sum(response.Body.Bytes(), hashKey), response.Header().Get(signature.HeaderName))
		expected++
		metric, err := application.GetByName(context.Background(), "Test")
		require.NoError(t, err)
		require.Equal(t, expected, *metric.Delta)
		require.Len(t, auditRecorder.events, int(expected))
	})

	for _, tc := range []struct{ method, path, body string }{
		{method: "POST", path: "/value", body: `{"id":"Test","type":"counter"}`},
		{method: "GET", path: "/value/counter/Test"},
		{method: "GET", path: "/ping"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			response := httptest.NewRecorder()
			server.Handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
		})
	}
}
