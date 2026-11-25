package src

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func fullRecoveryFileHash(
	t *testing.T,
	path string,
) [32]byte {

	t.Helper()

	content, err := os.ReadFile(path)

	if err != nil {
		t.Fatal(err)
	}

	return sha256.Sum256(content)
}

func setupFullRecoveryMemoryTest(
	t *testing.T,
	count int,
) (string, []string) {

	t.Helper()

	setupExpandedRecoveryGlobals(t)

	paths := populateRecoveryMemoryTest(t, count)

	root := filepath.Dir(paths[0])

	fixture := expandedMemoryFixture(count)

	if err := fixture.Restore(); err != nil {
		t.Fatal(err)
	}

	settingsPath := filepath.Join(root, "settings.json")

	settingsJSON, err := json.Marshal(Settings)

	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		settingsPath, settingsJSON, 0600,
	); err != nil {
		t.Fatal(err)
	}

	paths = append(paths, settingsPath)

	if len(paths) != 6 {
		t.Fatal("Expected six protected files")
	}

	return root, paths
}

func assertFullRecoveryMemoryCounts(
	t *testing.T,
	count int,
) {

	t.Helper()

	if len(Data.XEPG.Channels) != count {
		t.Fatal("Channel mapping count mismatch")
	}

	if len(Data.XEPG.ChannelsFiltered) != count {
		t.Fatal("Filtered mapping count mismatch")
	}

	if len(Data.XMLTV.Mapping) != count {
		t.Fatal("EPG mapping count mismatch")
	}

	if len(Data.Cache.Streams.Active) != count {
		t.Fatal("Active cache count mismatch")
	}

	if Data.XEPG.XEPGCount != int64(count) {
		t.Fatal("XEPG count mismatch")
	}

	streamingURLsMu.RLock()

	streamCount := len(Data.Cache.StreamingURLS)
	ageCount := len(streamingURLsAge)

	streamingURLsMu.RUnlock()

	if streamCount != count || ageCount != count {
		t.Fatal("Streaming reference count mismatch")
	}

	if len(Data.Streams.All) != count {
		t.Fatal("Expanded stream count mismatch")
	}

	if len(Data.Streams.Active)+
		len(Data.Streams.Inactive) != count {

		t.Fatal("Stream classification mismatch")
	}

	if len(Data.StreamPreviewUI.Active)+
		len(Data.StreamPreviewUI.Inactive) != count {

		t.Fatal("Preview count mismatch")
	}

	if Settings.UUID != "test-original-uuid" {
		t.Fatal("Settings UUID mismatch")
	}

	if len(Settings.Filter) == 0 ||
		len(Data.Filter) == 0 {

		t.Fatal("Filter state missing")
	}
}

func mutateFullRecoveryMemoryTest(
	t *testing.T,
	paths []string,
) {

	t.Helper()

	mutateCombinedRecoveryTest(t, paths[0])

	Data.XEPG.ChannelsFiltered = nil
	Data.XMLTV.Mapping = nil
	Data.XMLTV.Files = nil

	providerMutex.Lock()

	Settings.UUID = "modified-uuid"

	Settings.Files.M3U["provider"].(map[string]interface{})["name"] =
		"modified-provider"

	Settings.Filter = nil

	Data.Streams.All = nil
	Data.Streams.Active = nil
	Data.Streams.Inactive = nil

	Data.Filter = nil

	Data.Playlist.M3U.Groups.Text = nil
	Data.Playlist.M3U.Groups.Value = nil

	Data.StreamPreviewUI.Active = nil
	Data.StreamPreviewUI.Inactive = nil

	System.Folder.Temp = "modified-temp"
	System.DeviceID = "modified-device"

	providerMutex.Unlock()

	for _, path := range paths {

		if err := os.WriteFile(
			path, []byte("modified"), 0600,
		); err != nil {
			t.Fatal(err)
		}
	}
}

