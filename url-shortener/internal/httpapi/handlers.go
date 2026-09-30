package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/shortener"
)

type LongUrl struct {
	LongUrl string `json:"longUrl"`
}

type ShortUrl struct {
	ShortUrl string `json:"shortUrl"`
}

type handlers struct {
	svc *shortener.Service
	log *slog.Logger
}

func (h *handlers) getLongUrl(w http.ResponseWriter, r *http.Request) {
	longUrl, err := h.svc.GetLongUrl(r.Context(), r.PathValue("code"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	// TODO: Change to temporary redirect for click rate
	http.Redirect(w, r, longUrl, http.StatusFound)
}

func (h *handlers) createShortUrl(w http.ResponseWriter, r *http.Request) {
	var body LongUrl
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Request body must include 'longUrl' string field.", http.StatusBadRequest)
		return
	}

	shortUrl, err := h.svc.ShortenUrl(r.Context(), body.LongUrl)
	if err != nil {
		h.writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(ShortUrl{ShortUrl: shortUrl}); err != nil {
		h.log.Warn("writing response failed", "error", err.Error())
	}
}

// writeError answers with the status an error from the service maps to
func (h *handlers) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, shortener.ErrInvalidCode):
		http.Error(w, "short url is invalid", http.StatusBadRequest)
	case errors.Is(err, shortener.ErrInvalidUrl):
		http.Error(w, "Invalid URL", http.StatusBadRequest)
	case errors.Is(err, shortener.ErrNotFound):
		http.Error(w, "short url not found", http.StatusNotFound)
	case errors.Is(err, context.Canceled):
		// The client went away; nobody reads this, but the metrics count it
		w.WriteHeader(499)
	default:
		if o, ok := errors.AsType[*shortener.Overloaded](err); ok {
			h.log.Error("dependency timeout", shortener.ErrorAttrs(err)...)
			setUnavailableCause(w, o.Cause())
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		h.log.Error("internal server error", "error", err.Error())
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
