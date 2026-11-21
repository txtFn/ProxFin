package src

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	hlsStallTimeout    = 30 * time.Second
	hlsStartupGrace    = 45 * time.Second
	hlsIdleTimeout     = 5 * time.Minute
	hlsMonitorInterval = 5 * time.Second
	hlsPlaylistWait    = 25 * time.Second
)

var (
	hlsMu           sync.Mutex
	hlsSessions     = make(map[string]*hlsSession)
	hlsNextSequence = make(map[string]int64)
	hlsFileRE       = regexp.MustCompile(`^seg[0-9]{6,}\.ts$`)
	hlsChannelRE    = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
)

type hlsSession struct {
	dir             string
	cmd             *exec.Cmd
	done            chan struct{}
	registered      chan struct{}
	mu              sync.Mutex
	readers         sync.WaitGroup
	stopping        bool
	lastAccess      time.Time
	activeReaders   int
	started         time.Time
	lastPlaylistMod time.Time
	lastProgress    time.Time
	startSequence   int64
}

func HLSHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/hls/"), "/")
	if len(parts) != 2 || !hlsChannelRE.MatchString(parts[0]) {
		http.NotFound(w, r)
		return
	}
	filename := parts[1]
	if filename != "index.m3u8" && !hlsFileRE.MatchString(filename) {
		http.NotFound(w, r)
		return
	}
	channelID := parts[0]
	deadline := time.Now().Add(hlsPlaylistWait)

	// During a restart, playlist requests wait briefly for the replacement
	// instead of immediately returning 503 to a player.
	for {
		hlsMu.Lock()
		s := hlsSessions[channelID]
		if s == nil {
			if filename != "index.m3u8" {
				hlsMu.Unlock()
				http.NotFound(w, r)
				return
			}
			var err error
			s, err = startHLSSession(channelID)
			if err != nil {
				hlsMu.Unlock()
				log.Printf("HLS channel %s: start failed: %v", channelID, err)
				http.Error(w, "Unable to start HLS stream", http.StatusBadGateway)
				return
			}
			hlsSessions[channelID] = s
			close(s.registered)
			go monitorHLSSession(channelID, s)
		}
		s.mu.Lock()
		if s.stopping {
			s.mu.Unlock()
			hlsMu.Unlock()
			if filename != "index.m3u8" {
				http.NotFound(w, r)
				return
			}
			if !waitForHLSRetry(r, deadline) {
				http.Error(w, "HLS recovery in progress", http.StatusServiceUnavailable)
				return
			}
			continue
		}
		s.lastAccess = time.Now()
		s.activeReaders++
		s.readers.Add(1)
		s.mu.Unlock()
		hlsMu.Unlock()

		serveHLSFile(w, r, s, filename, deadline)
		s.mu.Lock()
		s.activeReaders--
		s.mu.Unlock()
		s.readers.Done()
		return
	}
}

func waitForHLSRetry(r *http.Request, deadline time.Time) bool {
	if time.Now().After(deadline) || r.Context().Err() != nil {
		return false
	}
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-r.Context().Done():
		return false
	case <-timer.C:
		return true
	}
}

