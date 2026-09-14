package storage

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "media.db")
	if err != nil {
		return nil, err
	}

	schema := `
    CREATE TABLE IF NOT EXISTS media (
        id TEXT PRIMARY KEY,
        filename TEXT,
        thumbnail TEXT,
        duration REAL,
        width INTEGER,
        height INTEGER,
        video_codec TEXT,
        audio_codec TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    `
	_, err = db.Exec(schema)
	return db, err
}
