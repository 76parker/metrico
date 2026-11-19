package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"

	"github.com/76Parker/metrico/internal/adapters/filestorage"
	"github.com/76Parker/metrico/internal/adapters/memstorage"
	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
	"github.com/76Parker/metrico/internal/domain/metrics"
	"github.com/76Parker/metrico/internal/grpcapi"
	metricspb "github.com/76Parker/metrico/internal/proto"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/76Parker/metrico/internal/api"
	"github.com/76Parker/metrico/pkg/logger"
)

func TestApplication_Run_StopsBothServersOnCancellation(t *testing.T) {
	for _, diagnosticsEnabled := range []bool{false, true} {
		name := "diagnostics disabled"
		if diagnosticsEnabled {
			name = "diagnostics enabled"
		}
		t.Run(name, func(t *testing.T) {
			serverAddress := unusedAddress(t)
			application := &application{
				server: &http.Server{Addr: serverAddress, Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
					response.WriteHeader(http.StatusNoContent)
				})},
				shutdownTimeout: time.Second,
				log:             logger.NewMockLogger(),
			}
			if diagnosticsEnabled {
				var err error
				application.diagnostics, err = api.NewDiagnosticsServer(unusedAddress(t))
				require.NoError(t, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			finished := make(chan error, 1)
			go func() { finished <- application.run(ctx) }()
			waitForHTTP(t, "http://"+serverAddress+"/")
			if diagnosticsEnabled {
				waitForHTTP(t, "http://"+application.diagnostics.Addr+"/debug/pprof/")
			}
			cancel()
			select {
			case err := <-finished:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("application did not stop")
			}
			addresses := []string{serverAddress}
			if diagnosticsEnabled {
				addresses = append(addresses, application.diagnostics.Addr)
			}
			for _, address := range addresses {
				listener, err := net.Listen("tcp", address)
				require.NoError(t, err, "listener must be released")
				require.NoError(t, listener.Close())
			}
		})
	}
}

func TestApplication_Run_PropagatesListenerFailure(t *testing.T) {
	for _, failedServer := range []string{"main", "diagnostics"} {
		t.Run(failedServer, func(t *testing.T) {
			occupied, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer func() { require.NoError(t, occupied.Close()) }()
			application := &application{
				server:          &http.Server{Addr: "127.0.0.1:0"},
				diagnostics:     &http.Server{Addr: "127.0.0.1:0"},
				shutdownTimeout: time.Second,
				log:             logger.NewMockLogger(),
			}
			if failedServer == "main" {
				application.server.Addr = occupied.Addr().String()
			} else {
				application.diagnostics.Addr = occupied.Addr().String()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = application.run(ctx)
			require.Error(t, err)
			var networkError *net.OpError
			require.True(t, errors.As(err, &networkError), "want listener failure, got %v", err)
			require.NoError(t, ctx.Err(), "failure must stop application before timeout")
		})
	}
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}

func waitForHTTP(t *testing.T, address string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		require.NoError(t, err)
		response, err := client.Do(request)
		if err == nil {
			require.NoError(t, response.Body.Close())
			require.Less(t, response.StatusCode, 300)
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("server did not become ready: %s: %v", address, ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestApplication_FinalSnapshotWaitsForHandler(t *testing.T) {
	address := unusedAddress(t)
	store, err := filestorage.NewStorage(filepath.Join(t.TempDir(), "metrics.json"))
	require.NoError(t, err)
	memory := memstorage.NewMemStorage()
	snapshot := metricsapp.NewSnapshotCoordinator(store, memory, time.Hour)
	started, release := make(chan struct{}), make(chan struct{})
	app := &application{server: &http.Server{Addr: address, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		value := 42.0
		_ = memory.Load(context.Background(), []metrics.Metrics{{ID: "last", Type: metrics.MetricTypeGauge, Value: &value}})
	})}, snapshotter: snapshot, backgroundWorkers: []runner{snapshot}, shutdownTimeout: 5 * time.Second, log: logger.NewMockLogger()}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- app.run(ctx) }()
	// Probe the listener without invoking the blocking handler.
	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}, 5*time.Second, time.Millisecond)
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := http.Get("http://" + address)
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	<-started
	cancel()
	close(release)
	require.NoError(t, <-finished)
	<-requestDone
	saved, err := store.Restore(context.Background())
	require.NoError(t, err)
	require.Len(t, saved, 1)
	require.Equal(t, "last", saved[0].ID)
	require.Equal(t, 42.0, *saved[0].Value)
}

