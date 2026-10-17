//go:build !race

package bench

import (
	"context"
	"sort"
	"testing"
	"time"

	widgetinfra "github.com/lesquel/termino/internal/features/widget/infrastructure"
)

// TestWidgetSummaryP50WithinTarget guards PRD success criterion 2 / TRD 14:
// the widget summary (open DB, query, render) for a 500-task board must stay
// under 100 ms. Measured p50 is ~3 ms on a laptop, so the margin is wide
// enough to be stable; the race detector build and -short skip it.
func TestWidgetSummaryP50WithinTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("latency guard skipped in -short mode")
	}
	const target = 100 * time.Millisecond
	e := seed(t, 500)
	ctx := context.Background()
	renderer := widgetinfra.NewNormalRenderer()

	samples := make([]time.Duration, 0, 21)
	for i := 0; i < cap(samples); i++ {
		start := time.Now()
		if err := widgetOnce(ctx, e, renderer); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p50 := samples[len(samples)/2]
	t.Logf("widget summary p50 = %v (target %v)", p50, target)
	if p50 > target {
		t.Fatalf("widget summary p50 = %v, want < %v", p50, target)
	}
}
