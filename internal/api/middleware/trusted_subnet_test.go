package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrustedSubnet(t *testing.T) {
	for _, tc := range []struct {
		name   string
		subnet string
		method string
		path   string
		ips    []string
		want   int
	}{
		{name: "disabled permits missing header", method: "POST", path: "/updates", want: http.StatusNoContent},
		{name: "disabled permits malformed header", method: "POST", path: "/updates", ips: []string{"invalid"}, want: http.StatusNoContent},
		{name: "IPv4 within subnet", subnet: "192.168.1.0/24", method: "POST", path: "/updates", ips: []string{"192.168.1.25"}, want: http.StatusNoContent},
		{name: "IPv4 outside subnet", subnet: "192.168.1.0/24", method: "POST", path: "/updates", ips: []string{"192.168.2.25"}, want: http.StatusForbidden},
		{name: "IPv6 within subnet", subnet: "2001:db8::/32", method: "POST", path: "/update", ips: []string{"2001:db8::1"}, want: http.StatusNoContent},
		{name: "IPv6 outside subnet", subnet: "2001:db8::/32", method: "POST", path: "/update", ips: []string{"2001:db9::1"}, want: http.StatusForbidden},
		{name: "mapped IPv4", subnet: "192.168.1.0/24", method: "POST", path: "/update", ips: []string{"::ffff:192.168.1.2"}, want: http.StatusNoContent},
		{name: "missing header", subnet: "10.0.0.0/8", method: "POST", path: "/updates/", want: http.StatusForbidden},
		{name: "empty header", subnet: "10.0.0.0/8", method: "POST", path: "/updates/", ips: []string{""}, want: http.StatusForbidden},
		{name: "invalid header", subnet: "10.0.0.0/8", method: "POST", path: "/updates/", ips: []string{"invalid"}, want: http.StatusForbidden},
		{name: "address with port", subnet: "10.0.0.0/8", method: "POST", path: "/updates/", ips: []string{"10.0.0.1:1234"}, want: http.StatusForbidden},
		{name: "comma-separated addresses", subnet: "10.0.0.0/8", method: "POST", path: "/updates/", ips: []string{"10.0.0.1, 10.0.0.2"}, want: http.StatusForbidden},
		{name: "duplicate headers", subnet: "10.0.0.0/8", method: "POST", path: "/updates/", ips: []string{"10.0.0.1", "10.0.0.2"}, want: http.StatusForbidden},
		{name: "path update", subnet: "10.0.0.0/8", method: "POST", path: "/update/gauge/Test/1", want: http.StatusForbidden},
		{name: "path update redirect", subnet: "10.0.0.0/8", method: "POST", path: "/update/gauge/Test/1/", want: http.StatusForbidden},
		{name: "metric read", subnet: "10.0.0.0/8", method: "POST", path: "/value", want: http.StatusNoContent},
		{name: "health", subnet: "10.0.0.0/8", method: "GET", path: "/ping", want: http.StatusNoContent},
		{name: "other method", subnet: "10.0.0.0/8", method: "GET", path: "/updates", want: http.StatusNoContent},
		{name: "unknown update path", subnet: "10.0.0.0/8", method: "POST", path: "/update/gauge/Test", want: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var subnet *net.IPNet
			if tc.subnet != "" {
				var err error
				_, subnet, err = net.ParseCIDR(tc.subnet)
				require.NoError(t, err)
			}
			handler := TrustedSubnet(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}), subnet, "")
			request := httptest.NewRequest(tc.method, tc.path, nil)
			for _, ip := range tc.ips {
				request.Header.Add("X-Real-IP", ip)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, tc.want, response.Code)
		})
	}
}
