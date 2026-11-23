package src

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const directHLSMaxPlaylistSize = 8 * 1024 * 1024

func serveDirectHLSUpstream(
	w http.ResponseWriter,
	r *http.Request,
	channelID string,
	upstreamURL string,
) {
	serveDirectHLSUpstreamWithClient(
		w, r, channelID, upstreamURL, newDirectHLSClient(),
	)
}

func serveDirectHLSUpstreamWithClient(
	w http.ResponseWriter,
	r *http.Request,
	channelID string,
	upstreamURL string,
	client *http.Client,
) {
	if err := validateDirectHLSURL(upstreamURL); err != nil {
		http.Error(w, "Invalid upstream URL", http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(
		r.Context(),
		http.MethodGet,
		upstreamURL,
		nil,
	)
	if err != nil {
		http.Error(w, "Unable to prepare upstream request", http.StatusBadGateway)
		return
	}

	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Unable to fetch upstream stream", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Upstream stream unavailable", http.StatusBadGateway)
		return
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	path := strings.ToLower(resp.Request.URL.Path)

	isPlaylist := strings.HasSuffix(path, ".m3u8") ||
		strings.Contains(contentType, "mpegurl")

	if isPlaylist {
		content, err := io.ReadAll(
			io.LimitReader(resp.Body, directHLSMaxPlaylistSize+1),
		)
		if err != nil || len(content) > directHLSMaxPlaylistSize {
			http.Error(w, "Unable to read upstream playlist", http.StatusBadGateway)
			return
		}

		if !bytes.HasPrefix(
			bytes.TrimSpace(content),
			[]byte("#EXTM3U"),
		) {
			http.Error(w, "Invalid upstream playlist", http.StatusBadGateway)
			return
		}

		rewritten, err := rewriteDirectHLSPlaylist(
			channelID,
			resp.Request.URL.String(),
			string(content),
		)
		if err != nil {
			http.Error(w, "Unable to rewrite upstream playlist", http.StatusBadGateway)
			return
		}

		w.Header().Set(
			"Content-Type",
			"application/vnd.apple.mpegurl",
		)
		w.Header().Set("Cache-Control", "no-store")

		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}

		_, _ = io.WriteString(w, rewritten)
		return
	}

	if resp.ContentLength > 128*1024*1024 {
		http.Error(w, "Upstream resource too large", http.StatusBadGateway)
		return
	}

	if value := resp.Header.Get("Content-Type"); value != "" {
		w.Header().Set("Content-Type", value)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	if resp.ContentLength >= 0 && resp.ContentLength <= directHLSMaxMediaSize {
		w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if resp.ContentLength > directHLSRecoveryMaxMediaSize {
		// Preserve streaming for large resources without buffering
		// their entire contents in Railway memory.
		w.WriteHeader(http.StatusOK)

		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		if err := copyDirectHLSMediaWithLength(
			w,
			resp.Body,
			resp.ContentLength,
		); err != nil {
			controller := http.NewResponseController(w)
			_ = controller.SetWriteDeadline(time.Now().Add(-time.Second))
		}
		return
	}
	data, recoveredType, err := fetchDirectHLSMediaFromResponse(
		r.Context(),
		client,
		upstreamURL,
		resp,
	)
	if err != nil {
		w.Header().Del("Content-Length")
		http.Error(w, "Unable to recover upstream media", http.StatusBadGateway)
		return
	}

	if recoveredType != "" {
		w.Header().Set("Content-Type", recoveredType)
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
