
package src

import (
	"os"
	"path/filepath"
	"testing"
)

func TestXMLTVStreamRecovery(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantError    bool
		wantChannels int
		wantPrograms int
	}{
		{
			name: "valid XMLTV",
			content: `<?xml version="1.0"?>
<tv>
  <channel id="test">
    <display-name>Test Channel</display-name>
  </channel>
  <programme channel="test"
    start="20261125080000 +0000"
    stop="20261125090000 +0000">
    <title>Test Programme</title>
  </programme>
</tv>`,
			wantChannels: 1,
			wantPrograms: 1,
		},
		{
			name:      "valid empty guide",
			content:   `<tv></tv>`,
			wantError: false,
		},
		{
			name:      "malformed channel",
			content:   `<tv><channel id="test"><display-name>Test</channel></tv>`,
			wantError: true,
		},
		{
			name:      "malformed programme",
			content:   `<tv><programme channel="test"><title>Test</programme></tv>`,
			wantError: true,
		},
		{
			name:      "truncated XMLTV",
			content:   `<tv><channel id="test"></channel>`,
			wantError: true,
		},
		{
			name:      "invalid XML token",
			content:   `<tv><channel id="test"></channel><`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "guide.xml")

			if err := os.WriteFile(
				file,
				[]byte(tt.content),
				0600,
			); err != nil {
				t.Fatal(err)
			}

			var guide XMLTV
			err := parseXMLTVStream(file, &guide)

			if tt.wantError {
				if err == nil {
					t.Fatal("expected parsing error")
				}

				t.Logf("Expected error received: %v", err)
				return
			}

			if err != nil {
				t.Fatalf("unexpected parsing error: %v", err)
			}

			if len(guide.Channel) != tt.wantChannels {
				t.Errorf(
					"channels: got %d, want %d",
					len(guide.Channel),
					tt.wantChannels,
				)
			}

			if len(guide.Program) != tt.wantPrograms {
				t.Errorf(
					"programmes: got %d, want %d",
					len(guide.Program),
					tt.wantPrograms,
				)
			}
		})
	}
}

func TestXMLTVStreamMissingFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "missing.xml")

	var guide XMLTV

	if err := parseXMLTVStream(file, &guide); err == nil {
		t.Fatal("expected missing file error")
	}
}
