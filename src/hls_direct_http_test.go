package src

import "testing"

func TestDirectHLSURLValidation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		allowed bool
	}{
		{"HTTPS", "https://example.com/live.m3u8", true},
		{"HTTP", "http://example.com/live.m3u8", true},
		{"custom port 826", "http://example.com:826/live.m3u8", true},
		{"custom HTTPS port", "https://example.com:8443/live.m3u8", true},
		{"FTP", "ftp://example.com/live.m3u8", false},
		{"missing hostname", "https:///live.m3u8", false},
		{"embedded credentials", "https://user:pass@example.com/live.m3u8", false},
		{"invalid port", "http://example.com:abc/live.m3u8", false},
		{"port too large", "http://example.com:99999/live.m3u8", false},
		{"zero port", "http://example.com:0/live.m3u8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDirectHLSURL(tt.url)

			if tt.allowed && err != nil {
				t.Fatalf("expected valid URL, got %v", err)
			}

			if !tt.allowed && err == nil {
				t.Fatalf("expected URL rejection: %s", tt.url)
			}
		})
	}
}
