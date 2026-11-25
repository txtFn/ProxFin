package src

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func setupExpandedRollbackTest(
	t *testing.T,
) (string, []string) {

	t.Helper()

	setupExpandedRecoveryGlobals(t)

	setupCombinedRecoveryTest(t)

	root, paths := setupSettingsDiskRecoveryPaths(t)

	fixture := expandedRecoveryFixture()

	if err := fixture.Restore(); err != nil {
		t.Fatal(err)
	}

	for _, path := range paths {
		writeSnapshotTestFile(
			t, path, "original:"+filepath.Base(path),
		)
	}

	return root, paths
}

func mutateExpandedRollbackTest(
	t *testing.T,
	paths []string,
) {

	t.Helper()

	mutateCombinedRecoveryTest(t, paths[0])

	providerMutex.Lock()

	Settings.UUID = "modified-uuid"

	Settings.Files.M3U["provider"].(map[string]interface{})["name"] = "modified-provider"

	Data.Streams.All = []interface{}{
		map[string]string{"name": "modified-stream"},
	}

	Data.Streams.Active = nil
	Data.Streams.Inactive = nil
	Data.Filter = nil

	Data.Playlist.M3U.Groups.Text =
		[]string{"modified-group"}

	Data.Playlist.M3U.Groups.Value = nil

	Data.StreamPreviewUI.Active = nil
	Data.StreamPreviewUI.Inactive = nil

	System.Folder.Temp = "modified-temp"
	System.DeviceID = "modified-device"

	providerMutex.Unlock()

	for _, path := range paths {
		writeSnapshotTestFile(t, path, "modified")
	}
}

func assertExpandedRollbackState(
	t *testing.T,
	snapshot *combinedXEPGRecoverySnapshot,
	paths []string,
) {

	t.Helper()

	expanded, err :=
		captureXEPGExpandedStateRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(
		expanded, snapshot.Expanded,
	) {
		t.Fatal("Expanded state was not restored")
	}

	mapping, err :=
		captureChannelMappingRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(
		mapping, snapshot.Mapping,
	) {
		t.Fatal("Channel mapping was not restored")
	}

	streaming :=
		captureStreamingURLRecoverySnapshot()

	if !reflect.DeepEqual(
		streaming, snapshot.Streaming,
	) {
		t.Fatal("Streaming state was not restored")
	}

	for _, path := range paths {

		assertSnapshotTestFile(
			t,
			path,
			"original:"+filepath.Base(path),
		)
	}

	t.Log("PASS: All four recovery components restored")
}

func TestXEPGExpandedRollbackRestore(t *testing.T) {

	root, paths := setupExpandedRollbackTest(t)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root, paths,
		)

	if err != nil {
		t.Fatal(err)
	}

	if snapshot.Expanded == nil {
		t.Fatal("Expanded snapshot missing")
	}

	for attempt := 0; attempt < 2; attempt++ {

		mutateExpandedRollbackTest(t, paths)

		if err := snapshot.Restore(); err != nil {
			t.Fatal(err)
		}

		assertExpandedRollbackState(
			t, snapshot, paths,
		)
	}

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}
}

func TestXEPGExpandedRollbackAutomatic(t *testing.T) {

	root, paths := setupExpandedRollbackTest(t)

	expected, err :=
		captureCombinedXEPGRecoverySnapshot(
			root, paths,
		)

	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := expected.Discard(); err != nil {
			t.Error(err)
		}
	}()

	failure := errors.New("synthetic rebuild failure")

	result, err := executeXEPGWithRollback(

		func() (xepgRollbackSnapshot, error) {

			snapshot, err :=
				captureCombinedXEPGRecoverySnapshot(
					root, paths,
				)

			if err != nil {
				return nil, err
			}

			return snapshot, nil
		},

		func() error {

			mutateExpandedRollbackTest(t, paths)

			return failure
		},
	)

	if !errors.Is(err, failure) {
		t.Fatal("Original operation failure was lost")
	}

	if result.Committed || !result.Restored {
		t.Fatal("Unexpected rollback result")
	}

	if result.RetainedSnapshot != nil {
		t.Fatal("Successful rollback retained snapshot")
	}

	assertExpandedRollbackState(
		t, expected, paths,
	)

	t.Log("PASS: Automatic expanded rollback completed")
}

