package src

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStartupSamePortRuntimeRestore(t *testing.T) {

	fixture := os.Getenv("PROXFIN_TEST_FIXTURE")

	if fixture == "" {
		t.Skip("Isolated runtime fixture not configured")
	}

	root := t.TempDir()

	config := filepath.Join(root, "config")
	temporary := filepath.Join(root, "temp")
	backup := filepath.Join(root, "backup")

	for _, directory := range []string{
		config,
		temporary,
		backup,
	} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("TEMP", temporary)
	t.Setenv("TMP", temporary)
	t.Setenv("TMPDIR", temporary)

	originalSystem := System
	originalSettings := Settings

	t.Cleanup(func() {
		System = originalSystem
		Settings = originalSettings
	})

	System = SystemStruct{}
	Settings = SettingsStruct{}

	System.Name = "ProxFin"
	System.Version = "2.0"
	System.Build = "0"
	System.DBVersion = "0.5.0"
	System.Folder.Config = config
	System.Flag.Restore = ""

	const port = "29126"
	const originalUUID = "2026-11-TEST-ORIGINAL"
	const restoredUUID = "2026-11-TEST-RESTORED"

	files := make(map[string][]byte)

	for _, name := range SystemFiles {

		raw, err := os.ReadFile(
			filepath.Join(fixture, name),
		)

		if err != nil {
			t.Fatal(err)
		}

		files[name] = raw
	}

	var settings map[string]interface{}

	if err := json.Unmarshal(
		files["settings.json"],
		&settings,
	); err != nil {
		t.Fatal(err)
	}

	settings["port"] = port
	settings["uuid"] = originalUUID
	settings["ssdp"] = false
	settings["backup.keep"] = 0
	settings["backup.path"] = backup +
		string(os.PathSeparator)
	settings["temp.path"] = temporary +
		string(os.PathSeparator)
	settings["cache.images"] = false
	settings["files.update"] = false
	settings["epgSource"] = "PMS"

	settings["files"] = map[string]interface{}{
		"m3u":   map[string]interface{}{},
		"hdhr":  map[string]interface{}{},
		"xmltv": map[string]interface{}{},
	}

	raw, err := json.MarshalIndent(settings, "", "  ")

	if err != nil {
		t.Fatal(err)
	}

	files["settings.json"] = raw

	for name, content := range files {

		if err := os.WriteFile(
			filepath.Join(config, name),
			content,
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	t.Log("Disposable configuration prepared")

	if err := Init(); err != nil {
		t.Fatalf("Initial application initialization failed: %v", err)
	}

	if Settings.Port != port {
		t.Fatalf(
			"Unexpected initialized port: %q",
			Settings.Port,
		)
	}

	if Settings.UUID != originalUUID {
		t.Fatalf(
			"Unexpected initialized UUID: %q",
			Settings.UUID,
		)
	}

	if Settings.SSDP {
		t.Fatal("SSDP must remain disabled")
	}

	if len(Settings.Files.M3U) != 0 ||
		len(Settings.Files.HDHR) != 0 ||
		len(Settings.Files.XMLTV) != 0 {
		t.Fatal("Unexpected configured providers")
	}

	t.Log("Initial application initialization completed")
	t.Logf("In-memory port: %s", Settings.Port)
	t.Logf("In-memory UUID: %s", Settings.UUID)

	for _, name := range SystemFiles {

		content, err := os.ReadFile(
			filepath.Join(config, name),
		)

		if err != nil {
			t.Fatal(err)
		}

		files[name] = content
	}

	if err := json.Unmarshal(
		files["settings.json"],
		&settings,
	); err != nil {
		t.Fatal(err)
	}

	settings["uuid"] = restoredUUID

	raw, err = json.MarshalIndent(settings, "", "  ")

	if err != nil {
		t.Fatal(err)
	}

	files["settings.json"] = raw

	archive := filepath.Join(root, "same-port.zip")

	file, err := os.Create(archive)

	if err != nil {
		t.Fatal(err)
	}

	defer file.Close()

	writer := zip.NewWriter(file)

	for name, content := range files {

		entry, err := writer.Create(name)

		if err != nil {
			t.Fatal(err)
		}

		if _, err := entry.Write(content); err != nil {
			t.Fatal(err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	t.Log("Same-port restoration archive created")

	originalTemp := filepath.Clean(System.Folder.Temp)

	restoredTemp := filepath.Join(
		filepath.Dir(originalTemp),
		restoredUUID,
	)

	if restoredTemp == originalTemp {
		t.Fatal("Restored temporary directory must differ")
	}

	if err := os.MkdirAll(restoredTemp, 0700); err != nil {
		t.Fatal(err)
	}

	marker := filepath.Join(
		restoredTemp,
		"same-port-initialization-marker",
	)

	if err := os.WriteFile(
		marker,
		[]byte("initialization verification"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	startupRuntimeAssertUnlocked(t)

	url, err := ThreadfinRestore(archive)

	if err != nil {
		t.Fatalf("Same-port restoration failed: %v", err)
	}

	if url != "" {
		t.Fatalf("Unexpected restart URL: %q", url)
	}

	if Settings.Port != port {
		t.Fatal("In-memory port changed")
	}

	if Settings.UUID != restoredUUID {
		t.Fatalf(
			"Restored in-memory UUID mismatch: %q",
			Settings.UUID,
		)
	}

	if filepath.Clean(System.Folder.Temp) != restoredTemp {
		t.Fatalf(
			"Unexpected restored temporary directory: %q",
			System.Folder.Temp,
		)
	}

	t.Log("PASS: Restored UUID temporary directory selected")

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("Runtime initialization cleanup was not verified")
	}

	if System.ScanInProgress != 0 {
		t.Fatal("Scan flag was not cleared")
	}

	for _, name := range SystemFiles {

		raw, err := os.ReadFile(
			filepath.Join(config, name),
		)

		if err != nil {
			t.Fatal(err)
		}

		var document map[string]interface{}

		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}

		if document == nil {
			t.Fatalf("Invalid configuration object: %s", name)
		}

		if name == "settings.json" {

			if document["uuid"] != restoredUUID {
				t.Fatal("Restored disk UUID mismatch")
			}

			if document["port"] != port {
				t.Fatal("Restored disk port mismatch")
			}
		}

		t.Logf("Verified configuration: %s", name)
	}

	startupRuntimeAssertUnlocked(t)

	t.Log("PASS: Same-port restoration completed")
	t.Log("PASS: Runtime reinitialization verified")
	t.Log("PASS: Restored UUID verified in memory and on disk")
	t.Log("PASS: Original port preserved")
	t.Log("PASS: Refresh mutex released")
}
