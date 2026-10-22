package api

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"net/netip"
	"time"
)

// NewDiagnosticsServer создает опциональный pprof-сервер для профилирования.
func NewDiagnosticsServer(address string) (*http.Server, error) {
	if address == "" {
		return nil, nil
	}
	endpoint, err := netip.ParseAddrPort(address)
	if err != nil || !endpoint.Addr().IsLoopback() || endpoint.Addr().Zone() != "" {
		return nil, fmt.Errorf("pprof address must be a numeric loopback host:port: %q", address)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}, nil
}
