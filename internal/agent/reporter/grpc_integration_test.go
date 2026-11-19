package reporter

import (
	"context"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/76Parker/metrico/internal/adapters/memstorage"
	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
	"github.com/76Parker/metrico/internal/grpcapi"
	metricspb "github.com/76Parker/metrico/internal/proto"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestGRPCReporterToServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	_, subnet, err := net.ParseCIDR("127.0.0.0/8")
	require.NoError(t, err)
	application := metricsapp.NewApplication(metricsapp.NewService(memstorage.NewMemStorage()))
	server := grpcapi.NewServer(address, application, subnet, nil)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, server.Shutdown(ctx))
		require.NoError(t, <-serveErr)
	})
	require.Eventually(t, func() bool {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = connection.Close()
		return true
	}, 5*time.Second, time.Millisecond)

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	reporter := NewMetricReporter("http://localhost:8080", WithGRPCClient(metricspb.NewMetricsClient(conn), address))
	require.NoError(t, reporter.sendMetrics(runtime.MemStats{Alloc: 42}, 7))
	require.NoError(t, reporter.sendMetrics(runtime.MemStats{Alloc: 23}, 2))
	gauge, err := application.GetByName(context.Background(), "Alloc")
	require.NoError(t, err)
	require.Equal(t, 23.0, *gauge.Value)
	counter, err := application.GetByName(context.Background(), "PollCount")
	require.NoError(t, err)
	require.EqualValues(t, 9, *counter.Delta)
}
