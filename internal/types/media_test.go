package types

import "testing"

func TestMediaInfo(t *testing.T) {
	cases := []struct {
		path         string
		wantCategory string
		wantOK       bool
	}{
		{"logo.png", "image", true},
		{"photo.JPG", "image", true}, // case-insensitive
		{"clip.mp4", "video", true},
		{"song.mp3", "audio", true},
		{"README.md", "", false},
		{"main.go", "", false},
		{"noext", "", false},
	}
	for _, c := range cases {
		cat, mime, ok := MediaInfo(c.path)
		if ok != c.wantOK || cat != c.wantCategory {
			t.Errorf("MediaInfo(%q) = (%q, %q, %v), want category %q ok %v", c.path, cat, mime, ok, c.wantCategory, c.wantOK)
		}
		if ok && mime == "" {
			t.Errorf("MediaInfo(%q): ok but empty mime", c.path)
		}
	}
}

func TestIsMediaFile(t *testing.T) {
	if !IsMediaFile("a/b/c.PNG") {
		t.Error("expected .PNG to be media")
	}
	if IsMediaFile("plan.md") {
		t.Error("expected .md not to be media")
	}
}

func TestContentItemIsMedia(t *testing.T) {
	if (ContentItem{}).IsMedia() {
		t.Error("empty item should not be media")
	}
	if !(ContentItem{MediaPath: "/x.png"}).IsMedia() {
		t.Error("item with MediaPath should be media")
	}
}
