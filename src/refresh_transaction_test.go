package src

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRefreshTransactionSuccess(t *testing.T) {
	calls := 0

	err := runRefreshTransaction(func() error {
		calls++
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if calls != 1 {
		t.Fatalf("Expected one call, got %d", calls)
	}
}

func TestRefreshTransactionErrorPropagation(t *testing.T) {
	expected := errors.New("synthetic transaction failure")

	err := runRefreshTransaction(func() error {
		return expected
	})

	if !errors.Is(err, expected) {
		t.Fatalf("Original error was not preserved: %v", err)
	}
}

func TestRefreshTransactionPanicHandling(t *testing.T) {
	err := runRefreshTransaction(func() error {
		panic("synthetic panic")
	})

	if err == nil {
		t.Fatal("Expected panic error")
	}

	if !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("Unexpected error: %v", err)
	}

	if strings.Contains(err.Error(), "synthetic panic") {
		t.Fatal("Panic value was exposed")
	}
}

func TestRefreshTransactionLockRelease(t *testing.T) {
	operations := []struct {
		name string
		run  func() error
	}{
		{
			name: "success",
			run: func() error {
				return nil
			},
		},
		{
			name: "error",
			run: func() error {
				return errors.New("synthetic error")
			},
		},
		{
			name: "panic",
			run: func() error {
				panic("synthetic panic")
			},
		},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			_ = runRefreshTransaction(operation.run)

			if !refreshMutex.TryLock() {
				t.Fatal("Transaction retained the mutex")
			}

			refreshMutex.Unlock()
		})
	}
}

func TestRefreshTransactionConcurrentExecution(t *testing.T) {
	const workers = 20
	const iterations = 50

	var active atomic.Int32
	var completed atomic.Int32
	var overlaps atomic.Int32
	var failures atomic.Int32

	var wg sync.WaitGroup

	start := make(chan struct{})

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			for j := 0; j < iterations; j++ {
				err := runRefreshTransaction(func() error {
					if active.Add(1) != 1 {
						overlaps.Add(1)
					}

					defer active.Add(-1)

					time.Sleep(time.Microsecond)

					completed.Add(1)
					return nil
				})

				if err != nil {
					failures.Add(1)
				}
			}
		}()
	}

	close(start)
	wg.Wait()

	if overlaps.Load() != 0 {
		t.Fatalf(
			"Overlapping transactions: %d",
			overlaps.Load(),
		)
	}

	if failures.Load() != 0 {
		t.Fatalf(
			"Failed transactions: %d",
			failures.Load(),
		)
	}

	if completed.Load() != workers*iterations {
		t.Fatalf(
			"Unexpected completion count: %d",
			completed.Load(),
		)
	}

	if active.Load() != 0 {
		t.Fatal("Active transactions remained")
	}

	t.Logf(
		"Completed %d transactions without overlap",
		completed.Load(),
	)
}

func TestRefreshTransactionExistingOwnership(t *testing.T) {
	refreshMutex.Lock()
	defer refreshMutex.Unlock()

	called := false

	err := runRefreshTransactionLocked(func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if !called {
		t.Fatal("Internal operation was not executed")
	}

	if refreshMutex.TryLock() {
		refreshMutex.Unlock()
		t.Fatal("Internal executor released caller mutex")
	}
}

func TestRefreshTransactionLockedFailure(t *testing.T) {
	refreshMutex.Lock()
	defer refreshMutex.Unlock()

	expected := errors.New("synthetic locked error")

	err := runRefreshTransactionLocked(func() error {
		return expected
	})

	if !errors.Is(err, expected) {
		t.Fatalf("Original error was not preserved: %v", err)
	}

	err = runRefreshTransactionLocked(func() error {
		panic("synthetic locked panic")
	})

	if err == nil {
		t.Fatal("Expected panic error")
	}

	if refreshMutex.TryLock() {
		refreshMutex.Unlock()
		t.Fatal("Caller mutex was unexpectedly released")
	}
}

func TestRefreshTransactionNilOperation(t *testing.T) {
	if err := runRefreshTransaction(nil); err == nil {
		t.Fatal("Expected nil operation rejection")
	}

	if !refreshMutex.TryLock() {
		t.Fatal("Nil operation retained the mutex")
	}

	defer refreshMutex.Unlock()

	if err := runRefreshTransactionLocked(nil); err == nil {
		t.Fatal("Expected nil locked operation rejection")
	}
}

func TestRefreshTransactionRepeatedExecution(t *testing.T) {
	const iterations = 1000

	for i := 0; i < iterations; i++ {
		err := runRefreshTransaction(func() error {
			return nil
		})

		if err != nil {
			t.Fatalf("Iteration %d failed: %v", i, err)
		}
	}

	if !refreshMutex.TryLock() {
		t.Fatal("Repeated execution retained the mutex")
	}

	refreshMutex.Unlock()
}
