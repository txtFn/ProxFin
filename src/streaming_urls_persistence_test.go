
package src

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

func TestStreamingURLPersistenceErrors(t *testing.T) {
    originalPath := System.File.URLS

    streamingURLsMu.Lock()
    originalCache := Data.Cache.StreamingURLS
    Data.Cache.StreamingURLS = map[string]StreamInfo{
        "test-stream": {},
    }
    streamingURLsMu.Unlock()

    t.Cleanup(func() {
        System.File.URLS = originalPath

        streamingURLsMu.Lock()
        Data.Cache.StreamingURLS = originalCache
        streamingURLsMu.Unlock()
    })

    t.Run("successful_save", func(t *testing.T) {
        System.File.URLS = filepath.Join(
            t.TempDir(), "urls.json",
        )

        if err := saveStreamingURLs(); err != nil {
            t.Fatalf("expected successful save: %v", err)
        }

        content, err := os.ReadFile(System.File.URLS)
        if err != nil {
            t.Fatalf("could not read saved file: %v", err)
        }

        var saved map[string]StreamInfo

        if err := json.Unmarshal(content, &saved); err != nil {
            t.Fatalf("invalid saved JSON: %v", err)
        }

        if _, exists := saved["test-stream"]; !exists {
            t.Fatal("expected stream missing from saved file")
        }
    })

    t.Run("failed_save_returns_error", func(t *testing.T) {
        System.File.URLS = filepath.Join(
            t.TempDir(),
            "missing-directory",
            "urls.json",
        )

        if err := saveStreamingURLs(); err == nil {
            t.Fatal("expected file-writing error")
        }
    })

    t.Run("nil_cache_is_safe", func(t *testing.T) {
        streamingURLsMu.Lock()
        previousCache := Data.Cache.StreamingURLS
        Data.Cache.StreamingURLS = nil
        streamingURLsMu.Unlock()

        defer func() {
            streamingURLsMu.Lock()
            Data.Cache.StreamingURLS = previousCache
            streamingURLsMu.Unlock()
        }()

        System.File.URLS = filepath.Join(
            t.TempDir(), "urls.json",
        )

        if err := saveStreamingURLs(); err != nil {
            t.Fatalf("nil cache should not fail: %v", err)
        }

        if _, err := os.Stat(System.File.URLS); !os.IsNotExist(err) {
            t.Fatalf("expected no file creation, got: %v", err)
        }
    })
}