func assertFullRecoveryMemoryState(
	t *testing.T,
	snapshot *combinedXEPGRecoverySnapshot,
	count int,
) {

	t.Helper()

	assertFullRecoveryMemoryCounts(t, count)

	assertCombinedRecoveryMemory(t, snapshot)

	expanded, err :=
		captureXEPGExpandedStateRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(
		expanded, snapshot.Expanded,
	) {
		t.Fatal("Expanded state restoration mismatch")
	}

	for i, item := range Data.Streams.All {

		expected := fmt.Sprintf("channel-%06d", i)

		stream := item.(map[string]string)

		if stream["name"] != expected {
			t.Fatalf("Stream %d restoration mismatch", i)
		}
	}

	Data.Streams.All[0].(map[string]string)["name"] =
		"inactive-alias-check"

	if Data.Streams.Inactive[0].(map[string]string)["name"] !=
		"inactive-alias-check" {

		t.Fatal("Inactive stream reference was lost")
	}

	Data.Streams.All[0].(map[string]string)["name"] =
		"channel-000000"

	Data.Streams.All[1].(map[string]string)["name"] =
		"active-alias-check"

	if Data.Streams.Active[0].(map[string]string)["name"] !=
		"active-alias-check" {

		t.Fatal("Active stream reference was lost")
	}

	Data.Streams.All[1].(map[string]string)["name"] =
		"channel-000001"

	if snapshot.Expanded.AllStreams[0].(map[string]string)["name"] !=
		"channel-000000" {

		t.Fatal("Restored state aliases snapshot")
	}
}

