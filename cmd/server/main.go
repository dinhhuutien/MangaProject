package main

import (
	"database/sql"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
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
	tcpServer := tcpsync.New(":9090", progressCh)
	go func() {
		if err := tcpServer.Start(); err != nil {
			log.Fatal(err)
		}
	}()
	udpServer := udpnotify.New(":7070")
	go func() {
		if err := udpServer.Start(); err != nil {
			log.Fatal(err)
		}
	}()

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
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

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

	// id đơn giản demo: dùng username làm id (sau này có thể đổi sang uuid)
	if err := user.CreateUser(db, req.Username, req.Username, req.Password); err != nil {
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

	u, err := user.VerifyLogin(db, req.Username, req.Password)
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
	limit := parseInt(c.Query("limit"), 20)
	offset := parseInt(c.Query("offset"), 0)

	res, err := manga.Search(db, q, genre, status, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": res, "limit": limit, "offset": offset})
}

func handleMangaDetail(c *gin.Context, db *sql.DB) {
	id := c.Param("id")
	m, err := manga.GetByID(db, id)
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
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.MangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id required"})
		return
	}
	userID := c.GetString(auth.CtxUserIDKey)

	if _, err := manga.GetByID(db, req.MangaID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}

	if err := library.UpsertProgress(db, library.Progress{
		UserID: userID, MangaID: req.MangaID, CurrentChapter: req.CurrentChapter, Status: req.Status,
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
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.MangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id required"})
		return
	}
	userID := c.GetString(auth.CtxUserIDKey)

	m, err := manga.GetByID(db, req.MangaID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}

	// Validate chapter number
	if req.CurrentChapter < 0 || (m.TotalChapters > 0 && req.CurrentChapter > m.TotalChapters) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter number"})
		return
	}

	evt := models.ProgressUpdate{
		UserID:    userID,
		MangaID:   req.MangaID,
		Chapter:   req.CurrentChapter,
		Timestamp: time.Now().Unix(),
	}

	// tránh block nếu channel đầy
	select {
	case progressCh <- evt:
	default:
		log.Println("warn: progress channel full, drop event")
	}

	// Day 3: thay log này bằng push event -> TCP broadcast
	log.Printf("[TODO TCP BROADCAST] user=%s manga=%s chapter=%d", userID, req.MangaID, req.CurrentChapter)

	c.JSON(http.StatusOK, gin.H{"ok": true})
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
