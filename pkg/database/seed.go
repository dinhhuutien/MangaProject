package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"mangahub/pkg/models"
)

func LoadMangaFromJSON(jsonPath string) ([]models.Manga, error) {
	b, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read manga json: %w", err)
	}

	var list []models.Manga
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("unmarshal manga json: %w", err)
	}

	return list, nil
}

func SeedManga(db *sql.DB, mangaList []models.Manga) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES (?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare insert manga: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, m := range mangaList {
		genresJSON, err := json.Marshal(m.Genres)
		if err != nil {
			return 0, fmt.Errorf("marshal genres for %s: %w", m.ID, err)
		}

		res, err := stmt.Exec(m.ID, m.Title, m.Author, string(genresJSON), m.Status, m.TotalChapters, m.Description)
		if err != nil {
			return 0, fmt.Errorf("insert manga %s: %w", m.ID, err)
		}

		aff, _ := res.RowsAffected()
		if aff > 0 {
			inserted++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return inserted, nil
}
