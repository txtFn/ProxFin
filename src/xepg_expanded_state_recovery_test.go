package src

import (
	"reflect"
	"testing"
)

func expandedRecoveryFixture() xepgExpandedStateRecoverySnapshot {

	var fixture xepgExpandedStateRecoverySnapshot

	fixture.Settings.UUID = "test-original-uuid"
	fixture.Settings.BackupKeep = 10
	fixture.Settings.Tuner = 2

	fixture.Settings.FileM3U = []string{"playlist"}
	fixture.Settings.FileXMLTV = []string{"guide"}
	fixture.Settings.Update = []string{"04:00"}

	fixture.Settings.Files.M3U = map[string]interface{}{
		"provider": map[string]interface{}{
			"name":                  "Original provider",
			"counter.download":      float64(5),
			"provider.availability": int(100),
		},
	}

	fixture.Settings.Files.HDHR = map[string]interface{}{
		"hdhr": map[string]interface{}{
			"name": "Original HDHR",
		},
	}

	fixture.Settings.Files.XMLTV = map[string]interface{}{
		"xmltv": map[string]interface{}{
			"name": "Original XMLTV",
		},
	}

	fixture.Settings.Filter = map[int64]interface{}{
		1: map[string]interface{}{
			"name": "Original filter",
		},
		2: FilterStruct{Name: "Value filter"},
		3: &FilterStruct{Name: "Pointer filter"},
		4: (*FilterStruct)(nil),
	}

	active := map[string]string{
		"name": "Active channel",
		"url":  "https://example.invalid/active",
	}

	inactive := map[string]string{
		"name": "Inactive channel",
		"url":  "https://example.invalid/inactive",
	}

	fixture.AllStreams = []interface{}{active, inactive}
	fixture.ActiveStreams = []interface{}{active}
	fixture.InactiveStreams = []interface{}{inactive}

	fixture.Filters = []Filter{
		{Rule: "Original rule", Type: "group"},
	}

	fixture.GroupText = []string{"Original group"}
	fixture.GroupValue = []string{"original"}
	fixture.ActivePreview = []string{"Active preview"}
	fixture.InactivePreview = []string{"Inactive preview"}

	fixture.TempDirectory = "original-temp"
	fixture.DeviceID = "original-device"

	return fixture
}

func TestXEPGExpandedRecoverySettingsIsolation(t *testing.T) {

	original := expandedRecoveryFixture()

	cloned, err := cloneExpandedSettings(original.Settings)

	if err != nil {
		t.Fatal(err)
	}

	original.Settings.Files.M3U["provider"].(map[string]interface{})["name"] = "Changed"

	original.Settings.Files.HDHR["hdhr"].(map[string]interface{})["name"] = "Changed"

	original.Settings.Files.XMLTV["xmltv"].(map[string]interface{})["name"] = "Changed"

	original.Settings.FileM3U[0] = "Changed"
	original.Settings.FileXMLTV[0] = "Changed"
	original.Settings.Update[0] = "Changed"

	expected := expandedRecoveryFixture().Settings

	if !reflect.DeepEqual(expected, cloned) {
		t.Fatal("Settings snapshot was modified")
	}

	t.Log("PASS: Provider maps and settings slices isolated")
}

func TestXEPGExpandedRecoveryNumericTypes(t *testing.T) {

	fixture := expandedRecoveryFixture()

	cloned, err := cloneExpandedSettings(fixture.Settings)

	if err != nil {
		t.Fatal(err)
	}

	provider := cloned.Files.M3U["provider"].(map[string]interface{})

	if value, ok := provider["counter.download"].(float64); !ok || value != 5 {
		t.Fatal("Float type or value changed")
	}

	if value, ok := provider["provider.availability"].(int); !ok || value != 100 {
		t.Fatal("Integer type or value changed")
	}

	t.Log("PASS: Numeric types preserved")
}

func TestXEPGExpandedRecoveryConfiguredFilters(t *testing.T) {

	fixture := expandedRecoveryFixture()

	cloned, err := cloneExpandedSettings(fixture.Settings)

	if err != nil {
		t.Fatal(err)
	}

	fixture.Settings.Filter[1].(map[string]interface{})["name"] = "Changed"

	fixture.Settings.Filter[3].(*FilterStruct).Name = "Changed"

	delete(fixture.Settings.Filter, 2)

	expected := expandedRecoveryFixture().Settings.Filter

	if !reflect.DeepEqual(expected, cloned.Filter) {
		t.Fatal("Configured filter snapshot changed")
	}

	t.Log("PASS: Map, value, pointer and nil filters preserved")
}

