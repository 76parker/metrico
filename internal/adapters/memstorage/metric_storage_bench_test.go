package memstorage

import (
	"context"
	"fmt"
	"testing"

	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
)

func BenchmarkMemStorage(b *testing.B) {
	for _, size := range []int{1, 50, 500} {
		b.Run(fmt.Sprintf("metrics=%d", size), func(b *testing.B) {
			ctx := context.Background()
			storage := NewMemStorage()
			changes := make([]metricsapp.Change, size)
			for i := range changes {
				name := fmt.Sprintf("metric_%04d", i)
				if i%2 == 0 {
					changes[i] = metricsapp.NewSetGaugeChange(name, float64(i))
				} else {
					changes[i] = metricsapp.NewAddCounterChange(name, 1)
				}
			}
			if err := storage.Apply(ctx, changes); err != nil {
				b.Fatal(err)
			}
			b.Run("Apply", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if err := storage.Apply(ctx, changes); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("Get", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := storage.Get(ctx, "metric_0000"); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("GetAll", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := storage.GetAll(ctx); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
