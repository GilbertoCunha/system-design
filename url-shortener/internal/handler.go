package internal

import (
	"fmt"
	"net/http"
)

func RequestHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		fmt.Fprintf(w, "This is a GET method!")
	case http.MethodPost:
		fmt.Fprintf(w, "This is a POST method!")
	}
}
