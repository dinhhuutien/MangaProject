package main

import (
	"database/sql"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/yourname/mangahub/pkg/database"
)

func main() {
	// Initialize database
	db, err := database.InitDB("./data/mangahub.db")
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Setup Gin router
	router := gin.Default()

	// Test endpoint
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// API routes
	api := router.Group("/api/v1")
	{
		// Auth routes
		auth := api.Group("/auth")
		{
			auth.POST("/register", registerHandler(db))
			auth.POST("/login", loginHandler(db))
		}

		// Manga routes
		manga := api.Group("/manga")
		{
			manga.GET("/search", searchMangaHandler(db))
			manga.GET("/:id", getMangaHandler(db))
		}
	}

	// Start server
	log.Println("Server starting on :8080")
	router.Run(":8080")
}

// Placeholder handlers - bạn sẽ implement sau
func registerHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Register endpoint - to be implemented"})
	}
}

func loginHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Login endpoint - to be implemented"})
	}
}

func searchMangaHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Search endpoint - to be implemented"})
	}
}

func getMangaHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Get manga endpoint - to be implemented"})
	}
}
