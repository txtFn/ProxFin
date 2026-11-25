package src

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func recoveryMemoryMiB(value uint64) float64 {
	return float64(value) / 1048576.0
}

func recoveryMemoryDelta(before, after uint64) float64 {
	return (float64(after) - float64(before)) / 1048576.0
}

func readRecoveryMemory() runtime.MemStats {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats
}

func populateRecoveryMemoryTest(
	t *testing.T,
	count int,
) []string {

	t.Helper()

	setupChannelMappingRecoveryTest(t)
	setupStreamingURLRecoveryTest(t)

	Data.XEPG.Channels =
		make(map[string]interface{}, count)

	Data.XEPG.ChannelsFiltered =
		make(map[string]interface{}, count)

	Data.Cache.Streams.Active =
		make([]string, 0, count)

	Data.XMLTV.Mapping =
		make(map[string]interface{}, count)

	Data.XMLTV.Files =
		[]string{"synthetic-guide.xml"}

	streamingURLsMu.Lock()
	defer streamingURLsMu.Unlock()

	Data.Cache.StreamingURLS =
		make(map[string]StreamInfo, count)

	streamingURLsAge =
		make(map[string]uint64, count)

	streamingURLsGen = 10

	for i := 0; i < count; i++ {

		id := fmt.Sprintf("channel-%06d", i)

		name := fmt.Sprintf(
			"Synthetic Channel %06d", i,
		)

		epg := fmt.Sprintf(
			"synthetic-%06d.epg", i,
		)

		channel := XEPGChannelStruct{
			XActive:        true,
			XName:          name,
			XMapping:       epg,
			BackupChannel1: &BackupStream{},
		}

		Data.XEPG.Channels[id] = channel

		Data.XEPG.ChannelsFiltered[id] =
			map[string]interface{}{
				"name":    name,
				"active":  true,
				"mapping": epg,
				"metadata": map[string]interface{}{
					"group": "Synthetic Group",
					"tags": []string{
						"test",
						"benchmark",
					},
				},
			}

		Data.Cache.Streams.Active =
			append(Data.Cache.Streams.Active, id)

		Data.XMLTV.Mapping[epg] =
			map[string]interface{}{
				"id":   epg,
				"name": name,
			}

		Data.Cache.StreamingURLS[id] =
			StreamInfo{
				BackupChannel1: &BackupStream{},
			}

		streamingURLsAge[id] = 10
	}

	Data.XEPG.XEPGCount = int64(count)

	root := t.TempDir()

	paths := []string{
		filepath.Join(root, "xepg.json"),
		filepath.Join(root, "urls.json"),
		filepath.Join(root, "proxfin.m3u"),
		filepath.Join(root, "proxfin.xml"),
		filepath.Join(root, "proxfin.xml.gz"),
	}

	channelJSON, err :=
		json.Marshal(Data.XEPG.Channels)

	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		paths[0], channelJSON, 0600,
	); err != nil {
		t.Fatal(err)
	}

	streamJSON, err :=
		json.Marshal(Data.Cache.StreamingURLS)

	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		paths[1], streamJSON, 0600,
	); err != nil {
		t.Fatal(err)
	}

	for _, path := range paths[2:] {
		if err := os.WriteFile(
			path,
			[]byte("synthetic benchmark data"),
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}

	return paths
}

func TestCombinedXEPGRecoveryMemoryScale(
	t *testing.T,
) {

	counts := []int{
		424,
		5000,
		12268,
		15000,
	}

	for _, count := range counts {

		t.Run(
			fmt.Sprintf("channels-%d", count),
			func(t *testing.T) {

				paths :=
					populateRecoveryMemoryTest(
						t, count,
					)

				root := filepath.Dir(paths[0])

				runtime.GC()

				baseline := readRecoveryMemory()

				captureStart := time.Now()

				snapshot, err :=
					captureCombinedXEPGRecoverySnapshot(
						root,
						paths,
					)

				captureDuration :=
					time.Since(captureStart)

				if err != nil {
					t.Fatal(err)
				}

				afterCapture :=
					readRecoveryMemory()

				runtime.GC()

				retained :=
					readRecoveryMemory()

				restoreStart := time.Now()

				if err := snapshot.Restore(); err != nil {
					t.Fatal(err)
				}

				restoreDuration :=
					time.Since(restoreStart)

				afterRestore :=
					readRecoveryMemory()

				if len(Data.XEPG.Channels) != count {
					t.Fatal("Restored channel count mismatch")
				}

				if Data.XEPG.XEPGCount != int64(count) {
					t.Fatal("Restored XEPG count mismatch")
				}

				streamingURLsMu.RLock()

				streamCount :=
					len(Data.Cache.StreamingURLS)

				streamingURLsMu.RUnlock()

				if streamCount != count {
					t.Fatal("Restored stream count mismatch")
				}

				runtime.GC()

				retainedAfterRestore :=
					readRecoveryMemory()

				discardStart := time.Now()

				if err := snapshot.Discard(); err != nil {
					t.Fatal(err)
				}

				discardDuration :=
					time.Since(discardStart)

				runtime.GC()

				afterDiscard :=
					readRecoveryMemory()

				fmt.Printf(
					"\n=== MEMORY BENCHMARK: %d CHANNELS ===\n",
					count,
				)

				fmt.Printf(
					"Baseline heap: %.2f MiB\n",
					recoveryMemoryMiB(baseline.HeapAlloc),
				)

				fmt.Printf(
					"Heap after capture: %.2f MiB\n",
					recoveryMemoryMiB(afterCapture.HeapAlloc),
				)

				fmt.Printf(
					"Retained heap after capture GC: %.2f MiB\n",
					recoveryMemoryMiB(retained.HeapAlloc),
				)

				fmt.Printf(
					"Retained snapshot overhead: %.2f MiB\n",
					recoveryMemoryDelta(
						baseline.HeapAlloc,
						retained.HeapAlloc,
					),
				)

				fmt.Printf(
					"Capture cumulative allocations: %.2f MiB\n",
					recoveryMemoryDelta(
						baseline.TotalAlloc,
						afterCapture.TotalAlloc,
					),
				)

				fmt.Printf(
					"Heap after restoration: %.2f MiB\n",
					recoveryMemoryMiB(
						afterRestore.HeapAlloc,
					),
				)

				fmt.Printf(
					"Restore cumulative allocations: %.2f MiB\n",
					recoveryMemoryDelta(
						retained.TotalAlloc,
						afterRestore.TotalAlloc,
					),
				)

				fmt.Printf(
					"Retained heap after restoration GC: %.2f MiB\n",
					recoveryMemoryMiB(
						retainedAfterRestore.HeapAlloc,
					),
				)

				fmt.Printf(
					"Heap after discard and GC: %.2f MiB\n",
					recoveryMemoryMiB(
						afterDiscard.HeapAlloc,
					),
				)

				fmt.Printf(
					"Go runtime system memory after restore: %.2f MiB\n",
					recoveryMemoryMiB(
						afterRestore.Sys,
					),
				)

				fmt.Printf(
					"Capture duration: %s\n",
					captureDuration,
				)

				fmt.Printf(
					"Restore duration: %s\n",
					restoreDuration,
				)

				fmt.Printf(
					"Discard duration: %s\n",
					discardDuration,
				)

				fmt.Printf(
					"Verified channels: %d\n",
					count,
				)

				fmt.Println("BENCHMARK PASS")
			},
		)
	}
}
