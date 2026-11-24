package src

import (
    "bytes"
    "errors"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

type m3uPersistenceFailWriter struct{}

func (m3uPersistenceFailWriter) Write(p []byte) (int, error) {
    return 0, errors.New("simulated writer failure")
}

func TestM3UPersistenceErrors(t *testing.T) {
    originalM3U := System.File.M3U
    originalURLs := System.File.URLS
    originalChannels := Data.XEPG.Channels

    streamingURLsMu.Lock()
    originalCache := Data.Cache.StreamingURLS
    Data.Cache.StreamingURLS = map[string]StreamInfo{
        "test-stream": {},
    }
    streamingURLsMu.Unlock()

    Data.XEPG.Channels = nil

    t.Cleanup(func() {
        System.File.M3U = originalM3U
        System.File.URLS = originalURLs
        Data.XEPG.Channels = originalChannels

        streamingURLsMu.Lock()
        Data.Cache.StreamingURLS = originalCache
        streamingURLsMu.Unlock()
    })

    t.Run("successful_generation", func(t *testing.T) {
        dir := t.TempDir()

        System.File.M3U = filepath.Join(dir, "playlist.m3u")
        System.File.URLS = filepath.Join(dir, "urls.json")

        if err := createM3UFile(); err != nil {
            t.Fatalf("expected successful generation: %v", err)
        }

        content, err := os.ReadFile(System.File.M3U)
        if err != nil {
            t.Fatal(err)
        }

        if !bytes.HasPrefix(content, []byte("#EXTM3U")) {
            t.Fatal("invalid M3U header")
        }

        if _, err := os.Stat(System.File.URLS); err != nil {
            t.Fatalf("URL persistence failed: %v", err)
        }
    })

    t.Run("m3u_write_failure", func(t *testing.T) {
        dir := t.TempDir()

        System.File.M3U = filepath.Join(
            dir, "missing-directory", "playlist.m3u",
        )
        System.File.URLS = filepath.Join(dir, "urls.json")

        err := createM3UFile()

        if err == nil {
            t.Fatal("expected M3U generation error")
        }

        if !strings.Contains(err.Error(), "M3U generation failed") {
            t.Fatalf("unexpected error: %v", err)
        }

        if _, err := os.Stat(System.File.URLS); !os.IsNotExist(err) {
            t.Fatalf("URL persistence should have been skipped: %v", err)
        }
    })

    t.Run("url_persistence_failure", func(t *testing.T) {
        dir := t.TempDir()

        System.File.M3U = filepath.Join(dir, "playlist.m3u")
        System.File.URLS = filepath.Join(
            dir, "missing-directory", "urls.json",
        )

        err := createM3UFile()

        if err == nil {
            t.Fatal("expected URL persistence error")
        }

        if !strings.Contains(
            err.Error(), "streaming URL persistence failed",
        ) {
            t.Fatalf("unexpected error: %v", err)
        }
    })

    t.Run("preserve_existing_playlist", func(t *testing.T) {
        dir := t.TempDir()
        System.File.M3U = filepath.Join(dir, "playlist.m3u")

        original := []byte(
            "#EXTM3U\n#EXTINF:-1,Original Channel\n" +
                "https://example.invalid/original.m3u8\n",
        )

        if err := os.WriteFile(
            System.File.M3U, original, 0644,
        ); err != nil {
            t.Fatal(err)
        }

        if err := saveM3UFile(m3uPersistenceFailWriter{}); err == nil {
            t.Fatal("expected simulated writer failure")
        }

        content, err := os.ReadFile(System.File.M3U)
        if err != nil {
            t.Fatal(err)
        }

        if !bytes.Equal(content, original) {
            t.Fatal("existing playlist was modified after failure")
        }
    })
}
