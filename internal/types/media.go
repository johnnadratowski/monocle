package types

import (
	"path/filepath"
	"strings"
)

// mediaMIMEByExt maps lowercase file extensions to MIME types for media that
// Monocle can accept as reviewable artifacts (images, video, audio). It is
// explicit (rather than relying on the stdlib mime package) so detection is
// deterministic across platforms and testable.
var mediaMIMEByExt = map[string]string{
	// images
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".bmp": "image/bmp",
	".svg": "image/svg+xml", ".tiff": "image/tiff", ".tif": "image/tiff",
	".heic": "image/heic", ".heif": "image/heif", ".ico": "image/x-icon",
	".avif": "image/avif",
	// video
	".mp4": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm",
	".mkv": "video/x-matroska", ".avi": "video/x-msvideo", ".m4v": "video/x-m4v",
	".mpg": "video/mpeg", ".mpeg": "video/mpeg",
	// audio
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg",
	".flac": "audio/flac", ".m4a": "audio/mp4", ".aac": "audio/aac",
	".opus": "audio/opus",
}

// MediaInfo returns the media category ("image"/"video"/"audio"), the MIME type,
// and ok=true when path has a recognized media extension.
func MediaInfo(path string) (category, mimeType string, ok bool) {
	mt, found := mediaMIMEByExt[strings.ToLower(filepath.Ext(path))]
	if !found {
		return "", "", false
	}
	if i := strings.IndexByte(mt, '/'); i > 0 {
		category = mt[:i]
	}
	return category, mt, true
}

// IsMediaFile reports whether path looks like a media file Monocle can accept as
// an artifact.
func IsMediaFile(path string) bool {
	_, _, ok := MediaInfo(path)
	return ok
}