func TestXEPGExpandedRollbackCaptureFailure(t *testing.T) {

	root, paths := setupExpandedRollbackTest(t)

	providerMutex.Lock()

	Settings.Files.M3U["invalid"] = make(chan int)

	providerMutex.Unlock()

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root, paths,
		)

	if err == nil || snapshot != nil {
		t.Fatal("Expected expanded capture failure")
	}

	if !strings.Contains(
		err.Error(), "expanded state snapshot",
	) {
		t.Fatal("Unexpected capture failure:", err)
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

	for _, path := range paths {

		assertSnapshotTestFile(
			t,
			path,
			"original:"+filepath.Base(path),
		)
	}

	t.Log("PASS: Failed capture cleaned up safely")
}

func TestXEPGExpandedRollbackFailureRetry(t *testing.T) {

	root, paths := setupExpandedRollbackTest(t)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root, paths,
		)

	if err != nil {
		t.Fatal(err)
	}

	originalExpanded := snapshot.Expanded

	invalid, err :=
		cloneExpandedState(*originalExpanded)

	if err != nil {
		t.Fatal(err)
	}

	invalid.AllStreams = []interface{}{42}

	snapshot.Expanded = invalid

	mutateExpandedRollbackTest(t, paths)

	if err := snapshot.Restore(); err == nil {
		t.Fatal("Expected expanded restoration failure")
	}

	if !snapshot.restorationFailed {
		t.Fatal("Restoration failure was not recorded")
	}

	if err := snapshot.Discard(); err == nil {
		t.Fatal("Failed snapshot could be discarded")
	}

	if _, err := os.Stat(snapshot.Disk.Directory); err != nil {
		t.Fatal("Failed snapshot directory was lost")
	}

	for _, path := range paths {

		assertSnapshotTestFile(
			t,
			path,
			"original:"+filepath.Base(path),
		)
	}

	mapping, err :=
		captureChannelMappingRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(mapping, snapshot.Mapping) {
		t.Fatal("Mapping restoration was not attempted")
	}

	streaming := captureStreamingURLRecoverySnapshot()

	if !reflect.DeepEqual(streaming, snapshot.Streaming) {
		t.Fatal("Streaming restoration was not attempted")
	}

	snapshot.Expanded = originalExpanded

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	if snapshot.restorationFailed {
		t.Fatal("Successful retry did not clear failure")
	}

	assertExpandedRollbackState(
		t, snapshot, paths,
	)

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}

	t.Log("PASS: Expanded restoration failure and retry")
}

func TestXEPGExpandedRollbackDiscard(t *testing.T) {

	root, paths := setupExpandedRollbackTest(t)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root, paths,
		)

	if err != nil {
		t.Fatal(err)
	}

	directory := snapshot.Disk.Directory

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}

	if snapshot.Disk != nil ||
		snapshot.Mapping != nil ||
		snapshot.Streaming != nil ||
		snapshot.Expanded != nil {

		t.Fatal("Snapshot references were not released")
	}

	if !snapshot.discarded {
		t.Fatal("Snapshot disposal was not recorded")
	}

	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("Snapshot directory was not removed")
	}

	if err := snapshot.Discard(); err != nil {
		t.Fatal("Repeated discard failed:", err)
	}

	if err := snapshot.Restore(); err == nil {
		t.Fatal("Discarded snapshot could be restored")
	}

	t.Log("PASS: All four snapshot references released")
}

func TestXEPGExpandedRollbackMissingComponent(t *testing.T) {

	root, paths := setupExpandedRollbackTest(t)

	snapshot, err :=
		captureCombinedXEPGRecoverySnapshot(
			root, paths,
		)

	if err != nil {
		t.Fatal(err)
	}

	expanded := snapshot.Expanded

	snapshot.Expanded = nil

	if err := snapshot.Restore(); err == nil {
		t.Fatal("Missing expanded snapshot accepted")
	}

	if err := snapshot.Discard(); err == nil {
		t.Fatal("Incomplete snapshot accepted")
	}

	snapshot.Expanded = expanded

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}

	t.Log("PASS: Missing expanded component rejected")
}
