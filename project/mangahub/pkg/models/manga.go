package models

type Manga struct {
	ID            string `json:"id" db:"id"`
	Title         string `json:"title" db:"title"`
	Author        string `json:"author" db:"author"`
	Genres        string `json:"genres" db:"genres"` // JSON array as text
	Status        string `json:"status" db:"status"`
	TotalChapters int    `json:"total_chapters" db:"total_chapters"`
	Description   string `json:"description" db:"description"`
}

type UserProgress struct {
	UserID         string `json:"user_id" db:"user_id"`
	MangaID        string `json:"manga_id" db:"manga_id"`
	CurrentChapter int    `json:"current_chapter" db:"current_chapter"`
	Status         string `json:"status" db:"status"` // reading, completed, plan-to-read
	UpdatedAt      string `json:"updated_at" db:"updated_at"`
}
