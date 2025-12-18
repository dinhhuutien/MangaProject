package library

import "database/sql"

type Progress struct {
	UserID         string `json:"user_id"`
	MangaID        string `json:"manga_id"`
	CurrentChapter int    `json:"current_chapter"`
	Status         string `json:"status"`
}

func UpsertProgress(db *sql.DB, p Progress) error {
	_, err := db.Exec(`
	INSERT INTO user_progress(user_id, manga_id, current_chapter, status)
	VALUES(?,?,?,?)
	ON CONFLICT(user_id, manga_id)
	DO UPDATE SET current_chapter=excluded.current_chapter,
	              status=excluded.status,
	              updated_at=CURRENT_TIMESTAMP
	`, p.UserID, p.MangaID, p.CurrentChapter, p.Status)
	return err
}

func GetProgress(db *sql.DB, userID, mangaID string) (Progress, error) {
	var p Progress
	err := db.QueryRow(`SELECT user_id,manga_id,current_chapter,status FROM user_progress WHERE user_id=? AND manga_id=?`,
		userID, mangaID).Scan(&p.UserID, &p.MangaID, &p.CurrentChapter, &p.Status)
	return p, err
}
