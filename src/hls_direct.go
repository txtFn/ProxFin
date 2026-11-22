package src

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type directHLSResource struct {
	channelID string
	url       string
	expires   time.Time
}

var directHLSResources = struct {
	sync.Mutex
	items map[string]directHLSResource
}{
	items: make(map[string]directHLSResource),
}

func registerDirectHLSResource(channelID, upstreamURL string) (string, error) {
	directHLSResources.Lock()
	defer directHLSResources.Unlock()

	now := time.Now()

	for id, resource := range directHLSResources.items {
		if !now.Before(resource.expires) {
			delete(directHLSResources.items, id)
			continue
		}

		if resource.channelID == channelID && resource.url == upstreamURL {
			resource.expires = now.Add(10 * time.Minute)
			directHLSResources.items[id] = resource
			return id, nil
		}
	}

	if len(directHLSResources.items) >= 50000 {
		return "", errors.New("direct HLS resource registry is full")
	}

	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}

	id := hex.EncodeToString(random[:])

	directHLSResources.items[id] = directHLSResource{
		channelID: channelID,
		url:       upstreamURL,
		expires:   now.Add(10 * time.Minute),
	}

	return id, nil
}
func lookupDirectHLSResource(channelID, id string) (string, bool) {
    directHLSResources.Lock()
    defer directHLSResources.Unlock()

    resource, ok := directHLSResources.items[id]
    if !ok || resource.channelID != channelID ||
        !time.Now().Before(resource.expires) {
        return "", false
    }

    resource.expires = time.Now().Add(10 * time.Minute)
    directHLSResources.items[id] = resource

    return resource.url, true
}

func HLSDirectHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/hls-direct/"), "/")
	if len(parts) != 2 || !hlsChannelRE.MatchString(parts[0]) {
		http.NotFound(w, r)
		return
	}

	channelID := parts[0]
	resourceName := parts[1]

	if resourceName == "index.m3u8" {
		info, err := getStreamInfo(channelID)
		if err != nil {
			http.Error(w, "Channel not found", http.StatusNotFound)
			return
		}

		u, err := url.Parse(info.URL)
		if err != nil || u.Hostname() == "" ||
			(u.Scheme != "http" && u.Scheme != "https") {
			http.Error(w, "Unsupported stream URL", http.StatusBadRequest)
			return
		}

		serveDirectHLSUpstream(w, r, channelID, info.URL)
		return
	}

	if !strings.HasPrefix(resourceName, "r-") {
		http.NotFound(w, r)
		return
	}

	id := strings.TrimPrefix(resourceName, "r-")
	if len(id) != 32 {
		http.NotFound(w, r)
		return
	}

	upstreamURL, ok := lookupDirectHLSResource(channelID, id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	serveDirectHLSUpstream(w, r, channelID, upstreamURL)
}
