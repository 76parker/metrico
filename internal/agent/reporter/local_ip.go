package reporter

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
)

type localIPResolver func(context.Context, *http.Client, *http.Request) (string, error)

// setRealIP uses the request context and the client's timeout for route lookup.
func (r *MetricReporter) setRealIP(request *http.Request, client *http.Client) error {
	ctx := request.Context()
	if client.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, client.Timeout)
		defer cancel()
	}
	ip, err := r.resolveLocalIP(ctx, client, request)
	if err != nil {
		return fmt.Errorf("determine agent IP: %w", err)
	}
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("determine agent IP: invalid local IP %q", ip)
	}
	request.Header.Set("X-Real-IP", ip)
	return nil
}

// routeLocalIP asks the OS which local address it would use for the first hop.
// For a hostname with multiple addresses, a TCP connection lets the dialer
// select a reachable address instead of reporting the route to an unused one.
func routeLocalIP(ctx context.Context, client *http.Client, request *http.Request) (string, error) {
	target := request.URL
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if httpTransport, ok := transport.(*http.Transport); ok && httpTransport.Proxy != nil {
		proxyURL, err := httpTransport.Proxy(request.WithContext(ctx))
		if err != nil {
			return "", fmt.Errorf("resolve HTTP proxy: %w", err)
		}
		if proxyURL != nil {
			target = proxyURL
		}
	}
	address, err := targetAddress(target)
	if err != nil {
		return "", err
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("parse destination %s: %w", address, err)
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", &net.OpError{Op: "dial", Net: "tcp", Err: err}
	}
	if len(addresses) == 0 {
		return "", fmt.Errorf("no IP addresses for %s", host)
	}
	network := "udp"
	if len(addresses) == 1 {
		address = net.JoinHostPort(addresses[0].String(), port)
	} else {
		network = "tcp"
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return "", fmt.Errorf("select route to %s: %w", address, err)
	}
	defer connection.Close()
	var ip net.IP
	switch local := connection.LocalAddr().(type) {
	case *net.UDPAddr:
		ip = local.IP
	case *net.TCPAddr:
		ip = local.IP
	}
	if ip == nil || ip.IsUnspecified() {
		return "", fmt.Errorf("no local IP for route to %s", address)
	}
	return ip.String(), nil
}

func targetAddress(target *url.URL) (string, error) {
	host := target.Hostname()
	if host == "" {
		return "", fmt.Errorf("destination has no host")
	}
	port := target.Port()
	if port == "" {
		switch target.Scheme {
		case "", "http":
			port = "80"
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			return "", fmt.Errorf("unsupported destination scheme %q", target.Scheme)
		}
	}
	return net.JoinHostPort(host, port), nil
}
