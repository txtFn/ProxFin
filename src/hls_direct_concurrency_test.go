package src

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDirectHLSRecoveryConcurrencyLimit(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32

	release := make(chan struct{})
	started := make(chan struct{}, 5)

	upstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			current := active.Add(1)
			defer active.Add(-1)

			for {
				old := maximum.Load()
				if current <= old || maximum.CompareAndSwap(old, current) {
					break
				}
			}

			started <- struct{}{}

			select {
			case <-release:
				w.Header().Set("Content-Type", "video/mp2t")
				_, _ = io.WriteString(w, "1234567890")
			case <-r.Context().Done():
				return
			}
		},
	))
	defer upstream.Close()

	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			defer cancel()

			_, _, _ = fetchDirectHLSMediaWithRetry(
				ctx,
				upstream.Client(),
				upstream.URL+"/segment.ts",
			)
		}()
	}

	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			close(release)
			wg.Wait()
			t.Fatal("four downloads did not start")
		}
	}

	// Give the fifth request time to attempt entering.
	time.Sleep(200 * time.Millisecond)

	if got := maximum.Load(); got > 4 {
		close(release)
		wg.Wait()
		t.Fatalf("expected maximum 4, observed %d", got)
	}

	close(release)
	wg.Wait()

	if got := maximum.Load(); got > 4 {
		t.Fatalf("concurrency limit exceeded: %d", got)
	}

	t.Logf("Maximum simultaneous upstream downloads: %d", maximum.Load())
}
