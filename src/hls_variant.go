
package src

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const foreverKidsURL = "https://jmp2.uk:443/plu-56171fafada51f8004c4b40f.m3u8"
const foreverKidsBandwidth = "3321280"

func resolveForeverKidsHLS(originalURL string) (string, error) {
	if originalURL != foreverKidsURL {
		return originalURL, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, originalURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch Forever Kids master: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Forever Kids master returned HTTP %d", resp.StatusCode)
	}

	// Limit the amount of data read from the remote playlist.
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 2*1024*1024))
	scanner.Buffer(make([]byte, 4096), 1024*1024)

	masterURL := resp.Request.URL
	selectNext := false
	foundMaster := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "#EXTM3U" {
			foundMaster = true
		}

		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			selectNext = false
			attributes := strings.Split(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"), ",")
			for _, attribute := range attributes {
				if strings.TrimSpace(attribute) == "BANDWIDTH="+foreverKidsBandwidth {
					selectNext = true
					break
				}
			}
			continue
		}

		if selectNext && line != "" && !strings.HasPrefix(line, "#") {
			variant, err := url.Parse(line)
			if err != nil {
				return "", err
			}
			if !foundMaster {
				return "", fmt.Errorf("invalid HLS master playlist")
			}
			resolved := masterURL.ResolveReference(variant)
			if resolved.Scheme != "https" && resolved.Scheme != "http" {
				return "", fmt.Errorf("unsupported HLS variant URL scheme")
			}
			return resolved.String(), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read Forever Kids master: %w", err)
	}

	return "", fmt.Errorf("Forever Kids bandwidth %s not found", foreverKidsBandwidth)
}
