package provider

import (
	"runtime"
	"strconv"
	"testing"

	"github.com/76Parker/metrico/internal/domain/metrics"
)

func TestCollectSystemMetrics_IncludesMemoryAndPerCPU(t *testing.T) {
	got, err := collectSystemMetrics()
	if err != nil {
		t.Fatalf("collectSystemMetrics() error = %v", err)
	}

	byName := make(map[string]metrics.Metrics, len(got))
	for _, metric := range got {
		byName[metric.ID] = metric
	}
	for _, name := range []string{"TotalMemory", "FreeMemory"} {
		if metric, ok := byName[name]; !ok || metric.Type != metrics.MetricTypeGauge || metric.Value == nil {
			t.Fatalf("missing valid gauge %q: %#v", name, metric)
		}
	}
	if len(got) != runtime.NumCPU()+2 {
		t.Fatalf("metric count = %d, want %d", len(got), runtime.NumCPU()+2)
	}
	for i := 1; i <= runtime.NumCPU(); i++ {
		name := "CPUutilization" + strconv.Itoa(i)
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing CPU metric %q", name)
		}
	}
}
