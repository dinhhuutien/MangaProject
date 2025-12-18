package database

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	// SQLite thường nên set 1 connection cho đơn giản demo
	db.SetMaxOpenConns(1)
	return db, nil
}
