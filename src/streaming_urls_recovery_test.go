package src

import (
    "reflect"
    "sync"
    "testing"
)

func setupStreamingURLRecoveryTest(t *testing.T) {

    t.Helper()

    streamingURLsSaveMu.Lock()
    streamingURLsMu.Lock()

    originalURLs := Data.Cache.StreamingURLS
    originalAges := streamingURLsAge
    originalGeneration := streamingURLsGen

    Data.Cache.StreamingURLS = make(map[string]StreamInfo)
    streamingURLsAge = make(map[string]uint64)
    streamingURLsGen = 0

    streamingURLsMu.Unlock()
    streamingURLsSaveMu.Unlock()

    t.Cleanup(func() {

        streamingURLsSaveMu.Lock()
        defer streamingURLsSaveMu.Unlock()

        streamingURLsMu.Lock()
        defer streamingURLsMu.Unlock()

        Data.Cache.StreamingURLS = originalURLs
        streamingURLsAge = originalAges
        streamingURLsGen = originalGeneration
    })
}

func TestStreamingURLRecoveryRestore(t *testing.T) {

    setupStreamingURLRecoveryTest(t)

    streamingURLsMu.Lock()

    Data.Cache.StreamingURLS["channel-1"] = StreamInfo{
        BackupChannel1: &BackupStream{},
        BackupChannel2: &BackupStream{},
        BackupChannel3: &BackupStream{},
    }

    Data.Cache.StreamingURLS["channel-2"] = StreamInfo{}

    streamingURLsAge["channel-1"] = 7
    streamingURLsAge["channel-2"] = 9
    streamingURLsGen = 10

    streamingURLsMu.Unlock()

    snapshot := captureStreamingURLRecoverySnapshot()

    streamingURLsMu.Lock()

    delete(Data.Cache.StreamingURLS, "channel-1")
    delete(Data.Cache.StreamingURLS, "channel-2")

    Data.Cache.StreamingURLS["channel-3"] = StreamInfo{}

    streamingURLsAge = map[string]uint64{
        "channel-3": 20,
    }

    streamingURLsGen = 20

    streamingURLsMu.Unlock()

    if err := snapshot.Restore(); err != nil {
        t.Fatal(err)
    }

    restored := captureStreamingURLRecoverySnapshot()

    if !reflect.DeepEqual(snapshot, restored) {
        t.Fatal("Restored streaming URL state does not match snapshot")
    }

    if restored.Generation != 10 {
        t.Fatal("Generation was not restored")
    }

    if restored.Ages["channel-1"] != 7 ||
        restored.Ages["channel-2"] != 9 {

        t.Fatal("Entry ages were not restored")
    }

    if _, exists := restored.URLs["channel-3"]; exists {
        t.Fatal("Unexpected channel remained after rollback")
    }
}

func TestStreamingURLRecoveryPointerIndependence(t *testing.T) {

    original := StreamInfo{
        BackupChannel1: &BackupStream{},
        BackupChannel2: &BackupStream{},
        BackupChannel3: &BackupStream{},
    }

    cloned := cloneRecoveryStreamInfo(original)

    if !reflect.DeepEqual(original, cloned) {
        t.Fatal("Cloned stream differs from original")
    }

    if original.BackupChannel1 == cloned.BackupChannel1 {
        t.Fatal("Backup channel 1 was not independently copied")
    }

    if original.BackupChannel2 == cloned.BackupChannel2 {
        t.Fatal("Backup channel 2 was not independently copied")
    }

    if original.BackupChannel3 == cloned.BackupChannel3 {
        t.Fatal("Backup channel 3 was not independently copied")
    }
}

