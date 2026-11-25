package src

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

func expandedMemoryFixture(
	count int,
) xepgExpandedStateRecoverySnapshot {

	fixture := expandedRecoveryFixture()

	fixture.AllStreams = make([]interface{}, 0, count)
	fixture.ActiveStreams = make([]interface{}, 0, count)
	fixture.InactiveStreams = make([]interface{}, 0, count)

	fixture.ActivePreview = nil
	fixture.InactivePreview = nil

	for i := 0; i < count; i++ {

		id := fmt.Sprintf("channel-%06d", i)

		stream := map[string]string{
			"name":           id,
			"tvg-id":         id,
			"tvg-name":       id,
			"tvg-logo":       "https://example.invalid/logo/" + id,
			"group-title":    fmt.Sprintf("Group-%d", i%100),
			"url":            "https://example.invalid/live/" + id,
			"tvg-chno":       fmt.Sprintf("%d", i+1),
			"_file.m3u.id":   "test-provider",
			"_file.m3u.name": "Synthetic provider",
			"_file.m3u.path": "test-provider.m3u",
			"_uuid.key":      "tvg-id",
			"_uuid.value":    id,
		}

		fixture.AllStreams = append(
			fixture.AllStreams, stream,
		)

		if i%5 == 0 {

			fixture.InactiveStreams = append(
				fixture.InactiveStreams, stream,
			)

			fixture.InactivePreview = append(
				fixture.InactivePreview, id,
			)

		} else {

			fixture.ActiveStreams = append(
				fixture.ActiveStreams, stream,
			)

			fixture.ActivePreview = append(
				fixture.ActivePreview, id,
			)
		}
	}

	return fixture
}

func expandedMemoryMiB(bytes uint64) float64 {
	return float64(bytes) / 1048576
}

func TestXEPGExpandedRecoveryMemoryScale(t *testing.T) {

	if testing.Short() {
		t.Skip("Large channel memory test")
	}

	sizes := []int{
		424,
		5000,
		12268,
		15000,
	}

	for _, count := range sizes {

		t.Run(fmt.Sprintf("Channels_%d", count),
			func(t *testing.T) {

				setupExpandedRecoveryGlobals(t)

				fixture := expandedMemoryFixture(count)

				if err := fixture.Restore(); err != nil {
					t.Fatal(err)
				}

				fixture = xepgExpandedStateRecoverySnapshot{}

				runtime.GC()

				var baseline runtime.MemStats
				var captured runtime.MemStats
				var restored runtime.MemStats
				var collected runtime.MemStats

				runtime.ReadMemStats(&baseline)

				captureStart := time.Now()

				snapshot, err :=
					captureXEPGExpandedStateRecoverySnapshot()

				captureDuration := time.Since(captureStart)

				if err != nil {
					t.Fatal(err)
				}

				runtime.ReadMemStats(&captured)

				if len(snapshot.AllStreams) != count {
					t.Fatal("Snapshot channel count mismatch")
				}

				if len(snapshot.ActiveStreams)+
					len(snapshot.InactiveStreams) != count {
					t.Fatal("Stream classification mismatch")
				}

				Data.Streams.All[0].(map[string]string)["name"] = "Changed"

				Data.Streams.All = nil
				Data.Streams.Active = nil
				Data.Streams.Inactive = nil

				Data.StreamPreviewUI.Active = nil
				Data.StreamPreviewUI.Inactive = nil

				Settings.UUID = "changed"

				restoreStart := time.Now()

				if err := snapshot.Restore(); err != nil {
					t.Fatal(err)
				}

				restoreDuration := time.Since(restoreStart)

				runtime.ReadMemStats(&restored)

				if len(Data.Streams.All) != count {
					t.Fatal("Restored channel count mismatch")
				}

				if len(Data.Streams.Active)+
					len(Data.Streams.Inactive) != count {
					t.Fatal("Restored classification mismatch")
				}

				if Settings.UUID != "test-original-uuid" {
					t.Fatal("Settings UUID was not restored")
				}

				for i, item := range Data.Streams.All {

					expected := fmt.Sprintf(
						"channel-%06d", i,
					)

					stream := item.(map[string]string)

					if stream["name"] != expected {
						t.Fatalf(
							"Channel %d restoration failed", i,
						)
					}
				}

				Data.Streams.All[0].(map[string]string)["name"] = "Alias check"

				if Data.Streams.Inactive[0].(map[string]string)["name"] != "Alias check" {
					t.Fatal("Inactive reference was lost")
				}

				Data.Streams.All[1].(map[string]string)["name"] = "Alias check"

				if Data.Streams.Active[0].(map[string]string)["name"] != "Alias check" {
					t.Fatal("Active reference was lost")
				}

				if snapshot.AllStreams[0].(map[string]string)["name"] != "channel-000000" {
					t.Fatal("Restored state aliases snapshot")
				}

				runtime.KeepAlive(snapshot)
				snapshot = nil

				runtime.GC()
				runtime.ReadMemStats(&collected)

				captureDelta :=
					float64(captured.HeapAlloc) -
						float64(baseline.HeapAlloc)

				t.Logf("CHANNELS: %d", count)

				t.Logf(
					"BASELINE_HEAP_MIB: %.2f",
					expandedMemoryMiB(baseline.HeapAlloc),
				)

				t.Logf(
					"CAPTURE_HEAP_MIB: %.2f",
					expandedMemoryMiB(captured.HeapAlloc),
				)

				t.Logf(
					"CAPTURE_HEAP_DELTA_MIB: %.2f",
					captureDelta/1048576,
				)

				t.Logf(
					"RESTORE_HEAP_MIB: %.2f",
					expandedMemoryMiB(restored.HeapAlloc),
				)

				t.Logf(
					"POST_GC_HEAP_MIB: %.2f",
					expandedMemoryMiB(collected.HeapAlloc),
				)

				t.Logf(
					"GO_SYS_AFTER_RESTORE_MIB: %.2f",
					expandedMemoryMiB(restored.Sys),
				)

				t.Logf(
					"CAPTURE_DURATION: %s",
					captureDuration,
				)

				t.Logf(
					"RESTORE_DURATION: %s",
					restoreDuration,
				)

				t.Log(
					"PASS: Channel count, settings, " +
						"stream references and snapshot isolation",
				)
			},
		)
	}
}
