package src

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func setupCombinedRecoveryTest(
	t *testing.T,
) (string, string) {

	t.Helper()

	setupChannelMappingRecoveryTest(t)
	setupStreamingURLRecoveryTest(t)

	root := t.TempDir()
	path := filepath.Join(root, "xepg.json")

	if err := os.WriteFile(
		path,
		[]byte("original"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	Data.XEPG.Channels["original"] =
		XEPGChannelStruct{
			XActive: true,
			XName:   "Original channel",
		}

	Data.XEPG.XEPGCount = 1

	Data.Cache.Streams.Active =
		[]string{"original-hash"}

	streamingURLsMu.Lock()

	Data.Cache.StreamingURLS["original"] =
		StreamInfo{}

	streamingURLsAge["original"] = 5
	streamingURLsGen = 6

	streamingURLsMu.Unlock()

	return root, path
}

func mutateCombinedRecoveryTest(
	t *testing.T,
	path string,
) {

	t.Helper()

	if err := os.WriteFile(
		path,
		[]byte("modified"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	Data.XEPG.Channels =
		map[string]interface{}{
			"replacement": XEPGChannelStruct{},
		}

	Data.XEPG.XEPGCount = 99

	Data.Cache.Streams.Active =
		[]string{"modified-hash"}

	streamingURLsMu.Lock()

	Data.Cache.StreamingURLS =
		map[string]StreamInfo{
			"replacement": StreamInfo{},
		}

	streamingURLsAge =
		map[string]uint64{
			"replacement": 99,
		}

	streamingURLsGen = 100

	streamingURLsMu.Unlock()
}

func assertCombinedRecoveryDisk(
	t *testing.T,
	path string,
	expected string,
) {

	t.Helper()

	content, err := os.ReadFile(path)

	if err != nil {
		t.Fatal(err)
	}

	if string(content) != expected {
		t.Fatal("Unexpected disk state")
	}
}

func assertCombinedRecoveryMemory(
	t *testing.T,
	snapshot *combinedXEPGRecoverySnapshot,
) {

	t.Helper()

	mapping, err :=
		captureChannelMappingRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(
		snapshot.Mapping,
		mapping,
	) {
		t.Fatal("Channel mapping restoration failed")
	}

	streaming :=
		captureStreamingURLRecoverySnapshot()

	if !reflect.DeepEqual(
		snapshot.Streaming,
		streaming,
	) {
		t.Fatal("Streaming URL restoration failed")
	}
}

func TestCombinedXEPGRecoveryRestore(t *testing.T) {

	root, path := setupCombinedRecoveryTest(t)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root,
			[]string{path},
		)

	if err != nil {
		t.Fatal(err)
	}

	mutateCombinedRecoveryTest(t, path)

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	assertCombinedRecoveryDisk(
		t, path, "original",
	)

	assertCombinedRecoveryMemory(t, snapshot)

	if snapshot.restorationFailed {
		t.Fatal("Successful restoration marked as failed")
	}

	mutateCombinedRecoveryTest(t, path)

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	assertCombinedRecoveryDisk(
		t, path, "original",
	)

	assertCombinedRecoveryMemory(t, snapshot)

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}
}

func TestCombinedXEPGRecoveryDiscard(t *testing.T) {

	root, path := setupCombinedRecoveryTest(t)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root,
			[]string{path},
		)

	if err != nil {
		t.Fatal(err)
	}

	directory := snapshot.Disk.Directory

	mutateCombinedRecoveryTest(t, path)

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}

	if !snapshot.discarded {
		t.Fatal("Snapshot was not marked discarded")
	}

	if snapshot.Disk != nil ||
		snapshot.Streaming != nil ||
		snapshot.Mapping != nil {

		t.Fatal("Snapshot references were retained")
	}

	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("Snapshot directory was not removed")
	}

	assertCombinedRecoveryDisk(
		t, path, "modified",
	)

	if Data.XEPG.XEPGCount != 99 {
		t.Fatal("Discard unexpectedly restored memory")
	}

	if err := snapshot.Discard(); err != nil {
		t.Fatal("Repeated discard failed")
	}

	if err := snapshot.Restore(); err == nil {
		t.Fatal("Discarded snapshot was restored")
	}
}

func TestCombinedXEPGRecoveryDiskCaptureFailure(
	t *testing.T,
) {

	root, path := setupCombinedRecoveryTest(t)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root,
			[]string{path, path},
		)

	if err == nil || snapshot != nil {
		t.Fatal("Invalid disk snapshot was accepted")
	}

	if !strings.Contains(
		err.Error(),
		"Combined disk snapshot failed",
	) {
		t.Fatal(err)
	}

	assertCombinedRecoveryDisk(
		t, path, "original",
	)
}

func TestCombinedXEPGRecoveryMappingCaptureFailure(
	t *testing.T,
) {

	root, path := setupCombinedRecoveryTest(t)

	Data.XEPG.Channels["invalid"] =
		make(chan int)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root,
			[]string{path},
		)

	if err == nil || snapshot != nil {
		t.Fatal("Invalid mapping snapshot was accepted")
	}

	if !strings.Contains(
		err.Error(),
		"Combined channel mapping snapshot failed",
	) {
		t.Fatal(err)
	}

	leftovers, err := filepath.Glob(
		filepath.Join(root, ".xepg-rollback-*"),
	)

	if err != nil {
		t.Fatal(err)
	}

	if len(leftovers) != 0 {
		t.Fatal("Incomplete disk snapshot was retained")
	}

	assertCombinedRecoveryDisk(
		t, path, "original",
	)
}