type grpcBatchFunc func(context.Context, metricsapp.BatchUpdateCommand) error

func (f grpcBatchFunc) BatchUpdate(ctx context.Context, commands metricsapp.BatchUpdateCommand) error {
	return f(ctx, commands)
}

func TestApplication_FinalSnapshotWaitsForGRPC(t *testing.T) {
	httpAddress, grpcAddress := unusedAddress(t), unusedAddress(t)
	store, err := filestorage.NewStorage(filepath.Join(t.TempDir(), "metrics.json"))
	require.NoError(t, err)
	memory := memstorage.NewMemStorage()
	snapshot := metricsapp.NewSnapshotCoordinator(store, memory, time.Hour)
	started, release := make(chan struct{}), make(chan struct{})
	grpcApp := grpcBatchFunc(func(ctx context.Context, _ metricsapp.BatchUpdateCommand) error {
		close(started)
		<-release
		value := 42.0
		return memory.Load(ctx, []metrics.Metrics{{ID: "last", Type: metrics.MetricTypeGauge, Value: &value}})
	})
	app := &application{
		server:            &http.Server{Addr: httpAddress, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })},
		grpcServer:        grpcapi.NewServer(grpcAddress, grpcApp, nil, nil),
		snapshotter:       snapshot,
		backgroundWorkers: []runner{snapshot},
		shutdownTimeout:   5 * time.Second,
		log:               logger.NewMockLogger(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- app.run(ctx) }()
	waitForHTTP(t, "http://"+httpAddress+"/")
	require.Eventually(t, func() bool {
		connection, err := net.DialTimeout("tcp", grpcAddress, 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = connection.Close()
		return true
	}, 5*time.Second, time.Millisecond)
	conn, err := grpc.NewClient(grpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	rpcDone := make(chan error, 1)
	go func() {
		_, err := metricspb.NewMetricsClient(conn).UpdateMetrics(context.Background(), &metricspb.UpdateMetricsRequest{Metrics: []*metricspb.Metric{{Id: "last", Value: 42}}})
		rpcDone <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("gRPC handler did not start")
	}
	cancel()
	close(release)
	require.NoError(t, <-finished)
	require.NoError(t, <-rpcDone)
	saved, err := store.Restore(context.Background())
	require.NoError(t, err)
	require.Len(t, saved, 1)
	require.Equal(t, "last", saved[0].ID)
	require.Equal(t, 42.0, *saved[0].Value)
	for _, address := range []string{httpAddress, grpcAddress} {
		listener, err := net.Listen("tcp", address)
		require.NoError(t, err, "listener must be released")
		require.NoError(t, listener.Close())
	}
}

func TestApplication_PropagatesGRPCListenerFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()
	application := &application{
		server:          &http.Server{Addr: "127.0.0.1:0"},
		grpcServer:      grpcapi.NewServer(occupied.Addr().String(), grpcBatchFunc(func(context.Context, metricsapp.BatchUpdateCommand) error { return nil }), nil, nil),
		shutdownTimeout: time.Second,
		log:             logger.NewMockLogger(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = application.run(ctx)
	require.Error(t, err)
	var networkError *net.OpError
	require.True(t, errors.As(err, &networkError))
	require.NoError(t, ctx.Err(), "listener failure must stop the application")
}