func TestXEPGExpandedRecoveryRuntimeFilters(t *testing.T) {

	fixture := expandedRecoveryFixture()

	cloned, err := cloneExpandedState(fixture)

	if err != nil {
		t.Fatal(err)
	}

	fixture.Filters[0].Rule = "Changed"
	fixture.GroupText[0] = "Changed"
	fixture.GroupValue[0] = "Changed"
	fixture.ActivePreview[0] = "Changed"
	fixture.InactivePreview[0] = "Changed"

	expected := expandedRecoveryFixture()

	if !reflect.DeepEqual(expected, *cloned) {
		t.Fatal("Runtime filters or UI state changed")
	}

	t.Log("PASS: Runtime filters and UI slices isolated")
}

func TestXEPGExpandedRecoverySharedStreams(t *testing.T) {

	fixture := expandedRecoveryFixture()

	cloned, err := cloneExpandedState(fixture)

	if err != nil {
		t.Fatal(err)
	}

	cloned.AllStreams[0].(map[string]string)["name"] = "Changed active"

	if cloned.ActiveStreams[0].(map[string]string)["name"] != "Changed active" {
		t.Fatal("Active stream reference was not preserved")
	}

	cloned.AllStreams[1].(map[string]string)["name"] = "Changed inactive"

	if cloned.InactiveStreams[0].(map[string]string)["name"] != "Changed inactive" {
		t.Fatal("Inactive stream reference was not preserved")
	}

	if fixture.AllStreams[0].(map[string]string)["name"] != "Active channel" {
		t.Fatal("Cloned stream aliases original state")
	}

	if fixture.AllStreams[1].(map[string]string)["name"] != "Inactive channel" {
		t.Fatal("Cloned inactive stream aliases original state")
	}

	t.Log("PASS: Shared references preserved without original aliases")
}

func TestXEPGExpandedRecoveryDistinctStreams(t *testing.T) {

	fixture := expandedRecoveryFixture()

	first := map[string]string{"name": "Same"}
	second := map[string]string{"name": "Same"}

	fixture.AllStreams = []interface{}{first, second}
	fixture.ActiveStreams = []interface{}{first}
	fixture.InactiveStreams = []interface{}{second}

	cloned, err := cloneExpandedState(fixture)

	if err != nil {
		t.Fatal(err)
	}

	cloned.AllStreams[0].(map[string]string)["name"] = "Changed"

	if cloned.AllStreams[1].(map[string]string)["name"] != "Same" {
		t.Fatal("Distinct stream maps were incorrectly merged")
	}

	t.Log("PASS: Distinct maps remain independent")
}

func TestXEPGExpandedRecoveryNilAndEmpty(t *testing.T) {

	var fixture xepgExpandedStateRecoverySnapshot

	fixture.Settings.Files.M3U = map[string]interface{}{}
	fixture.Settings.Filter = map[int64]interface{}{}
	fixture.Settings.FileM3U = []string{}

	fixture.AllStreams = []interface{}{
		nil,
		map[string]string(nil),
		map[string]string{},
	}

	fixture.ActiveStreams = []interface{}{}
	fixture.InactiveStreams = nil

	cloned, err := cloneExpandedState(fixture)

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(fixture, *cloned) {
		t.Fatal("Nil and empty representations changed")
	}

	t.Log("PASS: Nil and empty values preserved")
}

func TestXEPGExpandedRecoveryUnsupportedSettings(t *testing.T) {

	cases := []struct {
		name   string
		modify func(*SettingsStruct)
	}{
		{
			"M3U",
			func(s *SettingsStruct) {
				s.Files.M3U["invalid"] = make(chan int)
			},
		},
		{
			"HDHR",
			func(s *SettingsStruct) {
				s.Files.HDHR["invalid"] = make(chan int)
			},
		},
		{
			"XMLTV",
			func(s *SettingsStruct) {
				s.Files.XMLTV["invalid"] = make(chan int)
			},
		},
		{
			"Filter",
			func(s *SettingsStruct) {
				s.Filter[99] = make(chan int)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			fixture := expandedRecoveryFixture()
			tc.modify(&fixture.Settings)

			cloned, err := cloneExpandedState(fixture)

			if err == nil || cloned != nil {
				t.Fatal("Unsupported settings were accepted")
			}
		})
	}
}

