package src

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func setupPMSRecoveryTest(t *testing.T) string {
	t.Helper()

	setupStreamingURLRecoveryTest(t)

	oldPMSFile := System.File.PMS
	oldURLsFile := System.File.URLS
	oldSource := Settings.EpgSource
	oldActive := Data.Streams.Active
	oldCache := Data.Cache.PMS
	oldGuideNumbers := pmsGuideNumbers

	t.Cleanup(func() {
		System.File.PMS = oldPMSFile
		System.File.URLS = oldURLsFile
		Settings.EpgSource = oldSource
		Data.Streams.Active = oldActive
		Data.Cache.PMS = oldCache
		pmsGuideNumbers = oldGuideNumbers
	})

	root := t.TempDir()

	System.File.PMS = filepath.Join(root, "pms.json")
	System.File.URLS = filepath.Join(root, "urls.json")

	Settings.EpgSource = "PMS"
	Data.Streams.Active = nil
	Data.Cache.PMS = nil

	pmsGuideNumbers.used = nil
	pmsGuideNumbers.next = 0
	pmsGuideNumbers.dirty = false

	if err := saveMapToJSONFile(
		System.File.PMS,
		map[string]string{
			"Original A": "id-0",
			"Original B": "id-2",
		},
	); err != nil {
		t.Fatal(err)
	}

	return System.File.PMS
}

func readPMSRecoveryBytes(
	t *testing.T, path string,
) []byte {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return content
}

func readPMSRecoveryMappings(
	t *testing.T, path string,
) map[string]string {
	t.Helper()

	var mappings map[string]string

	if err := json.Unmarshal(
		readPMSRecoveryBytes(t, path),
		&mappings,
	); err != nil {
		t.Fatal(err)
	}

	if mappings == nil {
		t.Fatal("PMS file contains a null object")
	}

	return mappings
}

func assertPMSRecoveryCleanup(t *testing.T) {
	t.Helper()

	if Data.Cache.PMS != nil {
		t.Fatal("PMS cache was not cleared")
	}

	if pmsGuideNumbers.used != nil {
		t.Fatal("Used guide numbers were not cleared")
	}

	if pmsGuideNumbers.dirty {
		t.Fatal("Dirty flag was not reset")
	}
}

func runPMSRecoveryLineup(t *testing.T) {
	t.Helper()

	if _, err := getLineup(); err != nil {
		t.Fatal(err)
	}
}

func TestPMSRecoveryNilCacheProtection(t *testing.T) {
	path := setupPMSRecoveryTest(t)
	before := readPMSRecoveryBytes(t, path)

	Data.Cache.PMS = nil
	pmsGuideNumbers.dirty = true

	runPMSRecoveryLineup(t)

	after := readPMSRecoveryBytes(t, path)

	if !bytes.Equal(before, after) {
		t.Fatal("Nil cache overwrote existing PMS data")
	}

	assertPMSRecoveryCleanup(t)
}

func TestPMSRecoveryCleanCacheProtection(t *testing.T) {
	path := setupPMSRecoveryTest(t)
	before := readPMSRecoveryBytes(t, path)

	Data.Cache.PMS = map[string]string{
		"Unsaved entry": "id-99",
	}

	pmsGuideNumbers.dirty = false

	runPMSRecoveryLineup(t)

	if !bytes.Equal(
		before, readPMSRecoveryBytes(t, path),
	) {
		t.Fatal("Clean cache unexpectedly modified disk")
	}

	assertPMSRecoveryCleanup(t)
}

func TestPMSRecoveryDirtyCachePersistence(t *testing.T) {
	path := setupPMSRecoveryTest(t)

	id, err := getGuideNumberPMS("New channel")
	if err != nil {
		t.Fatal(err)
	}

	if id != "id-1" {
		t.Fatalf("Unexpected guide number: %s", id)
	}

	if !pmsGuideNumbers.dirty {
		t.Fatal("New mapping was not marked dirty")
	}

	runPMSRecoveryLineup(t)

	mappings := readPMSRecoveryMappings(t, path)

	if mappings["New channel"] != id {
		t.Fatal("New mapping was not saved")
	}

	assertPMSRecoveryCleanup(t)
}

func TestPMSRecoveryExistingMappingPreservation(t *testing.T) {
	path := setupPMSRecoveryTest(t)

	if _, err := getGuideNumberPMS("New channel"); err != nil {
		t.Fatal(err)
	}

	runPMSRecoveryLineup(t)

	expected := map[string]string{
		"Original A":  "id-0",
		"Original B":  "id-2",
		"New channel": "id-1",
	}

	actual := readPMSRecoveryMappings(t, path)

	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf(
			"Mappings changed unexpectedly: %v",
			actual,
		)
	}

	assertPMSRecoveryCleanup(t)
}

func TestPMSRecoveryDirtyFlagCleanup(t *testing.T) {
	setupPMSRecoveryTest(t)

	Data.Cache.PMS = map[string]string{
		"Original A": "id-0",
		"Original B": "id-2",
	}

	pmsGuideNumbers.used = map[string]bool{
		"id-0": true,
		"id-2": true,
	}

	pmsGuideNumbers.dirty = true

	runPMSRecoveryLineup(t)
	assertPMSRecoveryCleanup(t)
}

func TestPMSRecoveryRepeatedLineup(t *testing.T) {
	path := setupPMSRecoveryTest(t)

	for i := 0; i < 20; i++ {
		id, err := getGuideNumberPMS("Stable channel")
		if err != nil {
			t.Fatal(err)
		}

		if id != "id-1" {
			t.Fatalf(
				"Iteration %d: unstable guide number %s",
				i, id,
			)
		}

		runPMSRecoveryLineup(t)
		assertPMSRecoveryCleanup(t)

		mappings := readPMSRecoveryMappings(t, path)

		if len(mappings) != 3 {
			t.Fatalf(
				"Iteration %d: unexpected mapping count",
				i,
			)
		}

		if mappings["Stable channel"] != "id-1" ||
			mappings["Original A"] != "id-0" ||
			mappings["Original B"] != "id-2" {
			t.Fatal("Existing guide numbers changed")
		}
	}
}

func TestPMSRecoveryEmptyLineup(t *testing.T) {
	path := setupPMSRecoveryTest(t)
	before := readPMSRecoveryBytes(t, path)

	for i := 0; i < 10; i++ {
		runPMSRecoveryLineup(t)
		assertPMSRecoveryCleanup(t)

		if !bytes.Equal(
			before, readPMSRecoveryBytes(t, path),
		) {
			t.Fatal("Empty lineup modified PMS mappings")
		}
	}
}
