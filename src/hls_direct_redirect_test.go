package src

import (
	"net/http"
	"testing"
)

func TestDirectHLSRedirectValidation(t *testing.T) {
	client := newDirectHLSClient()

	tests := []struct {
		name    string
		url     string
		allowed bool
	}{
		{"HTTPS CDN", "https://cdn.example.com/live.m3u8", true},
		{"HTTP port 826", "http://cdn.example.com:826/live.m3u8", true},
		{"FTP redirect", "ftp://example.com/live.m3u8", false},
		{"embedded credentials", "https://user:pass@example.com/live.m3u8", false},
		{"invalid port", "https://example.com:99999/live.m3u8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tt.url, nil)

			if err != nil {
				if tt.allowed {
					t.Fatal(err)
				}
				return
			}

			err = client.CheckRedirect(req, nil)

			if tt.allowed && err != nil {
				t.Fatalf("expected redirect allowed, got %v", err)
			}

			if !tt.allowed && err == nil {
				t.Fatalf("expected redirect rejected: %s", tt.url)
			}
		})
	}
}
