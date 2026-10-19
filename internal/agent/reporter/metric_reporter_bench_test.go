package reporter

import (
	"runtime"
	"testing"
)

func BenchmarkMetricsBatch(b *testing.B) {
	reporter := &MetricReporter{}
	snapshot := runtime.MemStats{Alloc: 1024, TotalAlloc: 4096, NumGC: 10, HeapAlloc: 1024}
	b.ReportAllocs()
	for b.Loop() {
		if batch := reporter.metricsBatch(snapshot, 1); len(batch) == 0 {
			b.Fatal("empty metric batch")
		}
	}
}
