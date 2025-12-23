package library

import "database/sql"

type Progress struct {
	UserID         string `json:"user_id"`
	MangaID        string `json:"manga_id"`
	CurrentChapter int    `json:"current_chapter"`
	Status         string `json:"status"`
	ListName       string `json:"list_name"` // Bonus: Multiple reading lists support
}

// Bonus: UpsertProgress now supports list_name for multiple reading lists
func UpsertProgress(db *sql.DB, p Progress) error {
	// Default to empty string if list_name not provided (backward compatible)
	listName := p.ListName
	if listName == "" {
		listName = "default"
	}
	_, err := db.Exec(`
	INSERT INTO user_progress(user_id, manga_id, current_chapter, status, list_name)
	VALUES(?,?,?,?,?)
	ON CONFLICT(user_id, manga_id)
	DO UPDATE SET current_chapter=excluded.current_chapter,
	              status=excluded.status,
	              list_name=excluded.list_name,
	              updated_at=CURRENT_TIMESTAMP
	`, p.UserID, p.MangaID, p.CurrentChapter, p.Status, listName)
	return err
}

func GetProgress(db *sql.DB, userID, mangaID string) (Progress, error) {
	var p Progress
	// Bonus: Include list_name in SELECT (handle case where column might not exist with COALESCE)
	err := db.QueryRow(`SELECT user_id,manga_id,current_chapter,status,COALESCE(list_name, 'default') FROM user_progress WHERE user_id=? AND manga_id=?`,
		userID, mangaID).Scan(&p.UserID, &p.MangaID, &p.CurrentChapter, &p.Status, &p.ListName)
	return p, err
}

// Bonus: Get progress by list name
func GetProgressByList(db *sql.DB, userID, listName string) ([]Progress, error) {
	rows, err := db.Query(`SELECT user_id,manga_id,current_chapter,status,COALESCE(list_name, 'default') FROM user_progress WHERE user_id=? AND list_name=?`,
		userID, listName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Progress
	for rows.Next() {
		var p Progress
		if err := rows.Scan(&p.UserID, &p.MangaID, &p.CurrentChapter, &p.Status, &p.ListName); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}
