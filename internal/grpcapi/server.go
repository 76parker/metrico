// Package grpcapi exposes the metrics application over gRPC.
package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/76Parker/metrico/internal/applications/audit"
	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
	"github.com/76Parker/metrico/internal/domain/metrics"
	metricspb "github.com/76Parker/metrico/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type metricsApplication interface {
	BatchUpdate(context.Context, metricsapp.BatchUpdateCommand) error
}

// AuditPublisher receives successful, nonempty metric updates.
type AuditPublisher interface {
	Notify(context.Context, audit.Event)
}

// Server owns the gRPC listener and registered metrics service.
type Server struct {
	address string
	server  *grpc.Server
}

// NewServer prepares a gRPC server without opening its listener.
func NewServer(address string, application metricsApplication, subnet *net.IPNet, publisher AuditPublisher) *Server {
	server := grpc.NewServer(grpc.UnaryInterceptor(trustedSubnetInterceptor(subnet)))
	metricspb.RegisterMetricsServer(server, &metricsService{application: application, audit: publisher})
	return &Server{address: address, server: server}
}

// Serve listens until stopped or a listener error occurs.
func (s *Server) Serve() error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("listen grpc %s: %w", s.address, err)
	}
	defer listener.Close()
	if err := s.server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("serve grpc %s: %w", s.address, err)
	}
	return nil
}

// Shutdown waits for active RPCs, forcing them to stop when ctx expires.
func (s *Server) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		// A handler may ignore cancellation, so neither Stop nor GracefulStop
		// should keep shutdown blocked after its deadline.
		go s.server.Stop()
		return fmt.Errorf("shutdown grpc %s: %w", s.address, ctx.Err())
	}
}

func trustedSubnetInterceptor(subnet *net.IPNet) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if subnet != nil {
			md, _ := metadata.FromIncomingContext(ctx)
			values := md.Get("x-real-ip")
			if len(values) != 1 {
				return nil, status.Error(codes.PermissionDenied, "agent IP is not in the trusted subnet")
			}
			ip := net.ParseIP(strings.TrimSpace(values[0]))
			if ip == nil || !subnet.Contains(ip) {
				return nil, status.Error(codes.PermissionDenied, "agent IP is not in the trusted subnet")
			}
		}
		return handler(ctx, req)
	}
}

type metricsService struct {
	metricspb.UnimplementedMetricsServer
	application metricsApplication
	audit       AuditPublisher
}

func (s *metricsService) UpdateMetrics(ctx context.Context, request *metricspb.UpdateMetricsRequest) (*metricspb.UpdateMetricsResponse, error) {
	commands := make(metricsapp.BatchUpdateCommand, 0, len(request.GetMetrics()))
	for _, metric := range request.GetMetrics() {
		if metric == nil {
			return nil, status.Error(codes.InvalidArgument, "metric is missing")
		}
		command := metricsapp.UpdateCommand{Name: metric.Id}
		switch metric.Type {
		case metricspb.Metric_GAUGE:
			command.MetricType = string(metrics.MetricTypeGauge)
			command.Value = strconv.FormatFloat(metric.Value, 'g', -1, 64)
		case metricspb.Metric_COUNTER:
			command.MetricType = string(metrics.MetricTypeCounter)
			command.Value = strconv.FormatInt(metric.Delta, 10)
		default:
			return nil, status.Error(codes.InvalidArgument, "invalid metric type")
		}
		commands = append(commands, command)
	}
	if err := s.application.BatchUpdate(ctx, commands); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, status.FromContextError(err).Err()
		}
		if errors.Is(err, metrics.ErrMetricNameIsEmpty) || errors.Is(err, metrics.ErrInvalidMetricType) ||
			errors.Is(err, metrics.ErrInvalidValueForGauge) || errors.Is(err, metrics.ErrInvalidValueForCounter) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, "failed to save metrics")
	}
	if s.audit != nil && len(commands) > 0 {
		names := make([]string, 0, len(commands))
		for _, command := range commands {
			names = append(names, command.Name)
		}
		ip := ""
		if remote, ok := peer.FromContext(ctx); ok && remote.Addr != nil {
			ip = remote.Addr.String()
			if host, _, err := net.SplitHostPort(ip); err == nil {
				ip = host
			}
		}
		s.audit.Notify(ctx, audit.Event{TS: time.Now().Unix(), Metrics: names, IPAddress: ip})
	}
	return &metricspb.UpdateMetricsResponse{}, nil
}
