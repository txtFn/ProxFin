package src

import (
	"testing"
	"time"
)

func TestDirectHLSResourceDeduplication(t *testing.T) {
	channelA := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	channelB := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	upstream := "https://example.com/live/segment.ts"

	first, err := registerDirectHLSResource(channelA, upstream)
	if err != nil {
		t.Fatal(err)
	}

	second, err := registerDirectHLSResource(channelA, upstream)
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatal("identical channel and URL produced different IDs")
	}

	other, err := registerDirectHLSResource(channelB, upstream)
	if err != nil {
		t.Fatal(err)
	}

	if first == other {
		t.Fatal("different channels shared a resource ID")
	}

	directHLSResources.Lock()
	resource := directHLSResources.items[first]
	resource.expires = time.Now().Add(-time.Second)
	directHLSResources.items[first] = resource
	directHLSResources.Unlock()

	replacement, err := registerDirectHLSResource(channelA, upstream)
	if err != nil {
		t.Fatal(err)
	}

	if replacement == first {
		t.Fatal("expired resource ID was reused")
	}

	directHLSResources.Lock()
	delete(directHLSResources.items, first)
	delete(directHLSResources.items, second)
	delete(directHLSResources.items, other)
	delete(directHLSResources.items, replacement)
	directHLSResources.Unlock()
}
