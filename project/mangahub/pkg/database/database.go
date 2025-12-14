package database

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB(filepath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", filepath)
	if err != nil {
		return nil, err
	}

	// Test connection
	if err = db.Ping(); err != nil {
		return nil, err
	}

	// Create tables
	if err = createTables(db); err != nil {
		return nil, err
	}

	return db, nil
}

func createTables(db *sql.DB) error {
	schema := `
    CREATE TABLE IF NOT EXISTS users (
        id TEXT PRIMARY KEY,
        username TEXT UNIQUE NOT NULL,
        email TEXT UNIQUE NOT NULL,
        password_hash TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );

    CREATE TABLE IF NOT EXISTS manga (
        id TEXT PRIMARY KEY,
        title TEXT NOT NULL,
        author TEXT,
        genres TEXT,
        status TEXT,
        total_chapters INTEGER,
        description TEXT
    );

    CREATE TABLE IF NOT EXISTS user_progress (
        user_id TEXT,
        manga_id TEXT,
        current_chapter INTEGER DEFAULT 0,
        status TEXT DEFAULT 'plan-to-read',
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        PRIMARY KEY (user_id, manga_id),
        FOREIGN KEY (user_id) REFERENCES users(id),
        FOREIGN KEY (manga_id) REFERENCES manga(id)
    );
    `

	_, err := db.Exec(schema)
	return err
}
