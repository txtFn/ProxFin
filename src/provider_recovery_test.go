
package src

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderErrorAggregation(t *testing.T) {
	tests := []struct {
		name       string
		providers  map[string]bool
		cached     bool
		selected   string
		wantErrors []string
	}{
		{
			name: "AllProvidersSuccessful",
			providers: map[string]bool{
				"Mgood1": true,
				"Mgood2": true,
			},
		},
		{
			name: "OneProviderFails",
			providers: map[string]bool{
				"Mgood": true,
				"Mbad":  false,
			},
			wantErrors: []string{"Mbad"},
		},
		{
			name: "MultipleProvidersFail",
			providers: map[string]bool{
				"Mgood": true,
				"Mbad1": false,
				"Mbad2": false,
			},
			wantErrors: []string{"Mbad1", "Mbad2"},
		},
		{
			name: "FailedProviderWithCache",
			providers: map[string]bool{
				"Mbad":  false,
				"Mgood": true,
			},
			cached:     true,
			wantErrors: []string{"Mbad"},
		},
		{
			name: "FailedProviderWithoutCache",
			providers: map[string]bool{
				"Mbad": false,
			},
			wantErrors: []string{"Mbad"},
		},
		{
			name: "SelectedProviderOnly",
			providers: map[string]bool{
				"Mgood": true,
				"Mbad":  false,
			},
			selected: "Mgood",
		},
		{
			name:      "NoConfiguredProviders",
			providers: map[string]bool{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir() + string(os.PathSeparator)

			savedFiles := Settings.Files
			savedData := System.Folder.Data
			savedSettingsFile := System.File.Settings
			savedInfo := System.Flag.Info

			t.Cleanup(func() {
				Settings.Files = savedFiles
				System.Folder.Data = savedData
				System.File.Settings = savedSettingsFile
				System.Flag.Info = savedInfo
			})

			System.Folder.Data = dir
			System.File.Settings = filepath.Join(
				dir, "settings.json",
			)
			System.Flag.Info = true

			Settings.Files.M3U = make(map[string]interface{})

			playlist := []byte(
				"#EXTM3U\n" +
					"#EXTINF:-1 tvg-id=\"a\" group-title=\"Test\",Test Channel\n" +
					"http://example.invalid/test.m3u8\n",
			)

			for id, successful := range tc.providers {
				source := filepath.Join(
					dir, id+"-source.m3u",
				)

				if successful {
					if err := os.WriteFile(
						source, playlist, 0644,
					); err != nil {
						t.Fatal(err)
					}
				}

				if tc.cached && !successful {
					cache := filepath.Join(
						dir, id+".m3u",
					)

					if err := os.WriteFile(
						cache, playlist, 0644,
					); err != nil {
						t.Fatal(err)
					}
				}

				Settings.Files.M3U[id] = map[string]interface{}{
					"name":             id,
					"file.source":      source,
					"type":             "m3u",
					"tuner":            2.0,
					"counter.download": 0.0,
					"counter.error":    0.0,
				}
			}

			// Repeat to exercise different map iteration orders.
			for attempt := 0; attempt < 20; attempt++ {
				err := getProviderData(
					"m3u", tc.selected,
				)

				if len(tc.wantErrors) == 0 {
					if err != nil {
						t.Fatalf(
							"unexpected error: %v", err,
						)
					}
					continue
				}

				if err == nil {
					t.Fatalf(
						"attempt %d: expected provider errors",
						attempt,
					)
				}

				for _, id := range tc.wantErrors {
					expected := "provider " + id + " (m3u) failed:"

					if !strings.Contains(
						err.Error(), expected,
					) {
						t.Errorf(
							"missing error for %s: %v",
							id, err,
						)
					}
				}
			}

			t.Logf("PASS: %s", tc.name)
		})
	}
}
