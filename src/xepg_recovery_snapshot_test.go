package src

import (
    "bytes"
    "crypto/sha256"
    "io"
    "os"
    "path/filepath"
    "runtime"
    "testing"
)

func writeSnapshotTestFile(
    t *testing.T,
    path string,
    content string,
) {
    t.Helper()

    if err := os.WriteFile(
        path,
        []byte(content),
        0600,
    ); err != nil {
        t.Fatal(err)
    }
}

func assertSnapshotTestFile(
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
        t.Fatalf(
            "Unexpected file contents: %s",
            filepath.Base(path),
        )
    }
}

func TestXEPGRecoverySnapshotRestore(t *testing.T) {

    root := t.TempDir()

    modified := filepath.Join(root, "xepg.json")
    deleted := filepath.Join(root, "urls.json")
    created := filepath.Join(root, "proxfin.xml")
    absent := filepath.Join(root, "absent.xml")

    writeSnapshotTestFile(t, modified, "original mapping")
    writeSnapshotTestFile(t, deleted, "original urls")

    snapshot, err := captureXEPGRecoverySnapshot(
        root,
        []string{
            modified,
            deleted,
            created,
            absent,
        },
    )

    if err != nil {
        t.Fatal(err)
    }

    writeSnapshotTestFile(t, modified, "modified mapping")
    writeSnapshotTestFile(t, created, "new guide")

    if err := os.Remove(deleted); err != nil {
        t.Fatal(err)
    }

    if err := snapshot.Restore(); err != nil {
        t.Fatal(err)
    }

    assertSnapshotTestFile(
        t,
        modified,
        "original mapping",
    )

    assertSnapshotTestFile(
        t,
        deleted,
        "original urls",
    )

    for _, path := range []string{created, absent} {

        if _, err := os.Stat(path); !os.IsNotExist(err) {
            t.Fatalf(
                "Expected absent file: %s",
                filepath.Base(path),
            )
        }
    }

    // Restoration should be repeatable.
    if err := snapshot.Restore(); err != nil {
        t.Fatal(err)
    }

    if err := snapshot.Discard(); err != nil {
        t.Fatal(err)
    }

    if _, err := os.Stat(snapshot.Directory);
        !os.IsNotExist(err) {

        t.Fatal("Snapshot directory was not removed")
    }
}

func TestXEPGRecoverySnapshotPermissions(t *testing.T) {

    if runtime.GOOS == "windows" {
        t.Skip("Unix permission test")
    }

    root := t.TempDir()
    path := filepath.Join(root, "settings.json")

    writeSnapshotTestFile(t, path, "original")

    if err := os.Chmod(path, 0640); err != nil {
        t.Fatal(err)
    }

    snapshot, err := captureXEPGRecoverySnapshot(
        root,
        []string{path},
    )

    if err != nil {
        t.Fatal(err)
    }

    directoryInfo, err := os.Stat(snapshot.Directory)

    if err != nil {
        t.Fatal(err)
    }

    if directoryInfo.Mode().Perm() != 0700 {
        t.Fatal("Snapshot directory is not private")
    }

    backupInfo, err := os.Stat(snapshot.Files[0].Backup)

    if err != nil {
        t.Fatal(err)
    }

    if backupInfo.Mode().Perm() != 0600 {
        t.Fatal("Snapshot file is not private")
    }

    if err := os.Chmod(path, 0600); err != nil {
        t.Fatal(err)
    }

    writeSnapshotTestFile(t, path, "changed")

    if err := snapshot.Restore(); err != nil {
        t.Fatal(err)
    }

    info, err := os.Stat(path)

    if err != nil {
        t.Fatal(err)
    }

    if info.Mode().Perm() != 0640 {
        t.Fatal("Original permissions were not restored")
    }

    assertSnapshotTestFile(t, path, "original")
}

func TestXEPGRecoverySnapshotInvalidInputs(t *testing.T) {

    root := t.TempDir()
    path := filepath.Join(root, "xepg.json")

    writeSnapshotTestFile(t, path, "original")

    cases := []struct {
        name  string
        root  string
        paths []string
    }{
        {
            name: "empty-root",
            root: "",
            paths: []string{path},
        },
        {
            name: "missing-root",
            root: filepath.Join(root, "missing"),
            paths: []string{path},
        },
        {
            name: "empty-list",
            root: root,
            paths: nil,
        },
        {
            name: "empty-path",
            root: root,
            paths: []string{""},
        },
        {
            name: "duplicate-path",
            root: root,
            paths: []string{path, path},
        },
        {
            name: "directory-target",
            root: root,
            paths: []string{root},
        },
    }

    for _, tc := range cases {

        t.Run(tc.name, func(t *testing.T) {

            snapshot, err := captureXEPGRecoverySnapshot(
                tc.root,
                tc.paths,
            )

            if err == nil || snapshot != nil {
                t.Fatal("Invalid snapshot was accepted")
            }

            assertSnapshotTestFile(t, path, "original")

            leftovers, err := filepath.Glob(
                filepath.Join(root, ".xepg-rollback-*"),
            )

            if err != nil {
                t.Fatal(err)
            }

            if len(leftovers) != 0 {
                t.Fatal("Incomplete snapshot was not cleaned up")
            }
        })
    }
}

