package api

import (
	"path"
	"path/filepath"
	"strings"
)

// mediaDir returns the on-disk directory for a media item.
func mediaDir(id string) string {
	return filepath.Join(MediaDir, id)
}

// sourcePath returns the on-disk path of the imported source file.
func sourcePath(id, filename string) string {
	return filepath.Join(mediaDir(id), filename)
}

// playablePath returns the on-disk path of the browser-playable rendition.
func playablePath(id string) string {
	return filepath.Join(mediaDir(id), "playable.mp4")
}

// thumbDir returns the on-disk directory for thumbnails.
func thumbDir(id string) string {
	return filepath.Join(mediaDir(id), "thumbs")
}

// thumbPath returns the on-disk path of the generated thumbnail.
func thumbPath(id string) string {
	return filepath.Join(thumbDir(id), "thumb.jpg")
}

// relPath converts an on-disk path into a forward-slash URL path served by the
// static media route, so the frontend can load it directly. It deliberately
// joins against MediaURLPrefix (the router's mount path) rather than MediaDir
// (the disk path), since a thumbnail's on-disk location and its URL differ.
func relPath(p string) string {
	if p == "" {
		return ""
	}
	trimmed := strings.TrimPrefix(p, MediaDir+string(filepath.Separator))
	return path.Join(MediaURLPrefix, filepath.ToSlash(trimmed))
}
