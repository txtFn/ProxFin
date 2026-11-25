package src

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func setupSettingsDiskRecoveryPaths(
	t *testing.T,
) (string, []string) {

	t.Helper()

	root := t.TempDir()

	savedFile := System.File
	savedCompressed := System.Compressed

	t.Cleanup(func() {
		System.File = savedFile
		System.Compressed = savedCompressed
	})

	System.File.Settings = filepath.Join(
		root, "settings.json",
	)

	System.File.XEPG = filepath.Join(
		root, "xepg.json",
	)

	System.File.URLS = filepath.Join(
		root, "urls.json",
	)

	System.File.M3U = filepath.Join(
		root, "proxfin.m3u",
	)

	System.File.XML = filepath.Join(
		root, "proxfin.xml",
	)

	System.Compressed.GZxml = filepath.Join(
		root, "proxfin.xml.gz",
	)

	expected := []string{
		System.File.Settings,
		System.File.XEPG,
		System.File.URLS,
		System.File.M3U,
		System.File.XML,
		System.Compressed.GZxml,
	}

	return root, expected
}

func TestXEPGSettingsDiskRecoveryPaths(t *testing.T) {

	_, expected := setupSettingsDiskRecoveryPaths(t)

	actual := xepgRecoveryFilePaths()

	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("Unexpected recovery file coverage")
	}

	if len(actual) != 6 {
		t.Fatal("Expected six protected files")
	}

	seen := make(map[string]bool)

	for _, path := range actual {

		if path == "" || seen[path] {
			t.Fatal("Invalid or duplicate recovery path")
		}

		seen[path] = true
	}

	t.Log("PASS: All six recovery paths verified")
}

func TestXEPGSettingsDiskRecoveryRestore(t *testing.T) {

	root, paths := setupSettingsDiskRecoveryPaths(t)

	original := []string{
		`{"uuid":"original-uuid","files":{"m3u":{}}}`,
		`{"channel":"original-mapping"}`,
		`{"stream":"original-url"}`,
		"#EXTM3U\nOriginal playlist\n",
		"<tv></tv>",
		"synthetic compressed guide",
	}

	for i, path := range paths {
		writeSnapshotTestFile(t, path, original[i])
	}

	snapshot, err := captureXEPGRecoverySnapshot(
		root,
		xepgRecoveryFilePaths(),
	)

	if err != nil {
		t.Fatal(err)
	}

	if len(snapshot.Files) != 6 {
		t.Fatal("Incomplete disk snapshot")
	}

	directory := snapshot.Directory

	for _, path := range paths {
		writeSnapshotTestFile(t, path, "modified")
	}

	if err := os.Remove(paths[2]); err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt < 2; attempt++ {

		if err := snapshot.Restore(); err != nil {
			t.Fatal(err)
		}

		for i, path := range paths {
			assertSnapshotTestFile(
				t, path, original[i],
			)
		}
	}

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("Snapshot directory was not removed")
	}

	t.Log("PASS: All six files restored byte-for-byte")
}

func TestXEPGSettingsDiskRecoveryDeletedSettings(
	t *testing.T,
) {

	root, paths := setupSettingsDiskRecoveryPaths(t)

	original := `{"uuid":"original-uuid"}`

	writeSnapshotTestFile(
		t, System.File.Settings, original,
	)

	snapshot, err := captureXEPGRecoverySnapshot(
		root, paths,
	)

	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(System.File.Settings); err != nil {
		t.Fatal(err)
	}

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	assertSnapshotTestFile(
		t, System.File.Settings, original,
	)

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}

	t.Log("PASS: Deleted settings file restored")
}

func TestXEPGSettingsDiskRecoveryAbsentSettings(
	t *testing.T,
) {

	root, paths := setupSettingsDiskRecoveryPaths(t)

	snapshot, err := captureXEPGRecoverySnapshot(
		root, paths,
	)

	if err != nil {
		t.Fatal(err)
	}

	writeSnapshotTestFile(
		t,
		System.File.Settings,
		`{"uuid":"new-uuid"}`,
	)

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(System.File.Settings); !os.IsNotExist(err) {

		t.Fatal("Originally absent settings file remains")
	}

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}

	t.Log("PASS: Original absence preserved")
}

func TestXEPGSettingsDiskRecoveryPartialFailure(
	t *testing.T,
) {

	root, paths := setupSettingsDiskRecoveryPaths(t)

	for _, path := range paths {
		writeSnapshotTestFile(t, path, "original")
	}

	snapshot, err := captureXEPGRecoverySnapshot(
		root, paths,
	)

	if err != nil {
		t.Fatal(err)
	}

	for _, path := range paths {
		writeSnapshotTestFile(t, path, "modified")
	}

	settingsBackup := snapshot.Files[0].Backup

	if err := os.Remove(settingsBackup); err != nil {
		t.Fatal(err)
	}

	if err := snapshot.Restore(); err == nil {
		t.Fatal("Expected settings restoration failure")
	}

	assertSnapshotTestFile(
		t, System.File.Settings, "modified",
	)

	for _, path := range paths[1:] {
		assertSnapshotTestFile(t, path, "original")
	}

	if _, err := os.Stat(snapshot.Directory); err != nil {
		t.Fatal("Failed recovery snapshot was lost")
	}

	writeSnapshotTestFile(
		t, settingsBackup, "original",
	)

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	for _, path := range paths {
		assertSnapshotTestFile(t, path, "original")
	}

	if err := snapshot.Discard(); err != nil {
		t.Fatal(err)
	}

	t.Log("PASS: Partial failure reported and retry successful")
}
