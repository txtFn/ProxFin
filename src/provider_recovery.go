package src

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "path/filepath"
    "sync"
    "time"
)

const providerRecoveryStatusFile = ".provider-recovery.json"

const (
    providerRecoveryInitializing = "initializing"
    providerRecoveryRecovering   = "recovering"
    providerRecoveryHealthy      = "healthy"
    providerRecoveryFailed       = "failed"
)

var providerRecoveryMutex sync.Mutex

type providerRecoveryStatus struct {
    Version       int    `json:"version"`
    State         string `json:"state"`
    Generation    string `json:"generation"`
    BackupAllowed bool   `json:"backup_allowed"`
    UpdatedAt     string `json:"updated_at"`
}

func writeProviderRecoveryStatus(state string) error {

    providerRecoveryMutex.Lock()
    defer providerRecoveryMutex.Unlock()

    switch state {
    case providerRecoveryInitializing,
        providerRecoveryRecovering,
        providerRecoveryHealthy,
        providerRecoveryFailed:
    default:
        return fmt.Errorf("invalid provider recovery state")
    }

    if System.Folder.Config == "" {
        return fmt.Errorf("provider recovery configuration directory is missing")
    }

    var randomBytes [16]byte

    if _, err := rand.Read(randomBytes[:]); err != nil {
        return fmt.Errorf(
            "provider recovery generation creation failed: %w",
            err,
        )
    }

    status := providerRecoveryStatus{
        Version:       1,
        State:         state,
        Generation:    hex.EncodeToString(randomBytes[:]),
        BackupAllowed: state == providerRecoveryHealthy,
        UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
    }

    path := filepath.Join(
        System.Folder.Config,
        providerRecoveryStatusFile,
    )

    if err := saveMapToJSONFile(path, status); err != nil {
        return fmt.Errorf(
            "provider recovery status save failed: %w",
            err,
        )
    }

    return nil
}
