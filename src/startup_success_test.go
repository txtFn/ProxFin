package src

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupSuccessfulRestoreTest(t *testing.T) string {

	t.Helper()

	originalSystem := System
	originalSettings := Settings

	t.Cleanup(func() {
		System = originalSystem
		Settings = originalSettings
	})

	root := t.TempDir()

	config := filepath.Join(root, "config")
	temporary := filepath.Join(root, "temp")

	for _, directory := range []string{
		config,
		temporary,
	} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}

	System = SystemStruct{}
	Settings = SettingsStruct{}

	System.Folder.Config = config + string(os.PathSeparator)
	System.Folder.Temp = temporary + string(os.PathSeparator)
	System.Compatibility = "0.1.0"
	System.URLBase = "http://127.0.0.1:34400"

	Settings.Port = "34400"

	original := []byte(
		`{"version":"9.9.9","port":"34400","uuid":"original-test"}`,
	)

	if err := os.WriteFile(
		filepath.Join(config, "settings.json"),
		original,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(root, "synthetic-backup.zip")

	file, err := os.Create(archive)

	if err != nil {
		t.Fatal(err)
	}

	defer file.Close()

	writer := zip.NewWriter(file)

	files := map[string]string{
		"settings.json":       `{"version":"9.9.9","port":"34401","uuid":"restored-test"}`,
		"authentication.json": `{}`,
		"pms.json":            `{}`,
		"xepg.json":           `{}`,
		"urls.json":           `{}`,
	}

	for name, content := range files {

		entry, err := writer.Create(name)

		if err != nil {
			t.Fatal(err)
		}

		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	return archive
}

func verifySuccessfulRestore(t *testing.T, url string) {

	t.Helper()

	expectedURL := "http://127.0.0.1:34401/web/"

	if url != expectedURL {
		t.Fatalf("Unexpected restart URL: %q", url)
	}

	for _, name := range SystemFiles {

		path := filepath.Join(System.Folder.Config, name)

		raw, err := os.ReadFile(path)

		if err != nil {
			t.Fatal(err)
		}

		var document map[string]interface{}

		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}

		if name == "settings.json" {

			if document["uuid"] != "restored-test" {
				t.Fatal("Restored UUID mismatch")
			}

			if document["port"] != "34401" {
				t.Fatal("Restored port mismatch")
			}
		}
	}

	temporaryRestore := filepath.Join(
		System.Folder.Temp,
		"restore",
	)

	if _, err := os.Stat(temporaryRestore); !os.IsNotExist(err) {
		t.Fatal("Temporary extraction directory was not removed")
	}

	t.Log("All five configuration files restored")
	t.Log("Restored UUID and port verified")
	t.Log("Temporary extraction directory removed")
}

func TestStartupSuccessPublicArchiveRestore(t *testing.T) {

	archive := setupSuccessfulRestoreTest(t)

	url, err := ThreadfinRestore(archive)

	if err != nil {
		t.Fatal(err)
	}

	verifySuccessfulRestore(t, url)
	startupRuntimeAssertUnlocked(t)
}

func TestStartupSuccessLockedArchiveRestore(t *testing.T) {

	archive := setupSuccessfulRestoreTest(t)

	refreshMutex.Lock()
	defer refreshMutex.Unlock()

	url, err := threadfinRestoreLocked(archive)

	if err != nil {
		t.Fatal(err)
	}

	verifySuccessfulRestore(t, url)

	if refreshMutex.TryLock() {
		refreshMutex.Unlock()
		t.Fatal("Internal restoration released caller lock")
	}
}

func TestStartupSuccessPublicWebRestore(t *testing.T) {

	archive := setupSuccessfulRestoreTest(t)

	raw, err := os.ReadFile(archive)

	if err != nil {
		t.Fatal(err)
	}

	input := "data:application/zip;base64," +
		base64.StdEncoding.EncodeToString(raw)

	url, err := ThreadfinRestoreFromWeb(input)

	if err != nil {
		t.Fatal(err)
	}

	verifySuccessfulRestore(t, url)
	startupRuntimeAssertUnlocked(t)
}

func TestStartupSuccessLockedWebRestore(t *testing.T) {

	archive := setupSuccessfulRestoreTest(t)

	raw, err := os.ReadFile(archive)

	if err != nil {
		t.Fatal(err)
	}

	input := "data:application/zip;base64," +
		base64.StdEncoding.EncodeToString(raw)

	refreshMutex.Lock()
	defer refreshMutex.Unlock()

	url, err := threadfinRestoreFromWebLocked(input)

	if err != nil {
		t.Fatal(err)
	}

	verifySuccessfulRestore(t, url)

	if refreshMutex.TryLock() {
		refreshMutex.Unlock()
		t.Fatal("Internal web restoration released caller lock")
	}
}
