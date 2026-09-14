package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func MediaListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := DB.Query(`SELECT id, filename, thumbnail, duration FROM media ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	defer rows.Close()

	var items []map[string]string

	for rows.Next() {
		var id, filename, thumbnail, duration string
		rows.Scan(&id, &filename, &thumbnail, &duration)

		items = append(items, map[string]string{
			"id":        id,
			"filename":  filename,
			"thumbnail": thumbnail,
			"duration":  duration,
		})
	}

	json.NewEncoder(w).Encode(items)
}

func MediaDetailHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	row := DB.QueryRow(
		`
        SELECT id, filename, duration, width, height, video_codec, audio_codec
        FROM media WHERE id = ?
    	`, id)

	var m struct {
		ID         string  `json:"id"`
		Filename   string  `json:"filename"`
		Duration   float64 `json:"duration"`
		Width      int     `json:"width"`
		Height     int     `json:"height"`
		VideoCodec string  `json:"video_codec"`
		AudioCodec string  `json:"audio_codec"`
	}

	err := row.Scan(&m.ID, &m.Filename, &m.Duration, &m.Width, &m.Height, &m.VideoCodec, &m.AudioCodec)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}

	json.NewEncoder(w).Encode(m)
}
