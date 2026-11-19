package grpcapi

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/76Parker/metrico/internal/adapters/memstorage"
	"github.com/76Parker/metrico/internal/applications/audit"
	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
	metricspb "github.com/76Parker/metrico/internal/proto"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type recordingAudit struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *recordingAudit) Notify(_ context.Context, event audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, event)
}

func (a *recordingAudit) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]audit.Event(nil), a.events...)
}

func newTestClient(t *testing.T, subnet *net.IPNet, publisher AuditPublisher) (metricspb.MetricsClient, *metricsapp.Application) {
	t.Helper()
	application := metricsapp.NewApplication(metricsapp.NewService(memstorage.NewMemStorage()))
	server := NewServer("", application, subnet, publisher)
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.server.Serve(listener) }()
	t.Cleanup(func() {
		server.server.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return metricspb.NewMetricsClient(conn), application
}

func TestUpdateMetricsBatch(t *testing.T) {
	_, subnet, err := net.ParseCIDR("127.0.0.0/8")
	require.NoError(t, err)
	auditRecorder := &recordingAudit{}
	client, application := newTestClient(t, subnet, auditRecorder)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-real-ip", "127.0.0.1")

	_, err = client.UpdateMetrics(ctx, &metricspb.UpdateMetricsRequest{Metrics: []*metricspb.Metric{
		{Id: "temperature", Type: metricspb.Metric_GAUGE, Value: 2.5},
		{Id: "requests", Type: metricspb.Metric_COUNTER, Delta: 4},
		{Id: "requests", Type: metricspb.Metric_COUNTER, Delta: -1},
	}})
	require.NoError(t, err)
	_, err = client.UpdateMetrics(ctx, &metricspb.UpdateMetricsRequest{Metrics: []*metricspb.Metric{
		{Id: "temperature", Type: metricspb.Metric_GAUGE, Value: 0},
		{Id: "requests", Type: metricspb.Metric_COUNTER, Delta: 2},
	}})
	require.NoError(t, err)
	gauge, err := application.GetByName(ctx, "temperature")
	require.NoError(t, err)
	require.Equal(t, 0.0, *gauge.Value)
	counter, err := application.GetByName(ctx, "requests")
	require.NoError(t, err)
	require.EqualValues(t, 5, *counter.Delta)

	events := auditRecorder.snapshot()
	require.Len(t, events, 2)
	require.Equal(t, []string{"temperature", "requests", "requests"}, events[0].Metrics)
	require.Equal(t, "bufconn", events[0].IPAddress)

	_, err = client.UpdateMetrics(ctx, &metricspb.UpdateMetricsRequest{})
	require.NoError(t, err)
	require.Len(t, auditRecorder.snapshot(), 2)

	for _, invalid := range []*metricspb.Metric{
		{Id: "invalid", Type: metricspb.Metric_MType(99)},
		{Id: "", Type: metricspb.Metric_COUNTER, Delta: 1},
	} {
		_, err = client.UpdateMetrics(ctx, &metricspb.UpdateMetricsRequest{Metrics: []*metricspb.Metric{
			{Id: "requests", Type: metricspb.Metric_COUNTER, Delta: 100}, invalid,
		}})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
	counter, err = application.GetByName(ctx, "requests")
	require.NoError(t, err)
	require.EqualValues(t, 5, *counter.Delta)
	require.Len(t, auditRecorder.snapshot(), 2)
}

func TestTrustedSubnetInterceptor(t *testing.T) {
	tests := []struct {
		name   string
		cidr   string
		ips    []string
		status codes.Code
	}{
		{name: "disabled without metadata", status: codes.OK},
		{name: "IPv4 inside", cidr: "10.0.0.0/8", ips: []string{"10.2.3.4"}, status: codes.OK},
		{name: "IPv4 outside", cidr: "10.0.0.0/8", ips: []string{"192.168.0.1"}, status: codes.PermissionDenied},
		{name: "missing", cidr: "10.0.0.0/8", status: codes.PermissionDenied},
		{name: "invalid", cidr: "10.0.0.0/8", ips: []string{"not-an-ip"}, status: codes.PermissionDenied},
		{name: "duplicate", cidr: "10.0.0.0/8", ips: []string{"10.2.3.4", "10.2.3.5"}, status: codes.PermissionDenied},
		{name: "IPv6 inside", cidr: "2001:db8::/32", ips: []string{"2001:db8::1"}, status: codes.OK},
		{name: "IPv6 outside", cidr: "2001:db8::/32", ips: []string{"2001:db9::1"}, status: codes.PermissionDenied},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var subnet *net.IPNet
			if test.cidr != "" {
				var err error
				_, subnet, err = net.ParseCIDR(test.cidr)
				require.NoError(t, err)
			}
			client, application := newTestClient(t, subnet, nil)
			ctx := metadata.NewOutgoingContext(context.Background(), metadata.MD{"x-real-ip": test.ips})
			_, err := client.UpdateMetrics(ctx, &metricspb.UpdateMetricsRequest{Metrics: []*metricspb.Metric{{Id: "accepted", Value: 1}}})
			require.Equal(t, test.status, status.Code(err))
			items, err := application.GetAll(ctx)
			require.NoError(t, err)
			if test.status == codes.OK {
				require.Len(t, items, 1)
			} else {
				require.Empty(t, items)
			}
		})
	}
}

type failingApplication struct{ err error }

func (a failingApplication) BatchUpdate(context.Context, metricsapp.BatchUpdateCommand) error {
	return a.err
}

func TestUpdateMetricsStorageFailure(t *testing.T) {
	publisher := &recordingAudit{}
	service := &metricsService{application: failingApplication{errors.New("storage failed")}, audit: publisher}
	_, err := service.UpdateMetrics(context.Background(), &metricspb.UpdateMetricsRequest{Metrics: []*metricspb.Metric{{Id: "one"}}})
	require.Equal(t, codes.Internal, status.Code(err))
	require.Empty(t, publisher.snapshot())
}

type blockingApplication struct {
	started chan struct{}
	release chan struct{}
}

func (a blockingApplication) BatchUpdate(context.Context, metricsapp.BatchUpdateCommand) error {
	close(a.started)
	<-a.release
	return nil
}

func TestShutdownDeadlineWithBlockedHandler(t *testing.T) {
	application := blockingApplication{started: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(application.release) })
	server := NewServer("", application, nil, nil)
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.server.Serve(listener) }()
	defer listener.Close()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()
	rpcDone := make(chan struct{})
	go func() {
		_, _ = metricspb.NewMetricsClient(conn).UpdateMetrics(context.Background(), &metricspb.UpdateMetricsRequest{Metrics: []*metricspb.Metric{{Id: "blocked"}}})
		close(rpcDone)
	}()
	select {
	case <-application.started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- server.Shutdown(ctx) }()
	select {
	case err := <-shutdownDone:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("shutdown exceeded its deadline")
	}
	release.Do(func() { close(application.release) })
	select {
	case <-rpcDone:
	case <-time.After(time.Second):
		t.Fatal("RPC did not finish")
	}
}
