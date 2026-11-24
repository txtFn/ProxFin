package src

import (
    "encoding/hex"
    "encoding/json"
    "os"
    "path/filepath"
    "sync"
    "testing"
    "time"
)

func setupProviderRecoveryTest(t *testing.T) string {
    t.Helper()

    original := System.Folder.Config
    dir := t.TempDir()

    System.Folder.Config = dir + string(os.PathSeparator)

    t.Cleanup(func() {
        System.Folder.Config = original
    })

    return dir
}

func readProviderRecoveryTestStatus(
    t *testing.T,
    dir string,
) providerRecoveryStatus {
    t.Helper()

    data, err := os.ReadFile(
        filepath.Join(dir, providerRecoveryStatusFile),
    )

    if err != nil {
        t.Fatal(err)
    }

    var status providerRecoveryStatus

    if err := json.Unmarshal(data, &status); err != nil {
        t.Fatal(err)
    }

    return status
}

func TestProviderRecoveryStates(t *testing.T) {
    dir := setupProviderRecoveryTest(t)

    cases := []struct {
        state   string
        allowed bool
    }{
        {providerRecoveryInitializing, false},
        {providerRecoveryRecovering, false},
        {providerRecoveryHealthy, true},
        {providerRecoveryFailed, false},
    }

    for _, tc := range cases {
        t.Run(tc.state, func(t *testing.T) {
            if err := writeProviderRecoveryStatus(tc.state); err != nil {
                t.Fatal(err)
            }

            status := readProviderRecoveryTestStatus(t, dir)

            if status.Version != 1 {
                t.Fatalf("Unexpected version: %d", status.Version)
            }

            if status.State != tc.state {
                t.Fatalf("Unexpected state: %s", status.State)
            }

            if status.BackupAllowed != tc.allowed {
                t.Fatalf(
                    "Unexpected backup eligibility: %v",
                    status.BackupAllowed,
                )
            }

            generation, err := hex.DecodeString(status.Generation)

            if err != nil || len(generation) != 16 {
                t.Fatal("Invalid generation identifier")
            }

            if _, err := time.Parse(
                time.RFC3339Nano,
                status.UpdatedAt,
            ); err != nil {
                t.Fatal("Invalid status timestamp")
            }
        })
    }
}

func TestProviderRecoveryInvalidState(t *testing.T) {
    dir := setupProviderRecoveryTest(t)

    if err := writeProviderRecoveryStatus(
        providerRecoveryHealthy,
    ); err != nil {
        t.Fatal(err)
    }

    before := readProviderRecoveryTestStatus(t, dir)

    if err := writeProviderRecoveryStatus("invalid"); err == nil {
        t.Fatal("Invalid state was accepted")
    }

    after := readProviderRecoveryTestStatus(t, dir)

    if before != after {
        t.Fatal("Invalid state modified existing status")
    }
}

func TestProviderRecoveryMissingDirectory(t *testing.T) {
    setupProviderRecoveryTest(t)

    System.Folder.Config = ""

    if err := writeProviderRecoveryStatus(
        providerRecoveryRecovering,
    ); err == nil {
        t.Fatal("Missing configuration directory was accepted")
    }
}

func TestProviderRecoveryGenerationUniqueness(t *testing.T) {
    dir := setupProviderRecoveryTest(t)

    seen := make(map[string]bool)

    for i := 0; i < 100; i++ {
        if err := writeProviderRecoveryStatus(
            providerRecoveryRecovering,
        ); err != nil {
            t.Fatal(err)
        }

        status := readProviderRecoveryTestStatus(t, dir)

        if seen[status.Generation] {
            t.Fatal("Duplicate generation identifier")
        }

        seen[status.Generation] = true
    }
}

func TestProviderRecoveryWriteFailure(t *testing.T) {
    dir := setupProviderRecoveryTest(t)

    invalidPath := filepath.Join(dir, "not-a-directory")

    if err := os.WriteFile(
        invalidPath,
        []byte("test"),
        0600,
    ); err != nil {
        t.Fatal(err)
    }

    System.Folder.Config = invalidPath

    if err := writeProviderRecoveryStatus(
        providerRecoveryRecovering,
    ); err == nil {
        t.Fatal("Expected status write failure")
    }
}

func TestProviderRecoveryConcurrentWrites(t *testing.T) {
    dir := setupProviderRecoveryTest(t)

    states := []string{
        providerRecoveryInitializing,
        providerRecoveryRecovering,
        providerRecoveryHealthy,
        providerRecoveryFailed,
    }

    var wg sync.WaitGroup

    failures := make(chan error, 100)

    for i := 0; i < 100; i++ {
        wg.Add(1)

        go func(state string) {
            defer wg.Done()

            if err := writeProviderRecoveryStatus(state); err != nil {
                failures <- err
            }
        }(states[i % len(states)])
    }

    wg.Wait()
    close(failures)

    for err := range failures {
        t.Error(err)
    }

    status := readProviderRecoveryTestStatus(t, dir)

    validState := false

    for _, state := range states {
        if status.State == state {
            validState = true
            break
        }
    }

    if !validState {
        t.Fatal("Invalid final recovery state")
    }

    if status.BackupAllowed !=
        (status.State == providerRecoveryHealthy) {
        t.Fatal("Inconsistent final backup eligibility")
    }
}
