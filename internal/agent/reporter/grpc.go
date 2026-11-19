package reporter

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/76Parker/metrico/internal/domain/metrics"
	metricspb "github.com/76Parker/metrico/internal/proto"
	"google.golang.org/grpc/metadata"
)

func (r *MetricReporter) sendGRPCBatch(ctx context.Context, batch []metrics.Metrics) error {
	request := &metricspb.UpdateMetricsRequest{Metrics: make([]*metricspb.Metric, 0, len(batch))}
	for _, metric := range batch {
		item := &metricspb.Metric{Id: metric.ID}
		switch metric.Type {
		case metrics.MetricTypeGauge:
			if metric.Value == nil {
				return metrics.ErrGaugeValueIsNil
			}
			item.Type = metricspb.Metric_GAUGE
			item.Value = *metric.Value
		case metrics.MetricTypeCounter:
			if metric.Delta == nil {
				return metrics.ErrCounterValueIsNil
			}
			item.Type = metricspb.Metric_COUNTER
			item.Delta = *metric.Delta
		default:
			return metrics.ErrInvalidMetricType
		}
		request.Metrics = append(request.Metrics, item)
	}

	timeout := 5 * time.Second
	if r.client != nil && r.client.Timeout > 0 {
		timeout = r.client.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ip, err := r.resolveGRPCIP(ctx, r.grpcAddress)
	if err != nil {
		return fmt.Errorf("determine agent IP: %w", err)
	}
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("determine agent IP: invalid local IP %q", ip)
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set("x-real-ip", ip)
	ctx = metadata.NewOutgoingContext(ctx, md)
	if _, err := r.grpcClient.UpdateMetrics(ctx, request); err != nil {
		return fmt.Errorf("update metrics over gRPC: %w", err)
	}
	return nil
}
