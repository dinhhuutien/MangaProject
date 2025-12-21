package grpc

import (
	"context"
	"database/sql"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mangahub/internal/library"
	"mangahub/internal/manga"
	"mangahub/proto"
)

// Server implementation
type Server struct {
	proto.UnimplementedMangaServiceServer
	db *sql.DB
}

// Tạo server ở gRPC server
func NewServer(db *sql.DB) *Server {
	return &Server{
		db: db,
	}
}

// GetManga implementation
func (s *Server) GetManga(ctx context.Context, req *proto.GetMangaRequest) (*proto.MangaResponse, error) {

	// Get manga from db
	m, err := manga.GetByID(s.db, req.Id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "manga not found: %v", req.Id)
		}
		return nil, status.Errorf(codes.Internal, "failed to get manga: %v", err)
	}

	// Parse genres
	genres := parseGenres(m.Genres)

	// Convert to proto
	return &proto.MangaResponse{
		Id:            m.ID,
		Title:         m.Title,
		Author:        m.Author,
		Genres:        genres,
		Status:        m.Status,
		TotalChapters: int32(m.TotalChapters),
		Description:   m.Description,
	}, nil
}

// Search manga implementation
func (s *Server) SearchManga(ctx context.Context, req *proto.SearchRequest) (*proto.SearchResponse, error) {
	// Cai dat mac dinnh
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	offset := int(req.Offset)

	//search manga
	results, err := manga.Search(s.db, req.Query, req.Genre, req.Status, limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to search manga: %v", err)
	}

	// Convert to proto
	protoResults := make([]*proto.MangaResponse, 0, len(results))
	for _, m := range results {
		genres := parseGenres(m.Genres)
		protoResults = append(protoResults, &proto.MangaResponse{
			Id:            m.ID,
			Title:         m.Title,
			Author:        m.Author,
			Genres:        genres,
			Status:        m.Status,
			TotalChapters: int32(m.TotalChapters),
			Description:   m.Description,
		})
	}

	return &proto.SearchResponse{
		Results: protoResults,
		Limit:   int32(limit),
		Offset:  int32(offset),
	}, nil
}

// UpdateProgress implementation
func (s *Server) UpdateProgress(ctx context.Context, req *proto.ProgressRequest) (*proto.ProgressResponse, error) {
	// Update progress in database
	err := library.UpsertProgress(s.db, library.Progress{
		UserID:         req.UserId,
		MangaID:        req.MangaId,
		CurrentChapter: int(req.CurrentChapter),
		Status:         req.Status,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update progress: %v", err)
	}

	return &proto.ProgressResponse{
		Success: true,
		Message: "Progress updated successfully",
	}, nil
}

// Parse genres method
func parseGenres(genresJSON string) []string {
	if genresJSON == "" {
		return []string{}
	}

	// remove brackets []
	if len(genresJSON) < 2 {
		return []string{}
	}
	genresJSON = genresJSON[1 : len(genresJSON)-1]
	if genresJSON == "" {
		return []string{}
	}

	var genres []string
	parts := strings.Split(genresJSON, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, `"`) // remove quotes
		if part != "" {
			genres = append(genres, part)
		}
	}
	return genres
}
