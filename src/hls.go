package src

import (
"fmt"
"net/http"
"os"
"os/exec"
"path/filepath"
"regexp"
"strings"
"sync"
"time"
)

const hlsTestChannel = "3384e497f9ab3e7cda1dc1fb972f07c4"

var (
hlsMu      sync.Mutex
hlsCurrent *hlsSession
hlsFileRE  = regexp.MustCompile(`^seg[0-9]{6,}.ts$`)
)

type hlsSession struct {
dir      string
cmd      *exec.Cmd
done     chan struct{}
mu       sync.Mutex
readers  sync.WaitGroup
stopping bool
}

func HLSHandler(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodGet && r.Method != http.MethodHead {
http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
return
}

parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/hls/"), "/")
if len(parts) != 2 || parts[0] != hlsTestChannel {
http.NotFound(w, r)
return
}

filename := parts[1]
if filename != "index.m3u8" && !hlsFileRE.MatchString(filename) {
http.NotFound(w, r)
return
}

hlsMu.Lock()
s := hlsCurrent

if s == nil {
if filename != "index.m3u8" {
hlsMu.Unlock()
http.NotFound(w, r)
return
}

var err error
s, err = startHLSSession()
if err != nil {
hlsMu.Unlock()
http.Error(w, "Unable to start HLS stream", http.StatusBadGateway)
return
}
hlsCurrent = s
}

s.mu.Lock()
if s.stopping {
s.mu.Unlock()
hlsMu.Unlock()
http.Error(w, "HLS session restarting", http.StatusServiceUnavailable)
return
}

s.readers.Add(1)
s.mu.Unlock()
hlsMu.Unlock()
defer s.readers.Done()

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

w.Header().Set("Cache-Control", "no-store")
w.Header().Set("Access-Control-Allow-Origin", "*")
http.ServeFile(w, r, path)
}

func startHLSSession() (*hlsSession, error) {
info, err := getStreamInfo(hlsTestChannel)
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
args = append(args, "-http_proxy",
fmt.Sprintf("http://%s:%s", proxyIP, proxyPort))
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
cmd.Stderr = nil

if err := cmd.Start(); err != nil {
os.RemoveAll(dir)
return nil, err
}

s := &hlsSession{
dir:  dir,
cmd:  cmd,
done: make(chan struct{}),
}

go func() {
cmd.Wait()

// Mark the session as stopping before clearing it.
hlsMu.Lock()
s.mu.Lock()
s.stopping = true
s.mu.Unlock()
if hlsCurrent == s {
hlsCurrent = nil
}
hlsMu.Unlock()

close(s.done)

// Existing HTTP requests must finish before deletion.
s.readers.Wait()
os.RemoveAll(dir)
}()

return s, nil
}