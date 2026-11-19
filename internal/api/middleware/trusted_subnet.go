package middleware

import (
	"net"
	"net/http"
	"strings"
)

// TrustedSubnet rejects metric updates whose X-Real-IP is not in subnet.
// A nil subnet disables the check. Rejected responses retain response signing.
func TrustedSubnet(next http.Handler, subnet *net.IPNet, hashKey string) http.Handler {
	if subnet == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMetricUpdate(r) {
			next.ServeHTTP(w, r)
			return
		}
		values := r.Header.Values("X-Real-IP")
		if len(values) == 1 {
			ip := net.ParseIP(strings.TrimSpace(values[0]))
			if ip != nil && subnet.Contains(ip) {
				next.ServeHTTP(w, r)
				return
			}
		}
		if hashKey != "" {
			writeSignedResponse(w, http.StatusForbidden, nil, hashKey, r.Method)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	})
}

// Keep these paths aligned with the metric update routes in api/routes.go.
func isMetricUpdate(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/update", "/update/", "/updates", "/updates/":
		return true
	}
	if !strings.HasPrefix(r.URL.Path, "/update/") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/update/"), "/")
	if len(parts) == 4 && parts[3] == "" {
		parts = parts[:3]
	}
	return len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != ""
}
