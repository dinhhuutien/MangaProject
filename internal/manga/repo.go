package manga

import "database/sql"

type Manga struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Author        string `json:"author"`
	Genres        string `json:"genres"` // JSON array as text theo spec :contentReference[oaicite:10]{index=10}
	Status        string `json:"status"`
	TotalChapters int    `json:"total_chapters"`
	Description   string `json:"description"`
}

func Search(db *sql.DB, q, genre, status string, limit, offset int) ([]Manga, error) {
	sqlQ := `SELECT id,title,author,genres,status,total_chapters,description
	         FROM manga WHERE 1=1`
	args := []any{}

	if q != "" {
		sqlQ += " AND (title LIKE ? OR author LIKE ?)"
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	if status != "" {
		sqlQ += " AND status = ?"
		args = append(args, status)
	}
	if genre != "" {
		// genres lưu dạng JSON text => đơn giản demo: LIKE
		sqlQ += " AND genres LIKE ?"
		args = append(args, "%"+genre+"%")
	}
	sqlQ += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := db.Query(sqlQ, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []Manga
	for rows.Next() {
		var m Manga
		if err := rows.Scan(&m.ID, &m.Title, &m.Author, &m.Genres, &m.Status, &m.TotalChapters, &m.Description); err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	return res, rows.Err()
}

func GetByID(db *sql.DB, id string) (Manga, error) {
	var m Manga
	err := db.QueryRow(`SELECT id,title,author,genres,status,total_chapters,description FROM manga WHERE id = ?`, id).
		Scan(&m.ID, &m.Title, &m.Author, &m.Genres, &m.Status, &m.TotalChapters, &m.Description)
	return m, err
}
