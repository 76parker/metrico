package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
