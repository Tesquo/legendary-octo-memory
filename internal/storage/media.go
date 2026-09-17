package storage

import (
	"database/sql"
)

// Media is a single library item. JSON tags match the frontend client.
type Media struct {
	ID           string  `json:"id"`
	Filename     string  `json:"filename"`
	Thumbnail    string  `json:"thumbnail"`
	Duration     float64 `json:"duration"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	VideoCodec   string  `json:"video_codec"`
	AudioCodec   string  `json:"audio_codec"`
	Status       string  `json:"status"`
	PlayablePath string  `json:"playable_path"`
}

// Status values stored in the media.status column.
const (
	StatusImported   = "imported"   // stored, not yet prepared
	StatusProcessing = "processing" // ffmpeg job running
	StatusReady      = "ready"      // playable rendition available
	StatusFailed     = "failed"
)

// MediaStore wraps the database with typed helpers for the media table.
type MediaStore struct {
	db *sql.DB
}

func NewMediaStore(db *sql.DB) *MediaStore {
	return &MediaStore{db: db}
}

func (s *MediaStore) Insert(m *Media) error {
	_, err := s.db.Exec(`
		INSERT INTO media
			(id, filename, thumbnail, duration, width, height, video_codec, audio_codec, status, playable_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		m.ID, m.Filename, m.Thumbnail, m.Duration, m.Width, m.Height,
		m.VideoCodec, m.AudioCodec, m.Status, m.PlayablePath,
	)
	return err
}

func (s *MediaStore) List() ([]Media, error) {
	rows, err := s.db.Query(`
		SELECT id, filename, thumbnail, duration, width, height,
		       video_codec, audio_codec, status, playable_path
		FROM media
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Media{}
	for rows.Next() {
		var m Media
		if err := rows.Scan(
			&m.ID, &m.Filename, &m.Thumbnail, &m.Duration, &m.Width, &m.Height,
			&m.VideoCodec, &m.AudioCodec, &m.Status, &m.PlayablePath,
		); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

func (s *MediaStore) Get(id string) (*Media, error) {
	var m Media
	err := s.db.QueryRow(`
		SELECT id, filename, thumbnail, duration, width, height,
		       video_codec, audio_codec, status, playable_path
		FROM media WHERE id = ?
	`, id).Scan(
		&m.ID, &m.Filename, &m.Thumbnail, &m.Duration, &m.Width, &m.Height,
		&m.VideoCodec, &m.AudioCodec, &m.Status, &m.PlayablePath,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *MediaStore) SetStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE media SET status = ? WHERE id = ?`, status, id)
	return err
}

func (s *MediaStore) SetPlayable(id, playablePath, status string) error {
	_, err := s.db.Exec(`
		UPDATE media SET playable_path = ?, status = ? WHERE id = ?
	`, playablePath, status, id)
	return err
}

func (s *MediaStore) SetThumbnail(id, thumbnail string) error {
	_, err := s.db.Exec(`UPDATE media SET thumbnail = ? WHERE id = ?`, thumbnail, id)
	return err
}

func (s *MediaStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM media WHERE id = ?`, id)
	return err
}