func TestCombinedXEPGRecoveryDiskFailureRetry(
	t *testing.T,
) {

	root, path := setupCombinedRecoveryTest(t)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root,
			[]string{path},
		)

	if err != nil {
		t.Fatal(err)
	}

	backup := snapshot.Disk.Files[0].Backup

	saved, err := os.ReadFile(backup)

	if err != nil {
		t.Fatal(err)
	}

	mutateCombinedRecoveryTest(t, path)

	if err := os.Remove(backup); err != nil {
		t.Fatal(err)
	}

	err = snapshot.Restore()

	if err == nil {
		t.Fatal("Missing backup was accepted")
	}

	if !strings.Contains(
		err.Error(),
		"Combined disk restoration failed",
	) {
		t.Fatal(err)
	}

	if !snapshot.restorationFailed {
		t.Fatal("Restoration failure was not recorded")
	}

	assertCombinedRecoveryMemory(t, snapshot)

	assertCombinedRecoveryDisk(
		t, path, "modified",
	)

	if err := snapshot.Discard(); err == nil {
		t.Fatal("Failed recovery snapshot was discarded")
	}

	if _, err := os.Stat(
		snapshot.Disk.Directory,
	); err != nil {
		t.Fatal("Recovery directory was lost")
	}

	if err := os.WriteFile(
		backup,
		saved,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	assertCombinedRecoveryDisk(
		t, path, "original",
	)

	assertCombinedRecoveryMemory(t, snapshot)

	if snapshot.restorationFailed {
		t.Fatal("Successful retry did not clear failure")
	}

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}
}

func TestCombinedXEPGRecoveryMappingFailureRetry(
	t *testing.T,
) {

	root, path := setupCombinedRecoveryTest(t)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root,
			[]string{path},
		)

	if err != nil {
		t.Fatal(err)
	}

	mutateCombinedRecoveryTest(t, path)

	snapshot.Mapping.Channels["invalid"] =
		make(chan int)

	err = snapshot.Restore()

	if err == nil {
		t.Fatal("Invalid mapping restoration succeeded")
	}

	if !strings.Contains(
		err.Error(),
		"Combined mapping restoration failed",
	) {
		t.Fatal(err)
	}

	assertCombinedRecoveryDisk(
		t, path, "original",
	)

	streaming :=
		captureStreamingURLRecoverySnapshot()

	if !reflect.DeepEqual(
		snapshot.Streaming,
		streaming,
	) {
		t.Fatal("Streaming state was not restored")
	}

	if Data.XEPG.XEPGCount != 99 {
		t.Fatal("Failed mapping restoration changed state")
	}

	if err := snapshot.Discard(); err == nil {
		t.Fatal("Failed snapshot was discarded")
	}

	delete(snapshot.Mapping.Channels, "invalid")

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	assertCombinedRecoveryMemory(t, snapshot)

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}
}

func TestCombinedXEPGRecoveryMultipleFailures(
	t *testing.T,
) {

	root, path := setupCombinedRecoveryTest(t)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root,
			[]string{path},
		)

	if err != nil {
		t.Fatal(err)
	}

	mutateCombinedRecoveryTest(t, path)

	backup := snapshot.Disk.Files[0].Backup

	saved, err := os.ReadFile(backup)

	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(backup); err != nil {
		t.Fatal(err)
	}

	snapshot.Mapping.Channels["invalid"] =
		make(chan int)

	err = snapshot.Restore()

	if err == nil {
		t.Fatal("Multiple restoration failures ignored")
	}

	for _, expected := range []string{
		"Combined disk restoration failed",
		"Combined mapping restoration failed",
	} {

		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("Missing recovery error: %s", expected)
		}
	}

	streaming :=
		captureStreamingURLRecoverySnapshot()

	if !reflect.DeepEqual(
		snapshot.Streaming,
		streaming,
	) {
		t.Fatal("Streaming restoration was skipped")
	}

	if err := snapshot.Discard(); err == nil {
		t.Fatal("Failed snapshot was discarded")
	}

	if err := os.WriteFile(
		backup,
		saved,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	delete(snapshot.Mapping.Channels, "invalid")

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	assertCombinedRecoveryDisk(
		t, path, "original",
	)

	assertCombinedRecoveryMemory(t, snapshot)

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}
}

func TestCombinedXEPGRecoveryInvalidSnapshot(
	t *testing.T,
) {

	cases := []struct {
		name     string
		snapshot *combinedXEPGRecoverySnapshot
	}{
		{
			name:     "nil",
			snapshot: nil,
		},
		{
			name:     "missing-disk",
			snapshot: &combinedXEPGRecoverySnapshot{},
		},
		{
			name: "missing-streaming",
			snapshot: &combinedXEPGRecoverySnapshot{
				Disk: &xepgRecoverySnapshot{},
			},
		},
		{
			name: "missing-mapping",
			snapshot: &combinedXEPGRecoverySnapshot{
				Disk:      &xepgRecoverySnapshot{},
				Streaming: &streamingURLRecoverySnapshot{},
			},
		},
	}

	for _, tc := range cases {

		t.Run(tc.name, func(t *testing.T) {

			if err := tc.snapshot.Restore(); err == nil {
				t.Fatal("Invalid restoration was accepted")
			}

			if err := tc.snapshot.Discard(); err == nil {
				t.Fatal("Invalid discard was accepted")
			}
		})
	}
}
