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

	if _, ok := errors.AsType[*InvalidShortUrl](err); ok {
		http.Error(w, "short url is invalid", http.StatusBadRequest)
		return
	} else if _, ok := errors.AsType[*ShortUrlNotFound](err); ok {
		http.Error(w, "short url not found", http.StatusNotFound)
		return
	}

	ran := HttpErrorHandler(w, err, logger)
	if ran {
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
	}

	ran := HttpErrorHandler(w, err, logger)
	if ran {
		return
	}

	resp := &ShortUrl{ShortUrl: shortUrl}
	b, err := json.Marshal(resp)
	if err != nil {
		logger.Error("Internal server error", "error", err.Error())
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, err = w.Write(b)
	if err != nil {
		logger.Warn("Internal server error", "error", err.Error())
		return
	}
}
