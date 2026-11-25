package src

import (
	"path/filepath"
	"testing"
	"time"
)

func startupRuntimeAssertUnlocked(t *testing.T) {

	t.Helper()

	if !refreshMutex.TryLock() {
		t.Fatal("Refresh mutex was not released")
	}

	refreshMutex.Unlock()
}

func TestStartupRuntimeInvalidWebRestore(t *testing.T) {

	_, err := ThreadfinRestoreFromWeb(
		"%%%invalid-base64%%%",
	)

	if err == nil {
		t.Fatal("Invalid restoration input was accepted")
	}

	startupRuntimeAssertUnlocked(t)
}

func TestStartupRuntimeLockedWebRestore(t *testing.T) {

	refreshMutex.Lock()
	defer refreshMutex.Unlock()

	_, err := threadfinRestoreFromWebLocked(
		"%%%invalid-base64%%%",
	)

	if err == nil {
		t.Fatal("Invalid restoration input was accepted")
	}

	if refreshMutex.TryLock() {
		refreshMutex.Unlock()
		t.Fatal("Internal restore released caller's lock")
	}
}

func TestStartupRuntimeMissingArchive(t *testing.T) {

	archive := filepath.Join(
		t.TempDir(),
		"missing-backup.zip",
	)

	_, err := ThreadfinRestore(archive)

	if err == nil {
		t.Fatal("Missing archive was accepted")
	}

	startupRuntimeAssertUnlocked(t)
}

func TestStartupRuntimeLockedMissingArchive(t *testing.T) {

	archive := filepath.Join(
		t.TempDir(),
		"missing-backup.zip",
	)

	refreshMutex.Lock()
	defer refreshMutex.Unlock()

	_, err := threadfinRestoreLocked(archive)

	if err == nil {
		t.Fatal("Missing archive was accepted")
	}

	if refreshMutex.TryLock() {
		refreshMutex.Unlock()
		t.Fatal("Internal restore released caller's lock")
	}
}

func TestStartupRuntimeWebRestoreWaitsForLock(t *testing.T) {

	refreshMutex.Lock()

	started := make(chan struct{})
	result := make(chan error, 1)

	go func() {

		close(started)

		_, err := ThreadfinRestoreFromWeb(
			"%%%invalid-base64%%%",
		)

		result <- err
	}()

	<-started

	select {

	case <-result:

		refreshMutex.Unlock()

		t.Fatal("Public restore returned while lock was held")

	case <-time.After(50 * time.Millisecond):

	}

	refreshMutex.Unlock()

	select {

	case err := <-result:

		if err == nil {
			t.Fatal("Invalid restoration input was accepted")
		}

	case <-time.After(5 * time.Second):

		t.Fatal("Restore did not finish after lock release")
	}

	startupRuntimeAssertUnlocked(t)
}

func TestStartupRuntimeRepeatedRestoreFailures(t *testing.T) {

	for i := 0; i < 100; i++ {

		_, err := ThreadfinRestoreFromWeb(
			"%%%invalid-base64%%%",
		)

		if err == nil {
			t.Fatalf("Invalid input accepted at iteration %d", i)
		}

		startupRuntimeAssertUnlocked(t)
	}

	t.Log("Completed 100 failed restores without retaining the mutex")
}