func TestXEPGRecoverySnapshotSymlink(t *testing.T) {

    root := t.TempDir()

    target := filepath.Join(root, "original.json")
    link := filepath.Join(root, "linked.json")

    writeSnapshotTestFile(t, target, "original")

    if err := os.Symlink(target, link); err != nil {
        t.Skip("Symbolic links unavailable")
    }

    snapshot, err := captureXEPGRecoverySnapshot(
        root,
        []string{link},
    )

    if err == nil || snapshot != nil {
        t.Fatal("Symbolic link was accepted")
    }

    assertSnapshotTestFile(t, target, "original")
}

func TestXEPGRecoverySnapshotPartialFailure(t *testing.T) {

    root := t.TempDir()

    first := filepath.Join(root, "first.json")
    second := filepath.Join(root, "second.json")

    writeSnapshotTestFile(t, first, "first-original")
    writeSnapshotTestFile(t, second, "second-original")

    snapshot, err := captureXEPGRecoverySnapshot(
        root,
        []string{first, second},
    )

    if err != nil {
        t.Fatal(err)
    }

    writeSnapshotTestFile(t, first, "first-changed")
    writeSnapshotTestFile(t, second, "second-changed")

    // Simulate a missing rollback file.
    if err := os.Remove(
        snapshot.Files[0].Backup,
    ); err != nil {
        t.Fatal(err)
    }

    if err := snapshot.Restore(); err == nil {
        t.Fatal("Expected restoration failure")
    }

    assertSnapshotTestFile(
        t,
        first,
        "first-changed",
    )

    // Other files must still be restored.
    assertSnapshotTestFile(
        t,
        second,
        "second-original",
    )

    if _, err := os.Stat(snapshot.Directory); err != nil {
        t.Fatal("Snapshot was not retained")
    }
}

func TestXEPGRecoverySnapshotUnsafeDestination(t *testing.T) {

    root := t.TempDir()
    path := filepath.Join(root, "generated.xml")

    snapshot, err := captureXEPGRecoverySnapshot(
        root,
        []string{path},
    )

    if err != nil {
        t.Fatal(err)
    }

    if err := os.Mkdir(path, 0700); err != nil {
        t.Fatal(err)
    }

    if err := snapshot.Restore(); err == nil {
        t.Fatal("Unsafe destination was accepted")
    }

    info, err := os.Stat(path)

    if err != nil || !info.IsDir() {
        t.Fatal("Directory was unexpectedly removed")
    }
}

func snapshotTestChecksum(
    t *testing.T,
    path string,
) []byte {

    t.Helper()

    file, err := os.Open(path)

    if err != nil {
        t.Fatal(err)
    }

    defer file.Close()

    hash := sha256.New()

    if _, err := io.Copy(hash, file); err != nil {
        t.Fatal(err)
    }

    return hash.Sum(nil)
}

func TestXEPGRecoverySnapshotLargeFile(t *testing.T) {

    root := t.TempDir()
    path := filepath.Join(root, "large.xml")

    file, err := os.Create(path)

    if err != nil {
        t.Fatal(err)
    }

    // Generate 32 MiB using a reusable 64 KiB buffer.
    block := bytes.Repeat([]byte("X"), 64*1024)

    for i := 0; i < 512; i++ {

        if _, err := file.Write(block); err != nil {
            file.Close()
            t.Fatal(err)
        }
    }

    if err := file.Close(); err != nil {
        t.Fatal(err)
    }

    originalHash := snapshotTestChecksum(t, path)

    snapshot, err := captureXEPGRecoverySnapshot(
        root,
        []string{path},
    )

    if err != nil {
        t.Fatal(err)
    }

    writeSnapshotTestFile(t, path, "changed")

    if err := snapshot.Restore(); err != nil {
        t.Fatal(err)
    }

    restoredHash := snapshotTestChecksum(t, path)

    if !bytes.Equal(originalHash, restoredHash) {
        t.Fatal("Restored large file checksum mismatch")
    }

    if err := snapshot.Discard(); err != nil {
        t.Fatal(err)
    }
}

func TestXEPGRecoverySnapshotInvalidObject(t *testing.T) {

    var missing *xepgRecoverySnapshot

    if err := missing.Restore(); err == nil {
        t.Fatal("Nil snapshot was accepted")
    }

    if err := missing.Discard(); err == nil {
        t.Fatal("Nil snapshot disposal was accepted")
    }

    empty := &xepgRecoverySnapshot{}

    if err := empty.Restore(); err == nil {
        t.Fatal("Empty snapshot was accepted")
    }

    if err := empty.Discard(); err == nil {
        t.Fatal("Empty snapshot disposal was accepted")
    }
}