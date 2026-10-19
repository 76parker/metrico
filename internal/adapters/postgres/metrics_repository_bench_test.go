package postgres

import (
	"fmt"
	"testing"

	metricsapp "github.com/76Parker/metrico/internal/applications/metrics"
)

func BenchmarkMakeBatch(b *testing.B) {
	for _, size := range []int{1, 50, 500} {
		b.Run(fmt.Sprintf("metrics=%d", size), func(b *testing.B) {
			changes := make([]metricsapp.Change, size)
			for i := range changes {
				name := fmt.Sprintf("metric_%04d", i)
				if i%2 == 0 {
					changes[i] = metricsapp.NewSetGaugeChange(name, float64(i))
				} else {
					changes[i] = metricsapp.NewAddCounterChange(name, 1)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := makeBatch(changes); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
