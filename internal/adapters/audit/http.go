package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	auditapp "github.com/76Parker/metrico/internal/applications/audit"
)

// HTTP posts audit events using a shared client.
type HTTP struct {
	url    string
	client *http.Client
}

// NewHTTP validates the endpoint and requires a configured HTTP client.
func NewHTTP(endpoint string, client *http.Client) (*HTTP, error) {
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse audit url: %w", err)
	}
	if (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Hostname() == "" {
		return nil, errors.New("audit url must be an absolute http or https url with a host")
	}
	if client == nil {
		return nil, errors.New("audit http client is required")
	}
	return &HTTP{url: endpoint, client: client}, nil
}

// Notify posts the event and accepts only successful HTTP responses.
func (h *HTTP) Notify(ctx context.Context, event auditapp.Event) (err error) {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal http audit event: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create audit request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := h.client.Do(request)
	if err != nil {
		return fmt.Errorf("post audit event to %q: %w", h.url, err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close audit response: %w", closeErr))
		}
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("audit endpoint %q returned status %d", h.url, response.StatusCode)
	}
	// Bound response consumption even when the receiver returns a large body.
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, 4096)); err != nil {
		return fmt.Errorf("read audit response: %w", err)
	}
	return nil
}

// Close releases idle connections owned by the audit client.
func (h *HTTP) Close() error {
	h.client.CloseIdleConnections()
	return nil
}
