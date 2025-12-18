package user

import (
	"database/sql"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           string
	Username     string
	PasswordHash string
}

func CreateUser(db *sql.DB, id, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO users(id, username, password_hash) VALUES(?,?,?)`, id, username, string(hash))
	return err
}

func VerifyLogin(db *sql.DB, username, password string) (User, error) {
	var u User
	err := db.QueryRow(`SELECT id, username, password_hash FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash)
	if err != nil {
		return User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return User{}, errors.New("invalid credentials")
	}
	return u, nil
}
