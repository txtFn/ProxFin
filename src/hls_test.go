package src

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHLSHandlerRejectsInvalidMethods(t *testing.T) {
	methods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodOptions,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(
				method,
				"/hls/34f819bf4eca2fbc0c5ab121399e20fb/index.m3u8",
				nil,
			)
			rec := httptest.NewRecorder()

			HLSHandler(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d",
					rec.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestHLSHandlerRejectsInvalidPaths(t *testing.T) {
	paths := []string{
		"/hls/",
		"/hls/invalid/index.m3u8",
		"/hls/34f819bf4eca2fbc0c5ab121399e20fb/",
		"/hls/34f819bf4eca2fbc0c5ab121399e20fb/invalid.ts",
		"/hls/34f819bf4eca2fbc0c5ab121399e20fb/seg.ts",
		"/hls/34f819bf4eca2fbc0c5ab121399e20fb/seg000001.ts/extra",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			HLSHandler(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d",
					rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHLSHandlerUnknownChannelReturnsBadGateway(t *testing.T) {
	setupStreamingURLs(t)

	channelID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	hlsMu.Lock()
	_, existed := hlsSessions[channelID]
	hlsMu.Unlock()

	if existed {
		t.Fatal("test channel unexpectedly already has an HLS session")
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/hls/"+channelID+"/index.m3u8",
		nil,
	)
	rec := httptest.NewRecorder()

	HLSHandler(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body = %s",
			rec.Code, http.StatusBadGateway, rec.Body.String())
	}

	hlsMu.Lock()
	_, exists := hlsSessions[channelID]
	hlsMu.Unlock()

	if exists {
		t.Fatal("HLS session was created for an unknown channel")
	}
}

func TestHLSHandlerFFmpegStartFailure(t *testing.T) {
	setupStreamingURLs(t)

	oldPath := Settings.FFmpegPath
	Settings.FFmpegPath = filepath.Join(
		t.TempDir(),
		"missing-ffmpeg.exe",
	)
	t.Cleanup(func() {
		Settings.FFmpegPath = oldPath
	})

	channelID := streamID(t, 77)

	hlsMu.Lock()
	_, existed := hlsSessions[channelID]
	hlsMu.Unlock()

	if existed {
		t.Fatal("test channel unexpectedly already has an HLS session")
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/hls/"+channelID+"/index.m3u8",
		nil,
	)
	rec := httptest.NewRecorder()

	HLSHandler(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf(
			"status = %d, want %d; body = %s",
			rec.Code,
			http.StatusBadGateway,
			rec.Body.String(),
		)
	}

	hlsMu.Lock()
	_, exists := hlsSessions[channelID]
	hlsMu.Unlock()

	if exists {
		t.Fatal("failed FFmpeg startup left an HLS session registered")
	}
}

// fakeHLSSource implements a minimal FFmpeg substitute for HLS tests.
// It supports FFmpeg HTTP protocol help and creates a playlist and segment.
const fakeHLSSource = `package main

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"
)

func main() {
    args := os.Args[1:]

    for _, arg := range args {
        if arg == "-h" {
            fmt.Println("-reconnect <boolean>")
            fmt.Println("-reconnect_streamed <boolean>")
            fmt.Println("-reconnect_delay_max <int>")
            return
        }
    }

    playlist := args[len(args)-1]
    var pattern string

    for i, arg := range args {
        if arg == "-hls_segment_filename" && i+1 < len(args) {
            pattern = args[i+1]
            break
        }
    }

    if pattern == "" {
        os.Exit(2)
    }

    segment := strings.Replace(pattern, "%06d", "000001", 1)

    if err := os.MkdirAll(filepath.Dir(playlist), 0755); err != nil {
        os.Exit(3)
    }

    if err := os.WriteFile(segment, []byte("fake transport stream"), 0644); err != nil {
        os.Exit(4)
    }

    contents := fmt.Sprintf(
        "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:4.0,\n%s\n",
        filepath.Base(segment),
    )

    if err := os.WriteFile(playlist, []byte(contents), 0644); err != nil {
        os.Exit(5)
    }

    for {
        time.Sleep(time.Second)
    }
}
`

func setupHLSTest(t *testing.T) {
	t.Helper()

	setupStreamingURLs(t)

	dir := t.TempDir()
	source := filepath.Join(dir, "fake_ffmpeg.go")
	binary := filepath.Join(dir, "fake_ffmpeg.exe")

	if err := os.WriteFile(source, []byte(fakeHLSSource), 0600); err != nil {
		t.Fatal(err)
	}

	build := exec.Command("go", "build", "-o", binary, source)
	build.Dir = dir
	build.Env = append(os.Environ(), "GO111MODULE=off")

	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building fake FFmpeg: %v\n%s", err, output)
	}

	oldPath := Settings.FFmpegPath
	oldForceHTTP := Settings.FFmpegForceHttp
	oldAgent := Settings.UserAgent
	oldM3U := Settings.Files.M3U

	Settings.FFmpegPath = binary
	Settings.FFmpegForceHttp = false
	Settings.UserAgent = ""
	Settings.Files.M3U = map[string]interface{}{
		"Mtest": map[string]interface{}{
			"buffer": "ffmpeg",
			"tuner":  "4",
			"name":   "HLS test",
		},
	}

	t.Cleanup(func() {
		Settings.FFmpegPath = oldPath
		Settings.FFmpegForceHttp = oldForceHTTP
		Settings.UserAgent = oldAgent
		Settings.Files.M3U = oldM3U
	})
}

func hlsTestRequest(t *testing.T, channelID string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/hls/%s/index.m3u8", channelID),
		nil,
	)

	rec := httptest.NewRecorder()
	HLSHandler(rec, req)
	return rec
}

func cleanupHLSTestSessions(t *testing.T, channelIDs ...string) {
	t.Helper()

	for _, id := range channelIDs {
		hlsMu.Lock()
		s := hlsSessions[id]
		hlsMu.Unlock()

		if s == nil {
			continue
		}

		killProcessGroup(s.cmd)

		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			t.Errorf("HLS session %s did not stop", id)
		}
	}
}

// Multiple requests for the same channel must reuse one FFmpeg process.
func TestHLSHandlerReusesSession(t *testing.T) {
	setupHLSTest(t)

	channelID := streamID(t, 901)
	defer cleanupHLSTestSessions(t, channelID)

	first := hlsTestRequest(t, channelID)

	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d; body = %s",
			first.Code, first.Body.String())
	}

	if !strings.Contains(first.Body.String(), "seg000001.ts") {
		t.Fatalf("unexpected HLS playlist: %s", first.Body.String())
	}

	hlsMu.Lock()
	firstSession := hlsSessions[channelID]
	hlsMu.Unlock()

	if firstSession == nil {
		t.Fatal("first request did not create an HLS session")
	}

	second := hlsTestRequest(t, channelID)

	if second.Code != http.StatusOK {
		t.Fatalf("second request status = %d; body = %s",
			second.Code, second.Body.String())
	}

	hlsMu.Lock()
	secondSession := hlsSessions[channelID]
	hlsMu.Unlock()

	if firstSession != secondSession {
		t.Fatal("second request created a duplicate HLS session")
	}
}

