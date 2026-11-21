package src

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	hlsStallTimeout    = 30 * time.Second
	hlsStartupGrace    = 45 * time.Second
	hlsIdleTimeout     = 5 * time.Minute
	hlsMonitorInterval = 5 * time.Second
)

var (
	hlsMu        sync.Mutex
	hlsSessions  = make(map[string]*hlsSession)
	hlsFileRE    = regexp.MustCompile(`^seg[0-9]{6,}\.ts$`)
	hlsChannelRE = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
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
		http.Error(w, "HLS session restarting", http.StatusServiceUnavailable)
		return
	}
	s.lastAccess = time.Now()
	s.activeReaders++
	s.readers.Add(1)
	s.mu.Unlock()
	hlsMu.Unlock()
	defer func() {
		s.mu.Lock()
		s.activeReaders--
		s.mu.Unlock()
		s.readers.Done()
	}()
	path := filepath.Join(s.dir, filename)
	if filename == "index.m3u8" {
		deadline := time.Now().Add(20 * time.Second)
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
			time.Sleep(250 * time.Millisecond)
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
		"-hls_flags", "delete_segments+omit_endlist",
		"-hls_segment_filename", filepath.Join(dir, "seg%06d.ts"),
		filepath.Join(dir, "index.m3u8"),
	)
	cmd := exec.Command(path, args...)
	setProcessGroup(cmd)
	cmd.Stdout = nil
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
			delete(hlsSessions, channelID)
		}
		hlsMu.Unlock()
		close(s.done)
		s.readers.Wait()
		os.RemoveAll(dir)
	}()
	return s, nil
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
				// The next playlist request starts a new session after the old process exits.
				// A short 503 during this transition is expected.
				return
			}
		}
	}
}
