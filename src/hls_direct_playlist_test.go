package src

import (
	"strings"
	"testing"
)

func TestDirectHLSPlaylistRewrite(t *testing.T) {
	channelID := "582239afa878ca1e6e99ca8f2b368f59"

	playlist := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",URI="audio/index.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=2500000,AUDIO="audio"
video/index.m3u8
`

	rewritten, err := rewriteDirectHLSPlaylist(
		channelID,
		"https://example.com/master.m3u8",
		playlist,
	)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(rewritten, `URI="audio/index.m3u8"`) {
		t.Fatal("Audio playlist URL was not rewritten")
	}

	if strings.Contains(rewritten, "video/index.m3u8") {
		t.Fatal("Video playlist URL was not rewritten")
	}

	if strings.Count(rewritten, "/hls-direct/"+channelID+"/r-") != 2 {
		t.Fatal("Expected two proxied resource URLs")
	}

	media := `#EXTM3U
#EXT-X-TARGETDURATION:6
#EXTINF:6.000,
segment001.ts
#EXT-X-DISCONTINUITY
#EXTINF:6.000,
segment002.ts
`

	rewrittenMedia, err := rewriteDirectHLSPlaylist(
		channelID,
		"https://example.com/video/index.m3u8",
		media,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(rewrittenMedia, "#EXT-X-DISCONTINUITY") {
		t.Fatal("Discontinuity marker was removed")
	}

	if strings.Contains(rewrittenMedia, "segment001.ts") ||
		strings.Contains(rewrittenMedia, "segment002.ts") {
		t.Fatal("Media segment URLs were not rewritten")
	}
}
