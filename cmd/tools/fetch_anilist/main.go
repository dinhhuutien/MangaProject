package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"unicode"

	"mangahub/pkg/models"
)

const endpoint = "https://graphql.anilist.co"

var tagRe = regexp.MustCompile(`<[^>]+>`)

type gqlReq struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type gqlResp struct {
	Data struct {
		Page struct {
			Media []struct {
				Title struct {
					Romaji  string `json:"romaji"`
					English string `json:"english"`
					Native  string `json:"native"`
				} `json:"title"`
				Genres      []string `json:"genres"`
				Status      string   `json:"status"`
				Chapters    *int     `json:"chapters"`
				Description *string  `json:"description"`
				Staff       struct {
					Nodes []struct {
						Name struct {
							Full string `json:"full"`
						} `json:"name"`
					} `json:"nodes"`
				} `json:"staff"`
			} `json:"media"`
		} `json:"Page"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "manga"
	}
	return out
}

func pickTitle(tRomaji, tEnglish, tNative string) string {
	if strings.TrimSpace(tEnglish) != "" {
		return tEnglish
	}
	if strings.TrimSpace(tRomaji) != "" {
		return tRomaji
	}
	return tNative
}

func mapStatus(anilist string) string {
	switch anilist {
	case "FINISHED":
		return "completed"
	case "RELEASING":
		return "ongoing"
	case "HIATUS":
		return "hiatus"
	case "CANCELLED":
		return "cancelled"
	default:
		return "unknown"
	}
}

func cleanDesc(s string) string {
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	// giới hạn cho gọn DB/demo
	if len(s) > 500 {
		s = s[:500] + "..."
	}
	return s
}

func main() {
	outPath := flag.String("out", "data/manga.json", "output json path")
	n := flag.Int("n", 40, "number of manga to fetch")
	page := flag.Int("page", 1, "page number")
	flag.Parse()

	// Lấy “popular” cho dễ demo (bạn có thể đổi sort nếu muốn)
	query := `
query ($page: Int, $perPage: Int) {
  Page(page: $page, perPage: $perPage) {
    media(type: MANGA, sort: POPULARITY_DESC) {
      title { romaji english native }
      genres
      status
      chapters
      description
      staff(perPage: 1) {
        nodes { name { full } }
      }
    }
  }
}`

	reqBody := gqlReq{
		Query: query,
		Variables: map[string]any{
			"page":    *page,
			"perPage": *n,
		},
	}

	b, _ := json.Marshal(reqBody)
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(b))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		fmt.Println(string(raw))
		panic(fmt.Errorf("anilist http status: %s", resp.Status))
	}

	var parsed gqlResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		panic(err)
	}
	if len(parsed.Errors) > 0 {
		fmt.Println(string(raw))
		panic(fmt.Errorf("anilist gql error: %s", parsed.Errors[0].Message))
	}

	out := make([]models.Manga, 0, len(parsed.Data.Page.Media))
	used := map[string]int{}

	for _, m := range parsed.Data.Page.Media {
		title := pickTitle(m.Title.Romaji, m.Title.English, m.Title.Native)

		id := slugify(title)
		used[id]++
		if used[id] > 1 {
			id = fmt.Sprintf("%s-%d", id, used[id])
		}

		author := "Unknown"
		if len(m.Staff.Nodes) > 0 && strings.TrimSpace(m.Staff.Nodes[0].Name.Full) != "" {
			author = m.Staff.Nodes[0].Name.Full
		}

		total := 0
		if m.Chapters != nil {
			total = *m.Chapters
		}

		desc := ""
		if m.Description != nil {
			desc = cleanDesc(*m.Description)
		}

		out = append(out, models.Manga{
			ID:            id,
			Title:         title,
			Author:        author,
			Genres:        m.Genres,
			Status:        mapStatus(m.Status),
			TotalChapters: total,
			Description:   desc,
		})
	}

	// đảm bảo folder tồn tại
	_ = os.MkdirAll(strings.TrimSuffix(*outPath, "/manga.json"), 0755)

	j, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(*outPath, j, 0644); err != nil {
		panic(err)
	}

	fmt.Printf("Wrote %d manga -> %s\n", len(out), *outPath)
}
