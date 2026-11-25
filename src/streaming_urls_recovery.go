package src

import (
    "errors"
    "maps"
)

type streamingURLRecoverySnapshot struct {
    URLs       map[string]StreamInfo
    Ages       map[string]uint64
    Generation uint64
}

func cloneRecoveryBackupStream(
    original *BackupStream,
) *BackupStream {

    if original == nil {
        return nil
    }

    cloned := *original

    return &cloned
}

func cloneRecoveryStreamInfo(
    original StreamInfo,
) StreamInfo {

    cloned := original

    cloned.BackupChannel1 =
        cloneRecoveryBackupStream(
            original.BackupChannel1,
        )

    cloned.BackupChannel2 =
        cloneRecoveryBackupStream(
            original.BackupChannel2,
        )

    cloned.BackupChannel3 =
        cloneRecoveryBackupStream(
            original.BackupChannel3,
        )

    return cloned
}

func cloneRecoveryStreamingURLs(
    original map[string]StreamInfo,
) map[string]StreamInfo {

    if original == nil {
        return nil
    }

    cloned := make(
        map[string]StreamInfo,
        len(original),
    )

    for id, stream := range original {
        cloned[id] = cloneRecoveryStreamInfo(stream)
    }

    return cloned
}

func captureStreamingURLRecoverySnapshot() *streamingURLRecoverySnapshot {

    streamingURLsSaveMu.Lock()
    defer streamingURLsSaveMu.Unlock()

    streamingURLsMu.RLock()
    defer streamingURLsMu.RUnlock()

    return &streamingURLRecoverySnapshot{
        URLs: cloneRecoveryStreamingURLs(
            Data.Cache.StreamingURLS,
        ),

        Ages: maps.Clone(
            streamingURLsAge,
        ),

        Generation: streamingURLsGen,
    }
}

func (snapshot *streamingURLRecoverySnapshot) Restore() error {

    if snapshot == nil {
        return errors.New(
            "Streaming URL recovery snapshot is missing",
        )
    }

    streamingURLsSaveMu.Lock()
    defer streamingURLsSaveMu.Unlock()

    streamingURLsMu.Lock()
    defer streamingURLsMu.Unlock()

    Data.Cache.StreamingURLS =
        cloneRecoveryStreamingURLs(
            snapshot.URLs,
        )

    streamingURLsAge = maps.Clone(
        snapshot.Ages,
    )

    streamingURLsGen = snapshot.Generation

    return nil
}