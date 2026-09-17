package internal

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

type LongUrl struct {
	LongUrl string `json:"longUrl"`
}

type ShortUrl struct {
	ShortUrl string `json:"shortUrl"`
}

func RequestHandler(w http.ResponseWriter, r *http.Request, s *UrlShortenerService) {
	switch r.Method {

	// Retrieve long URL from short one
	case http.MethodGet:
		shortUrl := strings.TrimPrefix(r.URL.Path, "/")
		if len(shortUrl) == 0 {
			http.Error(w, "You must provide a short URL in the URL path", http.StatusBadRequest)
			return
		}
		longUrl, err := s.GetLongUrl(shortUrl)
		if err != nil {
			log.Printf("Internal server error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		resp := &LongUrl{LongUrl: longUrl}
		b, err := json.Marshal(resp)
		if err != nil {
			log.Printf("Internal server error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		w.Write(b)

	// Create a new short URL
	case http.MethodPost:
		var body LongUrl
		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			http.Error(w, "Request body must include 'longUrl' string field.", http.StatusBadRequest)
			return
		}

		shortUrl, err := s.ShortenUrl(body.LongUrl)
		if err != nil {
			log.Printf("Internal server error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		resp := &ShortUrl{ShortUrl: shortUrl}
		b, err := json.Marshal(resp)
		if err != nil {
			log.Printf("Internal server error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(b)

	default:
		w.Header().Add("Allow", "GET, POST")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
