package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDiagnosticsServer_ValidatesLoopbackAddress(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		address string
		valid   bool
	}{
		{"disabled", "", true},
		{"IPv4 loopback", "127.0.0.1:6060", true},
		{"IPv4 loopback range", "127.0.0.2:0", true},
		{"IPv6 loopback", "[::1]:6060", true},
		{"IPv4 mapped loopback", "[::ffff:127.0.0.1]:6060", true},
		{"hostname", "localhost:6060", false},
		{"wildcard IPv4", "0.0.0.0:6060", false},
		{"wildcard IPv6", "[::]:6060", false},
		{"empty host", ":6060", false},
		{"external", "192.168.1.1:6060", false},
		{"external mapped", "[::ffff:192.168.1.1]:6060", false},
		{"missing port", "127.0.0.1", false},
		{"named port", "127.0.0.1:http", false},
		{"out of range port", "127.0.0.1:65536", false},
		{"negative port", "127.0.0.1:-1", false},
		{"zone", "[::1%lo0]:6060", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server, err := NewDiagnosticsServer(testCase.address)
			if !testCase.valid {
				require.Error(t, err)
				require.Nil(t, server)
				return
			}
			require.NoError(t, err)
			if testCase.address == "" {
				require.Nil(t, server)
				return
			}
			require.Equal(t, testCase.address, server.Addr)
			require.NotSame(t, http.DefaultServeMux, server.Handler)
		})
	}
}

func TestNewDiagnosticsServer_ServesProfilesOnPrivateMux(t *testing.T) {
	server, err := NewDiagnosticsServer("127.0.0.1:6060")
	require.NoError(t, err)
	for _, path := range []string{"/debug/pprof/", "/debug/pprof/heap", "/debug/pprof/allocs", "/debug/pprof/goroutine", "/debug/pprof/cmdline", "/debug/pprof/symbol"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusOK, response.Code)
			require.NotEmpty(t, response.Body.Bytes())
		})
	}
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusNotFound, response.Code)
}