func TestStreamingURLRecoverySnapshotIndependence(t *testing.T) {

    setupStreamingURLRecoveryTest(t)

    streamingURLsMu.Lock()

    Data.Cache.StreamingURLS["original"] = StreamInfo{
        BackupChannel1: &BackupStream{},
    }

    streamingURLsAge["original"] = 5
    streamingURLsGen = 6

    streamingURLsMu.Unlock()

    snapshot := captureStreamingURLRecoverySnapshot()

    streamingURLsMu.Lock()

    delete(Data.Cache.StreamingURLS, "original")

    Data.Cache.StreamingURLS["replacement"] = StreamInfo{}

    streamingURLsAge["original"] = 99
    streamingURLsGen = 100

    streamingURLsMu.Unlock()

    if _, exists := snapshot.URLs["original"]; !exists {
        t.Fatal("Snapshot lost original channel")
    }

    if _, exists := snapshot.URLs["replacement"]; exists {
        t.Fatal("Snapshot contains subsequent changes")
    }

    if snapshot.Ages["original"] != 5 {
        t.Fatal("Snapshot entry age was modified")
    }

    if snapshot.Generation != 6 {
        t.Fatal("Snapshot generation was modified")
    }

    if err := snapshot.Restore(); err != nil {
        t.Fatal(err)
    }

    streamingURLsMu.Lock()

    restored := Data.Cache.StreamingURLS["original"]

    sharedPointer :=
        restored.BackupChannel1 ==
        snapshot.URLs["original"].BackupChannel1

    delete(Data.Cache.StreamingURLS, "original")
    streamingURLsAge["original"] = 200

    streamingURLsMu.Unlock()

    if sharedPointer {
        t.Fatal("Restored stream shares snapshot pointer")
    }

    if err := snapshot.Restore(); err != nil {
        t.Fatal(err)
    }

    repeated := captureStreamingURLRecoverySnapshot()

    if !reflect.DeepEqual(snapshot, repeated) {
        t.Fatal("Repeated restoration changed snapshot state")
    }
}

func TestStreamingURLRecoveryNilMaps(t *testing.T) {

    setupStreamingURLRecoveryTest(t)

    streamingURLsMu.Lock()

    Data.Cache.StreamingURLS = nil
    streamingURLsAge = nil
    streamingURLsGen = 3

    streamingURLsMu.Unlock()

    snapshot := captureStreamingURLRecoverySnapshot()

    if snapshot.URLs != nil || snapshot.Ages != nil {
        t.Fatal("Nil maps were not preserved")
    }

    streamingURLsMu.Lock()

    Data.Cache.StreamingURLS = make(map[string]StreamInfo)
    streamingURLsAge = make(map[string]uint64)
    streamingURLsGen = 10

    streamingURLsMu.Unlock()

    if err := snapshot.Restore(); err != nil {
        t.Fatal(err)
    }

    restored := captureStreamingURLRecoverySnapshot()

    if !reflect.DeepEqual(snapshot, restored) {
        t.Fatal("Nil map restoration failed")
    }
}

func TestStreamingURLRecoveryEmptyMaps(t *testing.T) {

    setupStreamingURLRecoveryTest(t)

    snapshot := captureStreamingURLRecoverySnapshot()

    if snapshot.URLs == nil || snapshot.Ages == nil {
        t.Fatal("Empty maps were incorrectly converted to nil")
    }

    if len(snapshot.URLs) != 0 || len(snapshot.Ages) != 0 {
        t.Fatal("Expected empty maps")
    }

    if err := snapshot.Restore(); err != nil {
        t.Fatal(err)
    }

    restored := captureStreamingURLRecoverySnapshot()

    if !reflect.DeepEqual(snapshot, restored) {
        t.Fatal("Empty map restoration failed")
    }
}

func TestStreamingURLRecoveryNilPointers(t *testing.T) {

    original := StreamInfo{}

    cloned := cloneRecoveryStreamInfo(original)

    if cloned.BackupChannel1 != nil ||
        cloned.BackupChannel2 != nil ||
        cloned.BackupChannel3 != nil {

        t.Fatal("Nil backup pointers were not preserved")
    }

    if cloneRecoveryBackupStream(nil) != nil {
        t.Fatal("Nil backup stream was not preserved")
    }
}

func TestStreamingURLRecoveryNilSnapshot(t *testing.T) {

    var snapshot *streamingURLRecoverySnapshot

    if err := snapshot.Restore(); err == nil {
        t.Fatal("Nil snapshot was accepted")
    }
}

func TestStreamingURLRecoveryConcurrentOperations(t *testing.T) {

    setupStreamingURLRecoveryTest(t)

    streamingURLsMu.Lock()

    Data.Cache.StreamingURLS["channel"] = StreamInfo{
        BackupChannel1: &BackupStream{},
    }

    streamingURLsAge["channel"] = 1
    streamingURLsGen = 1

    streamingURLsMu.Unlock()

    original := captureStreamingURLRecoverySnapshot()

    var workers sync.WaitGroup

    for i := 0; i < 20; i++ {

        workers.Add(1)

        go func() {

            defer workers.Done()

            for j := 0; j < 50; j++ {

                snapshot :=
                    captureStreamingURLRecoverySnapshot()

                if err := snapshot.Restore(); err != nil {
                    t.Error(err)
                    return
                }
            }
        }()
    }

    workers.Wait()

    restored := captureStreamingURLRecoverySnapshot()

    if !reflect.DeepEqual(original, restored) {
        t.Fatal("Concurrent operations changed original state")
    }
}