func TestXEPGFullRecoveryMemoryScale(t *testing.T) {

	if testing.Short() {
		t.Skip("Full combined memory benchmark")
	}

	counts := []int{424, 5000, 12268, 15000}

	for _, count := range counts {

		t.Run(
			fmt.Sprintf("Channels_%d", count),
			func(t *testing.T) {

				root, paths :=
					setupFullRecoveryMemoryTest(t, count)

				assertFullRecoveryMemoryCounts(t, count)

				hashes := make(map[string][32]byte)

				var diskBytes int64

				for _, path := range paths {

					hashes[path] =
						fullRecoveryFileHash(t, path)

					info, err := os.Stat(path)

					if err != nil {
						t.Fatal(err)
					}

					diskBytes += info.Size()
				}

				runtime.GC()

				baseline := readRecoveryMemory()

				captureStart := time.Now()

				snapshot, err :=
					captureCombinedXEPGRecoverySnapshot(
						root, paths,
					)

				captureDuration :=
					time.Since(captureStart)

				if err != nil {
					t.Fatal(err)
				}

				afterCapture := readRecoveryMemory()

				if snapshot.Disk == nil ||
					snapshot.Mapping == nil ||
					snapshot.Streaming == nil ||
					snapshot.Expanded == nil {

					t.Fatal("Incomplete combined snapshot")
				}

				if len(snapshot.Disk.Files) != 6 {
					t.Fatal("Incomplete disk coverage")
				}

				if len(snapshot.Expanded.AllStreams) != count {
					t.Fatal("Incomplete expanded snapshot")
				}

				directory := snapshot.Disk.Directory

				runtime.GC()

				retained := readRecoveryMemory()

				mutateFullRecoveryMemoryTest(t, paths)

				beforeRestore := readRecoveryMemory()

				restoreStart := time.Now()

				if err := snapshot.Restore(); err != nil {
					t.Fatal(err)
				}

				restoreDuration :=
					time.Since(restoreStart)

				afterRestore := readRecoveryMemory()

				assertFullRecoveryMemoryState(
					t, snapshot, count,
				)

				for _, path := range paths {

					if fullRecoveryFileHash(t, path) !=
						hashes[path] {

						t.Fatalf(
							"Disk restoration failed: %s",
							filepath.Base(path),
						)
					}
				}

				afterValidation := readRecoveryMemory()

				runtime.GC()

				retainedAfterRestore :=
					readRecoveryMemory()

				discardStart := time.Now()

				if err := snapshot.Discard(); err != nil {
					t.Fatal(err)
				}

				discardDuration :=
					time.Since(discardStart)

				if snapshot.Disk != nil ||
					snapshot.Mapping != nil ||
					snapshot.Streaming != nil ||
					snapshot.Expanded != nil {

					t.Fatal("Snapshot references retained")
				}

				if _, err := os.Stat(directory); !os.IsNotExist(err) {
					t.Fatal("Snapshot directory was not removed")
				}

				snapshot = nil

				runtime.GC()

				afterDiscard := readRecoveryMemory()

				t.Logf("CHANNELS: %d", count)

				t.Logf("PROTECTED_FILES: %d", len(paths))

				t.Logf(
					"PROTECTED_DISK_MIB: %.2f",
					float64(diskBytes)/1048576,
				)

				t.Logf(
					"BASELINE_HEAP_MIB: %.2f",
					recoveryMemoryMiB(baseline.HeapAlloc),
				)

				t.Logf(
					"CAPTURE_HEAP_MIB: %.2f",
					recoveryMemoryMiB(afterCapture.HeapAlloc),
				)

				t.Logf(
					"RETAINED_CAPTURE_HEAP_MIB: %.2f",
					recoveryMemoryMiB(retained.HeapAlloc),
				)

				t.Logf(
					"SNAPSHOT_OVERHEAD_MIB: %.2f",
					recoveryMemoryDelta(
						baseline.HeapAlloc,
						retained.HeapAlloc,
					),
				)

				t.Logf(
					"CAPTURE_ALLOCATIONS_MIB: %.2f",
					recoveryMemoryDelta(
						baseline.TotalAlloc,
						afterCapture.TotalAlloc,
					),
				)

				t.Logf(
					"BEFORE_RESTORE_HEAP_MIB: %.2f",
					recoveryMemoryMiB(
						beforeRestore.HeapAlloc,
					),
				)

				t.Logf(
					"RESTORE_HEAP_MIB: %.2f",
					recoveryMemoryMiB(
						afterRestore.HeapAlloc,
					),
				)

				t.Logf(
					"RESTORE_ALLOCATIONS_MIB: %.2f",
					recoveryMemoryDelta(
						beforeRestore.TotalAlloc,
						afterRestore.TotalAlloc,
					),
				)

				t.Logf(
					"VALIDATION_HEAP_MIB: %.2f",
					recoveryMemoryMiB(
						afterValidation.HeapAlloc,
					),
				)

				t.Logf(
					"RETAINED_RESTORE_HEAP_MIB: %.2f",
					recoveryMemoryMiB(
						retainedAfterRestore.HeapAlloc,
					),
				)

				t.Logf(
					"POST_DISCARD_GC_HEAP_MIB: %.2f",
					recoveryMemoryMiB(
						afterDiscard.HeapAlloc,
					),
				)

				t.Logf(
					"GO_SYS_AFTER_RESTORE_MIB: %.2f",
					recoveryMemoryMiB(afterRestore.Sys),
				)

				t.Logf(
					"GO_SYS_AFTER_VALIDATION_MIB: %.2f",
					recoveryMemoryMiB(afterValidation.Sys),
				)

				t.Logf(
					"GO_SYS_AFTER_DISCARD_MIB: %.2f",
					recoveryMemoryMiB(afterDiscard.Sys),
				)

				t.Logf(
					"CAPTURE_DURATION: %s",
					captureDuration,
				)

				t.Logf(
					"RESTORE_DURATION: %s",
					restoreDuration,
				)

				t.Logf(
					"DISCARD_DURATION: %s",
					discardDuration,
				)

				t.Log("PASS: All four components restored")
				t.Log("PASS: Six disk file hashes verified")
				t.Log("PASS: Stream references preserved")
				t.Log("PASS: Snapshot isolation verified")
				t.Log("PASS: Snapshot resources released")
				t.Log("FULL MEMORY BENCHMARK PASS")
			},
		)
	}
}
