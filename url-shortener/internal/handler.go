package internal

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type LongUrl struct {
	LongUrl string `json:"longUrl"`
}

type ShortUrl struct {
	ShortUrl string `json:"shortUrl"`
}

func GetLongUrlHandler(w http.ResponseWriter, r *http.Request, s UrlShortenerService, logger *slog.Logger) {
	shortUrl := r.PathValue("code")
	longUrl, err := s.GetLongUrl(r.Context(), shortUrl)

	_, invalidShortUrlOk := errors.AsType[*InvalidShortUrl](err)
	_, shortUrlNotFoundOk := errors.AsType[*ShortUrlNotFound](err)
	if invalidShortUrlOk {
		http.Error(w, "short url is invalid", http.StatusBadRequest)
		return
	} else if shortUrlNotFoundOk {
		http.Error(w, "short url not found", http.StatusNotFound)
		return
	} else if err != nil {
		logger.Warn("Internal server error", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// TODO: Change to temporary redirect for click rate
	http.Redirect(w, r, longUrl, http.StatusFound)
}

func CreateShortUrlHandler(w http.ResponseWriter, r *http.Request, s UrlShortenerService, logger *slog.Logger) {
	var body LongUrl
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		http.Error(w, "Request body must include 'longUrl' string field.", http.StatusBadRequest)
		return
	}

	shortUrl, err := s.ShortenUrl(r.Context(), body.LongUrl)
	if _, ok := errors.AsType[*InvalidUrl](err); ok {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	} else if err != nil {
		logger.Warn("Internal server error", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	resp := &ShortUrl{ShortUrl: shortUrl}
	b, err := json.Marshal(resp)
	if err != nil {
		logger.Warn("Internal server error", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, err = w.Write(b)
	if err != nil {
		logger.Warn("Internal server error", "err", err)
		return
	}
}
