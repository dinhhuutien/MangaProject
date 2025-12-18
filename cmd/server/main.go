package main

import (
	"fmt"
	"log"

	"mangahub/pkg/database"
)

func main() {
	dbPath := "./mangahub.db" // Day 1 dùng file local cho dễ
	db, err := database.OpenSQLite(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatal(err)
	}

	mangaList, err := database.LoadMangaFromJSON("./data/manga.json")
	if err != nil {
		log.Fatal(err)
	}

	n, err := database.SeedManga(db, mangaList)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("DB ready: %s | seeded: %d manga\n", dbPath, n)
}
