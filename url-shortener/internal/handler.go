package internal

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type LongUrl struct {
	LongUrl string `json:"longUrl"`
}

type ShortUrl struct {
	ShortUrl string `json:"shortUrl"`
}

func GetLongUrlHandler(w http.ResponseWriter, r *http.Request, s UrlShortenerService, shortUrl string) {
	longUrl, err := s.GetLongUrl(shortUrl)

	if _, ok := errors.AsType[*InvalidUrlError](err); ok {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	} else if err != nil {
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
}

func CreateShortUrlHandler(w http.ResponseWriter, r *http.Request, s UrlShortenerService) {
	var body LongUrl
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		http.Error(w, "Request body must include 'longUrl' string field.", http.StatusBadRequest)
		return
	}

	shortUrl, err := s.ShortenUrl(body.LongUrl)
	invalidShortUrlError, invalidShortUrlOk := errors.AsType[*InvalidShortUrl](err)
	shortUrlNotFoundError, shortUrlNotFoundOk := errors.AsType[*ShortUrlNotFound](err)
	if invalidShortUrlOk {
		http.Error(w, invalidShortUrlError.Error(), http.StatusBadRequest)
		return
	} else if shortUrlNotFoundOk {
		http.Error(w, shortUrlNotFoundError.Error(), http.StatusBadRequest)
	} else if err != nil {
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
}
