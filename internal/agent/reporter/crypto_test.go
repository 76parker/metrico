package reporter

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/76Parker/metrico/internal/api/middleware"
	"github.com/76Parker/metrico/internal/domain/metrics"
)

func TestSendEncryptedBatch(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var received []metrics.Metrics
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal(data, &received); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(middleware.VerifyAndSign(middleware.DecryptRequest(handler, key), "secret"))
	defer server.Close()
	reporter := NewMetricReporter(server.URL, WithHTTPClient(server.Client()), WithCryptoKey(&key.PublicKey), WithKey("secret"))
	value := 42.0
	if err := reporter.sendBatch([]metrics.Metrics{{ID: "test", Type: metrics.MetricTypeGauge, Value: &value}}); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 || received[0].ID != "test" || received[0].Value == nil || *received[0].Value != value {
		t.Fatalf("unexpected metrics: %+v", received)
	}
}
