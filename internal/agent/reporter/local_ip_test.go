package reporter

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/76Parker/metrico/internal/api/middleware"
	"github.com/stretchr/testify/require"
)

func TestTargetAddress(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{raw: "http://example.test", want: "example.test:80"},
		{raw: "https://example.test", want: "example.test:443"},
		{raw: "http://[::1]:8080", want: "[::1]:8080"},
		{raw: "socks5://proxy.test", want: "proxy.test:1080"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			target, err := url.Parse(tc.raw)
			require.NoError(t, err)
			address, err := targetAddress(target)
			require.NoError(t, err)
			require.Equal(t, tc.want, address)
		})
	}
}

func TestRouteLocalIPUsesProxy(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:8080")
	require.NoError(t, err)
	client := &http.Client{Transport: &http.Transport{Proxy: func(*http.Request) (*url.URL, error) {
		return proxyURL, nil
	}}}
	request := httptest.NewRequest(http.MethodPost, "http://unresolvable.invalid/updates", nil)
	ip, err := routeLocalIP(context.Background(), client, request)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", ip)
}

func TestRouteLocalIPIPv6(t *testing.T) {
	probe, err := net.Dial("udp6", "[::1]:8080")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	require.NoError(t, probe.Close())
	client := &http.Client{Transport: &http.Transport{}}
	request := httptest.NewRequest(http.MethodPost, "http://[::1]:8080/updates", nil)
	ip, err := routeLocalIP(context.Background(), client, request)
	require.NoError(t, err)
	require.Equal(t, net.ParseIP("::1"), net.ParseIP(ip))
}

func TestRouteLocalIPPassesContextToProxy(t *testing.T) {
	client := &http.Client{Transport: &http.Transport{Proxy: func(request *http.Request) (*url.URL, error) {
		select {
		case <-request.Context().Done():
			return nil, request.Context().Err()
		case <-time.After(time.Second):
			return nil, errors.New("proxy did not receive lookup context")
		}
	}}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "http://example.test/updates", nil)
	_, err := routeLocalIP(ctx, client, request)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSendBatchLocalhostUsesReachableAddress(t *testing.T) {
	_, subnet, err := net.ParseCIDR("127.0.0.0/8")
	require.NoError(t, err)
	received := make(chan string, 1)
	server := httptest.NewServer(middleware.TrustedSubnet(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		received <- request.Header.Get("X-Real-IP")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}), subnet, ""))
	defer server.Close()
	host, port, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	if net.ParseIP(host).To4() == nil {
		t.Skip("test server is not listening on IPv4")
	}
	r := NewMetricReporter("http://"+net.JoinHostPort("localhost", port), WithHTTPClient(server.Client()))
	require.NoError(t, r.sendBatch(nil))
	require.Equal(t, "127.0.0.1", <-received)
}

func TestSendBatchDeterminesIPForRedirect(t *testing.T) {
	var addresses []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		addresses = append(addresses, request.Header.Get("X-Real-IP"))
		if len(addresses) == 1 {
			return &http.Response{StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": []string{"http://second.test/updates"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	r := NewMetricReporter("http://first.test", WithHTTPClient(client))
	r.resolveLocalIP = func(_ context.Context, _ *http.Client, request *http.Request) (string, error) {
		if request.URL.Hostname() == "first.test" {
			return "192.0.2.1", nil
		}
		return "198.51.100.2", nil
	}
	require.NoError(t, r.sendBatch(nil))
	require.Equal(t, []string{"192.0.2.1", "198.51.100.2"}, addresses)
}

func TestSendBatchDeterminesIPForRetry(t *testing.T) {
	var addresses []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		addresses = append(addresses, request.Header.Get("X-Real-IP"))
		if len(addresses) == 1 {
			return nil, &net.OpError{Op: "dial", Err: errors.New("connection failed")}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	r := NewMetricReporter("http://metrics.test", WithHTTPClient(client))
	resolutions := 0
	r.resolveLocalIP = func(context.Context, *http.Client, *http.Request) (string, error) {
		resolutions++
		if resolutions == 1 {
			return "192.0.2.1", nil
		}
		return "192.0.2.2", nil
	}
	require.NoError(t, r.sendBatch(nil))
	require.Equal(t, []string{"192.0.2.1", "192.0.2.2"}, addresses)
}

func TestSendBatchFailsWhenIPCannotBeDetermined(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected request")
	})}
	r := NewMetricReporter("http://metrics.test", WithHTTPClient(client))
	r.resolveLocalIP = func(context.Context, *http.Client, *http.Request) (string, error) {
		return "", errors.New("no route")
	}
	require.ErrorContains(t, r.sendBatch(nil), "no route")
	require.False(t, called)
}

func TestSendBatchIPResolutionHonorsTimeout(t *testing.T) {
	client := &http.Client{Timeout: 20 * time.Millisecond, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("request must not be sent")
		return nil, nil
	})}
	r := NewMetricReporter("http://metrics.test", WithHTTPClient(client))
	r.resolveLocalIP = func(ctx context.Context, _ *http.Client, _ *http.Request) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	require.ErrorIs(t, r.sendBatch(nil), context.DeadlineExceeded)
}

func TestSendBatchTimeoutIncludesIPResolution(t *testing.T) {
	var lookupDeadline, requestDeadline time.Time
	client := &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestDeadline, _ = request.Context().Deadline()
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	r := NewMetricReporter("http://metrics.test", WithHTTPClient(client))
	r.resolveLocalIP = func(ctx context.Context, _ *http.Client, _ *http.Request) (string, error) {
		lookupDeadline, _ = ctx.Deadline()
		time.Sleep(20 * time.Millisecond)
		return "192.0.2.1", nil
	}
	require.NoError(t, r.sendBatch(nil))
	require.False(t, lookupDeadline.IsZero())
	require.Equal(t, lookupDeadline, requestDeadline)
}