// Concurrent requests for one channel must share the same session.
func TestHLSHandlerConcurrentRequestsShareSession(t *testing.T) {
	setupHLSTest(t)

	channelID := streamID(t, 902)
	defer cleanupHLSTestSessions(t, channelID)

	const requests = 8

	var wg sync.WaitGroup
	errors := make(chan string, requests)

	for i := 0; i < requests; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			rec := hlsTestRequest(t, channelID)

			if rec.Code != http.StatusOK {
				errors <- fmt.Sprintf(
					"HTTP status %d: %s",
					rec.Code,
					rec.Body.String(),
				)
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}

	hlsMu.Lock()
	session := hlsSessions[channelID]
	hlsMu.Unlock()

	if session == nil {
		t.Fatal("concurrent requests did not create an HLS session")
	}
}

func TestHLSHandlerCreatesIndependentSessions(t *testing.T) {
	setupHLSTest(t)

	channelA := streamID(t, 903)
	channelB := streamID(t, 904)

	defer cleanupHLSTestSessions(t, channelA, channelB)

	first := hlsTestRequest(t, channelA)
	if first.Code != http.StatusOK {
		t.Fatalf("channel A returned HTTP %d: %s",
			first.Code, first.Body.String())
	}

	second := hlsTestRequest(t, channelB)
	if second.Code != http.StatusOK {
		t.Fatalf("channel B returned HTTP %d: %s",
			second.Code, second.Body.String())
	}

	hlsMu.Lock()
	sessionA := hlsSessions[channelA]
	sessionB := hlsSessions[channelB]
	hlsMu.Unlock()

	if sessionA == nil || sessionB == nil {
		t.Fatal("one or both HLS sessions were not created")
	}

	if sessionA == sessionB {
		t.Fatal("different channels unexpectedly share one session")
	}

	if sessionA.cmd.Process.Pid == sessionB.cmd.Process.Pid {
		t.Fatal("different channels unexpectedly share one process")
	}

	// Stop channel A only.
	killProcessGroup(sessionA.cmd)

	select {
	case <-sessionA.done:
	case <-time.After(5 * time.Second):
		t.Fatal("channel A did not stop")
	}

	// Channel B must remain registered and running.
	hlsMu.Lock()
	remaining := hlsSessions[channelB] == sessionB
	hlsMu.Unlock()

	if !remaining {
		t.Fatal("stopping channel A affected channel B")
	}
}
