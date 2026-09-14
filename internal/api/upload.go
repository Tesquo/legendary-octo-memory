package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Tesquo/legendary-octo-memory/internal/processing"
	"github.com/google/uuid"
)

func UploadHandler(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file upload error", http.StatusBadRequest)
		return
	}
	defer file.Close()

	id := uuid.New().String()

	mediaDir := filepath.Join("media", id)
	os.MkdirAll(mediaDir, 0755)

	thumbDir := filepath.Join(mediaDir, "thumbs")
	os.MkdirAll(thumbDir, 0755)

	dstPath := filepath.Join(mediaDir, header.Filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "cannot save file", http.StatusInternalServerError)
		return
	}

	go func() {
		_, err := processing.GenerateThumbnail(dstPath, thumbDir)
		if err != nil {
			fmt.Println("thumbnail error:", err)
			return
		}

		// update DB with thumbnail path
		DB.Exec(`
			UPDATE media SET thumbnail = ? WHERE id = ?
		`, filepath.Join("media", id, "thumbs", "thumb.jpg"), id)
	}()

	defer dst.Close()

	io.Copy(dst, file)

	json.NewEncoder(w).Encode(map[string]string{
		"id":       id,
		"filename": header.Filename,
		"status":   "uploaded",
	})

	md, err := processing.ExtractMetadata(dstPath)
	if err != nil {
		http.Error(w, "metadata error", 500)
		return
	}

	_, err = DB.Exec(`
		INSERT INTO media (id, filename, duration, width, height, video_codec, audio_codec)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, header.Filename, md.Duration, md.Width, md.Height, md.VideoCodec, md.AudioCodec)

}