func serveHLSFile(w http.ResponseWriter, r *http.Request, s *hlsSession, filename string, deadline time.Time) {
	path := filepath.Join(s.dir, filename)
	if filename == "index.m3u8" {
		for {
			if info, err := os.Stat(path); err == nil && info.Size() > 0 {
				break
			}
			select {
			case <-s.done:
				http.Error(w, "HLS process stopped", http.StatusBadGateway)
				return
			case <-r.Context().Done():
				return
			default:
			}
			if time.Now().After(deadline) {
				http.Error(w, "HLS playlist not ready", http.StatusGatewayTimeout)
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	http.ServeFile(w, r, path)
}

func startHLSSession(channelID string) (*hlsSession, error) {
	info, err := getStreamInfo(channelID)
	if err != nil {
		return nil, err
	}
	if !isHTTPURL(info.URL) {
		return nil, fmt.Errorf("HLS requires an HTTP source")
	}
	dir, err := os.MkdirTemp("", "proxfin-hls-")
	if err != nil {
		return nil, err
	}
	path := Settings.FFmpegPath
	if path == "" {
		path = "ffmpeg"
	}
	url := info.URL
	if Settings.FFmpegForceHttp {
		url = strings.Replace(url, "https://", "http://", 1)
	}
	args := []string{"-hide_banner", "-loglevel", "warning"}
	if Settings.UserAgent != "" {
		args = append(args, "-user_agent", Settings.UserAgent)
	}
	t := playlistType(info.PlaylistID)
	proxyIP := getProviderParameter(info.PlaylistID, t, "http_proxy.ip")
	proxyPort := getProviderParameter(info.PlaylistID, t, "http_proxy.port")
	referer := getProviderParameter(info.PlaylistID, t, "http_headers.referer")
	origin := getProviderParameter(info.PlaylistID, t, "http_headers.origin")
	if proxyIP != "" && proxyPort != "" {
		args = append(args, "-http_proxy", fmt.Sprintf("http://%s:%s", proxyIP, proxyPort))
	}
	args = append(args, ffmpegReconnectArgs(path)...)
	var headers string
	if referer != "" {
		headers += "Referer: " + referer + "\r\n"
	}
	if origin != "" {
		headers += "Origin: " + origin + "\r\n"
	}
	if headers != "" {
		args = append(args, "-headers", headers)
	}

	// Called while hlsMu is locked. Keep segment numbers increasing across
	// FFmpeg processes so a client never mistakes a new segment for an old one.
	sequence := hlsNextSequence[channelID]
	flags := "delete_segments+omit_endlist"
	if sequence > 0 {
		flags += "+discont_start"
	}
	args = append(args,
		"-analyzeduration", "1000000",
		"-probesize", "1000000",
		"-i", url,
		"-map", "0:v:0?",
		"-map", "0:a:0?",
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "192k",
		"-ac", "2",
		"-f", "hls",
		"-hls_time", "4",
		"-hls_list_size", "6",
		"-hls_delete_threshold", "2",
		"-start_number", strconv.FormatInt(sequence, 10),
		"-hls_flags", flags,
		"-hls_segment_filename", filepath.Join(dir, "seg%06d.ts"),
		filepath.Join(dir, "index.m3u8"),
	)
	cmd := exec.Command(path, args...)
	setProcessGroup(cmd)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	now := time.Now()
	s := &hlsSession{
		dir: dir, cmd: cmd,
		done: make(chan struct{}), registered: make(chan struct{}),
		lastAccess: now, started: now, lastProgress: now,
		startSequence: sequence,
	}
	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("HLS channel %s: FFmpeg exited: %v", channelID, err)
		}
		<-s.registered
		hlsMu.Lock()
		s.mu.Lock()
		s.stopping = true
		s.mu.Unlock()
		if hlsSessions[channelID] == s {
			// Read the final playlist before deleting the directory. This
			// avoids reusing segment numbers when the process is restarted.
			next := nextHLSSequence(s)
			if next > hlsNextSequence[channelID] {
				hlsNextSequence[channelID] = next
			}
			delete(hlsSessions, channelID)
		}
		hlsMu.Unlock()
		close(s.done)
		s.readers.Wait()
		os.RemoveAll(dir)
	}()
	log.Printf("HLS channel %s: started FFmpeg at sequence %d", channelID, sequence)
	return s, nil
}

func nextHLSSequence(s *hlsSession) int64 {
	next := s.startSequence + 1
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return next
	}
	for _, entry := range entries {
		name := entry.Name()
		if !hlsFileRE.MatchString(name) {
			continue
		}
		number, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(name, "seg"), ".ts"), 10, 64)
		if err == nil && number >= next {
			next = number + 1
		}
	}
	// FFmpeg deletes old segments; the playlist media sequence is an
	// additional lower bound for the next start number.
	data, err := os.ReadFile(filepath.Join(s.dir, "index.m3u8"))
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:") {
				n, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:")), 10, 64)
				if err == nil && n >= next {
					next = n + 1
				}
			}
		}
	}
	return next
}

func monitorHLSSession(channelID string, s *hlsSession) {
	ticker := time.NewTicker(hlsMonitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			playlistPath := filepath.Join(s.dir, "index.m3u8")
			info, statErr := os.Stat(playlistPath)
			now := time.Now()
			hlsMu.Lock()
			if hlsSessions[channelID] != s {
				hlsMu.Unlock()
				return
			}
			s.mu.Lock()
			if s.stopping {
				s.mu.Unlock()
				hlsMu.Unlock()
				return
			}
			if statErr == nil && info.Size() > 0 && info.ModTime().After(s.lastPlaylistMod) {
				s.lastPlaylistMod = info.ModTime()
				s.lastProgress = now
			}
			idle := s.activeReaders == 0 && now.Sub(s.lastAccess) >= hlsIdleTimeout
			stalled := !idle && now.Sub(s.started) >= hlsStartupGrace && now.Sub(s.lastProgress) >= hlsStallTimeout
			if idle || stalled {
				s.stopping = true
			}
			s.mu.Unlock()
			hlsMu.Unlock()
			if idle {
				log.Printf("HLS channel %s: idle timeout", channelID)
				killProcessGroup(s.cmd)
				return
			}
			if stalled {
				log.Printf("HLS channel %s: playlist stalled for %s; terminating FFmpeg for recovery", channelID, hlsStallTimeout)
				killProcessGroup(s.cmd)
				return
			}
		}
	}
}
