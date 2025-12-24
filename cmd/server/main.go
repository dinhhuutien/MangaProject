package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"mangahub/internal/auth"
	grpcserver "mangahub/internal/grpc"
	"mangahub/internal/library"
	"mangahub/internal/manga"
	"mangahub/internal/tcpsync"
	"mangahub/internal/udpnotify"
	"mangahub/internal/user"
	"mangahub/internal/websocket"
	"mangahub/pkg/database"
	"mangahub/pkg/models"
	"mangahub/proto"
)

var jwtSecret = []byte("dev-secret-change-me")

func main() {
	// Dùng 1 DB cố định trong /data để tránh lệch working directory
	dbPath := "./data/mangahub.db"

	// Ensure data folder exists
	if err := os.MkdirAll("./data", 0755); err != nil {
		log.Fatal(err)
	}

	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// ✅ FIX: tạo schema trước khi chạy API
	if err := database.Migrate(db); err != nil {
		log.Fatal(err)
	}

	// ✅ Seed manga nếu có file JSON (Day 1)
	if _, err := os.Stat("./data/manga.json"); err == nil {
		mangaList, err := database.LoadMangaFromJSON("./data/manga.json")
		if err != nil {
			log.Fatal(err)
		}
		n, err := database.SeedManga(db, mangaList)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Seeded %d manga into %s", n, dbPath)
	} else {
		log.Printf("warn: data/manga.json not found; skip seeding (%v)", err)
	}

	r := gin.Default()
	//web

	// Serve UI entry
	r.GET("/ui", func(c *gin.Context) {
		c.File("./web/index.html")
	})

	// Serve static files
	r.Static("/ui", "./web")

	progressCh := make(chan models.ProgressUpdate, 100)

	// TCP server
	tcpServer := tcpsync.New(":9090", progressCh)
	go func() {
		if err := tcpServer.Start(); err != nil {
			log.Fatal(err)
		}
	}()

	// UDP server
	udpServer := udpnotify.New(":7070")
	go func() {
		if err := udpServer.Start(); err != nil {
			log.Fatal(err)
		}
	}()
	log.Println("UDP server listening on :7070")

	// gRPC server
	grpcServer := grpc.NewServer()
	grpcService := grpcserver.NewServer(db)
	proto.RegisterMangaServiceServer(grpcServer, grpcService)
	reflection.Register(grpcServer)
	go func() {
		lis, err := net.Listen("tcp", ":50051")
		if err != nil {
			log.Fatal("Failed to listen for gRPC:", err)
		}
		log.Println("gRPC server listening on :50051")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal("Failed to serve gRPC:", err)
		}
	}()

	// Chat hub
	chatHub := websocket.NewHub()
	go chatHub.Run()
	log.Println("Chat hub started")

	//ROUTES
	r.GET("/health", func(c *gin.Context) { handleHealthCheck(c, db, tcpServer, udpServer, grpcServer, chatHub) })

	// AUTH
	r.POST("/auth/register", func(c *gin.Context) { handleRegister(c, db) })
	r.POST("/auth/login", func(c *gin.Context) { handleLogin(c, db) })

	// PUBLIC MANGA
	r.GET("/manga", func(c *gin.Context) { handleSearchManga(c, db) })
	r.GET("/manga/:id", func(c *gin.Context) { handleMangaDetail(c, db) })

	// WEBSOCKET CHAT
	r.GET("/ws", websocket.HandleWebSocket(chatHub))

	// PROTECTED
	authed := r.Group("/")
	authed.Use(auth.RequireJWT(jwtSecret))
	authed.POST("/library", func(c *gin.Context) { handleAddLibrary(c, db) })
	authed.PATCH("/progress", func(c *gin.Context) { handleUpdateProgress(c, db, progressCh) })
	authed.POST("/admin/notify", func(c *gin.Context) {
		var req struct {
			Message string `json:"message"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" {
			c.JSON(400, gin.H{"error": "message required"})
			return
		}
		udpServer.Broadcast(req.Message)
		c.JSON(200, gin.H{"ok": true})
	})

	log.Println("HTTP API listening on :8080")
	log.Fatal(r.Run(":8080"))
}

func handleRegister(c *gin.Context, db *sql.DB) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username/password required"})
		return
	}

	// Bonus: Sanitize username input
	sanitizedUsername, err := sanitizeUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Bonus: Validate password length
	if len(req.Password) < 6 || len(req.Password) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be 6-100 characters"})
		return
	}

	// id đơn giản demo: dùng username làm id (sau này có thể đổi sang uuid)
	if err := user.CreateUser(db, sanitizedUsername, sanitizedUsername, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

func handleLogin(c *gin.Context, db *sql.DB) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	// Bonus: Sanitize username input
	sanitizedUsername, err := sanitizeUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	u, err := user.VerifyLogin(db, sanitizedUsername, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := auth.SignJWT(jwtSecret, u.ID, u.Username, 24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sign token failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

func handleSearchManga(c *gin.Context, db *sql.DB) {
	q := c.Query("q")
	genre := c.Query("genre")
	status := c.Query("status")
	sortBy := c.Query("sort_by") // Bonus: Add sort_by parameter
	limit := parseInt(c.Query("limit"), 20)
	offset := parseInt(c.Query("offset"), 0)

	// Bonus: Sanitize search query
	if q != "" {
		q = sanitizeSearchQuery(q)
	}
	// Bonus: Sanitize genre
	if genre != "" {
		genre = sanitizeSearchQuery(genre)
	}

	// Bonus: Validate and sanitize sortBy
	validSortOptions := []string{"title_asc", "title_desc", "author_asc", "author_desc", "chapters_asc", "chapters_desc"}
	sortByValid := false
	for _, valid := range validSortOptions {
		if sortBy == valid {
			sortByValid = true
			break
		}
	}
	if sortBy != "" && !sortByValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sort_by, options: title_asc, title_desc, author_asc, author_desc, chapters_asc, chapters_desc"})
		return
	}

	// Bonus: Use advanced search with sorting
	res, err := manga.AdvancedSearch(db, q, genre, status, sortBy, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": res, "limit": limit, "offset": offset, "sort_by": sortBy})
}

func handleMangaDetail(c *gin.Context, db *sql.DB) {
	id := c.Param("id")
	// Bonus: Sanitize manga ID
	sanitizedID, err := sanitizeMangaID(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m, err := manga.GetByID(db, sanitizedID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}

	// Nếu có JWT thì trả kèm progress (không bắt buộc, nhưng đúng hướng use-case)
	userIDAny, ok := c.Get(auth.CtxUserIDKey)
	if ok {
		if p, err := library.GetProgress(db, userIDAny.(string), id); err == nil {
			c.JSON(http.StatusOK, gin.H{"manga": m, "progress": p})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"manga": m})
}

func handleAddLibrary(c *gin.Context, db *sql.DB) {
	var req struct {
		MangaID        string `json:"manga_id"`
		Status         string `json:"status"`
		CurrentChapter int    `json:"current_chapter"`
		ListName       string `json:"list_name"` // Bonus: Multiple reading lists
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.MangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id required"})
		return
	}
	userID := c.GetString(auth.CtxUserIDKey)

	// Bonus: Sanitize manga ID
	sanitizedMangaID, err := sanitizeMangaID(req.MangaID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Bonus: Validate status
	validatedStatus, err := validateStatus(req.Status)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Bonus: Validate chapter number
	if req.CurrentChapter < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter number cannot be negative"})
		return
	}

	if _, err := manga.GetByID(db, sanitizedMangaID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}

	// Bonus: Use list_name from request, default to "default" if empty
	listName := req.ListName
	if listName == "" {
		listName = "default"
	}

	if err := library.UpsertProgress(db, library.Progress{
		UserID: userID, MangaID: sanitizedMangaID, CurrentChapter: req.CurrentChapter, Status: validatedStatus, ListName: listName,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleUpdateProgress(c *gin.Context, db *sql.DB, progressCh chan<- models.ProgressUpdate) {
	var req struct {
		MangaID        string `json:"manga_id"`
		CurrentChapter int    `json:"current_chapter"`
		Status         string `json:"status"`
		ListName       string `json:"list_name"` // Bonus: Multiple reading lists
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.MangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id required"})
		return
	}
	userID := c.GetString(auth.CtxUserIDKey)

	// Bonus: Sanitize manga ID
	sanitizedMangaID, err := sanitizeMangaID(req.MangaID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Bonus: Validate status if provided
	validatedStatus := req.Status
	if req.Status != "" {
		validatedStatus, err = validateStatus(req.Status)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	m, err := manga.GetByID(db, sanitizedMangaID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}

	// Validate chapter number
	if req.CurrentChapter < 0 || (m.TotalChapters > 0 && req.CurrentChapter > m.TotalChapters) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}

	// Bonus: Use list_name from request, default to "default" if empty
	listName := req.ListName
	if listName == "" {
		listName = "default"
	}

	// Save progress to database
	if err := library.UpsertProgress(db, library.Progress{
		UserID: userID, MangaID: sanitizedMangaID, CurrentChapter: req.CurrentChapter, Status: validatedStatus, ListName: listName,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	evt := models.ProgressUpdate{
		UserID:    userID,
		MangaID:   sanitizedMangaID,
		Chapter:   req.CurrentChapter,
		Timestamp: time.Now().Unix(),
	}

	// tránh block nếu channel đầy
	select {
	case progressCh <- evt:
	default:
		log.Println("warn: progress channel full, drop event")
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Bonus: Health Check endpoint - checks all service statuses
func handleHealthCheck(c *gin.Context, db *sql.DB, tcpServer *tcpsync.Server, udpServer *udpnotify.Server, grpcServer *grpc.Server, chatHub *websocket.ChatHub) {
	status := gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"services":  gin.H{},
	}

	allHealthy := true
	services := status["services"].(gin.H)

	// Check database
	if err := db.Ping(); err != nil {
		services["database"] = gin.H{"status": "unhealthy", "error": err.Error()}
		allHealthy = false
	} else {
		services["database"] = gin.H{"status": "healthy"}
	}

	// Check TCP server - try to connect to port
	tcpHealthy := checkTCPHealth(":9090")
	services["tcp"] = gin.H{"status": map[bool]string{true: "healthy", false: "unhealthy"}[tcpHealthy]}
	if !tcpHealthy {
		allHealthy = false
	}

	// Check UDP server - check if server is initialized
	if udpServer != nil {
		services["udp"] = gin.H{"status": "healthy"}
	} else {
		services["udp"] = gin.H{"status": "unhealthy"}
		allHealthy = false
	}

	// Check gRPC server - try to connect to port
	grpcHealthy := checkTCPHealth(":50051")
	services["grpc"] = gin.H{"status": map[bool]string{true: "healthy", false: "unhealthy"}[grpcHealthy]}
	if !grpcHealthy {
		allHealthy = false
	}

	// Check WebSocket hub - check if hub is initialized
	if chatHub != nil {
		services["websocket"] = gin.H{"status": "healthy"}
	} else {
		services["websocket"] = gin.H{"status": "unhealthy"}
		allHealthy = false
	}

	if !allHealthy {
		status["status"] = "degraded"
		c.JSON(http.StatusOK, status)
		return
	}

	c.JSON(http.StatusOK, status)
}

// Bonus: Helper function to check TCP port health
func checkTCPHealth(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Bonus: Input sanitization functions
func sanitizeUsername(username string) (string, error) {
	// Remove whitespace
	username = strings.TrimSpace(username)
	// Validate length
	if len(username) < 3 || len(username) > 20 {
		return "", fmt.Errorf("username must be 3-20 characters")
	}
	// Only allow alphanumeric and underscore
	for _, r := range username {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return "", fmt.Errorf("username can only contain letters, numbers, and underscores")
		}
	}
	return username, nil
}

// Bonus: Sanitize search query to prevent SQL injection patterns
func sanitizeSearchQuery(query string) string {
	query = strings.TrimSpace(query)
	// Remove potentially dangerous characters
	query = strings.ReplaceAll(query, ";", "")
	query = strings.ReplaceAll(query, "--", "")
	query = strings.ReplaceAll(query, "/*", "")
	query = strings.ReplaceAll(query, "*/", "")
	query = strings.ReplaceAll(query, "xp_", "")
	query = strings.ReplaceAll(query, "exec", "")
	query = strings.ReplaceAll(query, "union", "")
	// Limit length
	if len(query) > 100 {
		query = query[:100]
	}
	return query
}

// Bonus: Sanitize manga ID (alphanumeric only)
func sanitizeMangaID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("manga ID cannot be empty")
	}
	if len(id) > 50 {
		return "", fmt.Errorf("manga ID too long")
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return "", fmt.Errorf("invalid manga ID format")
		}
	}
	return id, nil
}

// Bonus: Validate status value
func validateStatus(status string) (string, error) {
	validStatuses := []string{"plan-to-read", "reading", "completed", "on-hold", "dropped"}
	status = strings.TrimSpace(strings.ToLower(status))
	for _, valid := range validStatuses {
		if status == valid {
			return status, nil
		}
	}
	return "", fmt.Errorf("invalid status, must be one of: %v", validStatuses)
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
