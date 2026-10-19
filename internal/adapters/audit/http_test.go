package audit_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/76Parker/metrico/internal/adapters/audit"
	"github.com/76Parker/metrico/internal/applications/auditobserver"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (transport roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestHTTP_Notify_PostsEvent(t *testing.T) {
	t.Parallel()
	expected := auditobserver.Event{TS: 12345678, Metrics: []string{"Alloc", "Frees"}, IPAddress: "192.168.0.42"}
	type receivedRequest struct {
		method      string
		contentType string
		path        string
		event       auditobserver.Event
		err         error
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event auditobserver.Event
		err := json.NewDecoder(r.Body).Decode(&event)
		received <- receivedRequest{r.Method, r.Header.Get("Content-Type"), r.URL.RequestURI(), event, err}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	receiver, err := audit.NewHTTP(server.URL+"/audit?source=metrics", server.Client())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, receiver.Close()) })
	require.NoError(t, receiver.Notify(context.Background(), expected))
	request := <-received
	require.NoError(t, request.err)
	require.Equal(t, http.MethodPost, request.method)
	require.Equal(t, "application/json", request.contentType)
	require.Equal(t, "/audit?source=metrics", request.path)
	require.Equal(t, expected, request.event)
}

func TestHTTP_Notify_ChecksResponseStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []int{200, 201, 204, 299, 300, 307, 400, 500} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			t.Cleanup(server.Close)
			receiver, err := audit.NewHTTP(server.URL, server.Client())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, receiver.Close()) })
			err = receiver.Notify(context.Background(), auditobserver.Event{})
			if status >= 200 && status < 300 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

func TestHTTP_Notify_RespectsCancellationAndTimeout(t *testing.T) {
	for _, isCanceled := range []bool{false, true} {
		name := "client timeout"
		if isCanceled {
			name = "request cancellation"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client := &http.Client{
					Timeout: time.Second,
					Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
						<-r.Context().Done()
						return nil, r.Context().Err()
					}),
				}
				receiver, err := audit.NewHTTP("http://receiver.example/audit", client)
				require.NoError(t, err)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				expectedErr := context.DeadlineExceeded
				if isCanceled {
					cancel()
					expectedErr = context.Canceled
				}
				require.ErrorIs(t, receiver.Notify(ctx, auditobserver.Event{}), expectedErr)
			})
		})
	}
}

type responseBody struct {
	io.Reader
	isClosed bool
	closeErr error
}

func (b *responseBody) Close() error {
	b.isClosed = true
	return b.closeErr
}

func TestHTTP_Notify_ClosesResponseBody(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("response close failed")
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			body := &responseBody{Reader: strings.NewReader(""), closeErr: closeErr}
			client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: body}, nil
			})}
			receiver, err := audit.NewHTTP("http://receiver.example/audit", client)
			require.NoError(t, err)
			require.ErrorIs(t, receiver.Notify(context.Background(), auditobserver.Event{}), closeErr)
			require.True(t, body.isClosed)
		})
	}
}

func TestNewHTTP_RejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	endpoints := []string{"", "/audit", "//example.com/audit", "ftp://example.com/audit", "http://", "http://%zz"}
	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			_, err := audit.NewHTTP(endpoint, &http.Client{})
			require.Error(t, err)
		})
	}
	t.Run("missing client", func(t *testing.T) {
		_, err := audit.NewHTTP("https://example.com/audit", nil)
		require.Error(t, err)
	})
}
