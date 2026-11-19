package reporter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/76Parker/metrico/internal/domain/metrics"
	metricspb "github.com/76Parker/metrico/internal/proto"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type recordingGRPCClient struct {
	request  *metricspb.UpdateMetricsRequest
	metadata metadata.MD
	deadline time.Time
	calls    int
	err      error
}

func (c *recordingGRPCClient) UpdateMetrics(ctx context.Context, request *metricspb.UpdateMetricsRequest, _ ...grpc.CallOption) (*metricspb.UpdateMetricsResponse, error) {
	c.calls++
	c.request = request
	c.metadata, _ = metadata.FromOutgoingContext(ctx)
	c.deadline, _ = ctx.Deadline()
	return &metricspb.UpdateMetricsResponse{}, c.err
}

func TestSendGRPCBatch(t *testing.T) {
	client := &recordingGRPCClient{}
	reporter := NewMetricReporter("http://localhost:8080", WithGRPCClient(client, "127.0.0.1:3200"))
	reporter.resolveGRPCIP = func(_ context.Context, address string) (string, error) {
		require.Equal(t, "127.0.0.1:3200", address)
		return "127.0.0.1", nil
	}
	value, zero := 2.5, 0.0
	delta := int64(-3)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-real-ip", "old-ip", "other", "preserved")
	err := reporter.sendBatchContext(ctx, []metrics.Metrics{
		{ID: "temperature", Type: metrics.MetricTypeGauge, Value: &value},
		{ID: "zero", Type: metrics.MetricTypeGauge, Value: &zero},
		{ID: "count", Type: metrics.MetricTypeCounter, Delta: &delta},
	})
	require.NoError(t, err)
	require.Equal(t, 1, client.calls)
	require.Equal(t, []string{"127.0.0.1"}, client.metadata.Get("x-real-ip"))
	require.Equal(t, []string{"preserved"}, client.metadata.Get("other"))
	require.WithinDuration(t, time.Now().Add(5*time.Second), client.deadline, time.Second)
	require.Len(t, client.request.Metrics, 3)
	require.Equal(t, &metricspb.Metric{Id: "temperature", Type: metricspb.Metric_GAUGE, Value: 2.5}, client.request.Metrics[0])
	require.Equal(t, &metricspb.Metric{Id: "zero", Type: metricspb.Metric_GAUGE, Value: 0}, client.request.Metrics[1])
	require.Equal(t, &metricspb.Metric{Id: "count", Type: metricspb.Metric_COUNTER, Delta: -3}, client.request.Metrics[2])
}

func TestSendGRPCBatchFailures(t *testing.T) {
	client := &recordingGRPCClient{}
	reporter := NewMetricReporter("http://localhost:8080", WithGRPCClient(client, "server:3200"))
	reporter.resolveGRPCIP = func(context.Context, string) (string, error) { return "127.0.0.1", nil }
	err := reporter.sendBatchContext(context.Background(), []metrics.Metrics{{ID: "invalid", Type: metrics.MetricTypeGauge}})
	require.ErrorIs(t, err, metrics.ErrGaugeValueIsNil)
	require.Zero(t, client.calls)

	routeErr := errors.New("no route")
	reporter.resolveGRPCIP = func(context.Context, string) (string, error) { return "", routeErr }
	require.ErrorIs(t, reporter.sendBatchContext(context.Background(), nil), routeErr)
	require.Zero(t, client.calls)

	reporter.resolveGRPCIP = func(context.Context, string) (string, error) { return "not-an-ip", nil }
	require.ErrorContains(t, reporter.sendBatchContext(context.Background(), nil), "invalid local IP")
	require.Zero(t, client.calls)

	reporter.resolveGRPCIP = func(context.Context, string) (string, error) { return "::1", nil }
	client.err = status.Error(codes.PermissionDenied, "outside subnet")
	err = reporter.sendBatchContext(context.Background(), nil)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Equal(t, 1, client.calls, "an RPC failure must not be retried")
}