func TestXEPGExpandedRecoveryUnsupportedStreams(t *testing.T) {

	cases := []struct {
		name   string
		modify func(*xepgExpandedStateRecoverySnapshot)
	}{
		{
			"All",
			func(s *xepgExpandedStateRecoverySnapshot) {
				s.AllStreams = []interface{}{42}
			},
		},
		{
			"Active",
			func(s *xepgExpandedStateRecoverySnapshot) {
				s.ActiveStreams = []interface{}{42}
			},
		},
		{
			"Inactive",
			func(s *xepgExpandedStateRecoverySnapshot) {
				s.InactiveStreams = []interface{}{42}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			fixture := expandedRecoveryFixture()
			tc.modify(&fixture)

			cloned, err := cloneExpandedState(fixture)

			if err == nil || cloned != nil {
				t.Fatal("Unsupported stream value accepted")
			}
		})
	}
}

// These tests must not run in parallel because they use
// package-level application state.
func setupExpandedRecoveryGlobals(t *testing.T) {

	t.Helper()

	oldSettings := Settings
	oldData := Data
	oldTemp := System.Folder.Temp
	oldDevice := System.DeviceID

	t.Cleanup(func() {
		providerMutex.Lock()
		Settings = oldSettings
		Data = oldData
		System.Folder.Temp = oldTemp
		System.DeviceID = oldDevice
		providerMutex.Unlock()
	})
}

func TestXEPGExpandedRecoveryRestore(t *testing.T) {

	setupExpandedRecoveryGlobals(t)

	fixture := expandedRecoveryFixture()

	if err := fixture.Restore(); err != nil {
		t.Fatal(err)
	}

	snapshot, err :=
		captureXEPGExpandedStateRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(fixture, *snapshot) {
		t.Fatal("Captured state differs from fixture")
	}

	Settings.UUID = "changed"
	Settings.BackupKeep = 99
	Settings.Files.M3U["provider"].(map[string]interface{})["name"] = "Changed"

	Data.Streams.All[0].(map[string]string)["name"] = "Changed"

	Data.Streams.Active = nil
	Data.Streams.Inactive = nil
	Data.Filter = nil

	Data.Playlist.M3U.Groups.Text = nil
	Data.Playlist.M3U.Groups.Value = nil

	Data.StreamPreviewUI.Active = nil
	Data.StreamPreviewUI.Inactive = nil

	System.Folder.Temp = "changed-temp"
	System.DeviceID = "changed-device"

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	restored, err :=
		captureXEPGExpandedStateRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(fixture, *restored) {
		t.Fatal("Application state was not restored")
	}

	Data.Streams.All[0].(map[string]string)["name"] = "Alias test"

	if Data.Streams.Active[0].(map[string]string)["name"] != "Alias test" {
		t.Fatal("Restored shared references were lost")
	}

	t.Log("PASS: Application state and shared references restored")
}

func TestXEPGExpandedRecoveryRepeatedRestore(t *testing.T) {

	setupExpandedRecoveryGlobals(t)

	fixture := expandedRecoveryFixture()

	if err := fixture.Restore(); err != nil {
		t.Fatal(err)
	}

	snapshot, err :=
		captureXEPGExpandedStateRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {

		Settings.UUID = "changed"

		Data.Streams.All[0].(map[string]string)["name"] = "Changed"

		if err := snapshot.Restore(); err != nil {
			t.Fatal(err)
		}

		restored, err :=
			captureXEPGExpandedStateRecoverySnapshot()

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(fixture, *restored) {
			t.Fatalf("Restore iteration %d failed", i)
		}
	}

	t.Log("PASS: Snapshot reusable for 10 restorations")
}

func TestXEPGExpandedRecoveryInvalidRestore(t *testing.T) {

	setupExpandedRecoveryGlobals(t)

	fixture := expandedRecoveryFixture()

	if err := fixture.Restore(); err != nil {
		t.Fatal(err)
	}

	var missing *xepgExpandedStateRecoverySnapshot

	if err := missing.Restore(); err == nil {
		t.Fatal("Nil snapshot was accepted")
	}

	invalid := expandedRecoveryFixture()
	invalid.InactiveStreams = []interface{}{42}

	if err := invalid.Restore(); err == nil {
		t.Fatal("Invalid snapshot was accepted")
	}

	current, err :=
		captureXEPGExpandedStateRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(fixture, *current) {
		t.Fatal("Failed restoration changed application state")
	}

	t.Log("PASS: Invalid restoration leaves state unchanged")
}
