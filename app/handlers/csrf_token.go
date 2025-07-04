package handlers

import (
	"encoding/json"
	"net/http"
	// "log"
	"github.com/gorilla/csrf"
)

func GetCSRFToken(w http.ResponseWriter, r *http.Request) {
	token := csrf.Token(r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"csrfToken": token,
	})
}

// CSRFFailureHandler handles CSRF validation failures
func CSRFFailureHandler(w http.ResponseWriter, r *http.Request) {
	// log.Printf("CSRF failure: token=%s, method=%s, origin=%s, referer=%s",
	// 	r.Header.Get("X-CSRF-Token"),
	// 	r.Method,
	// 	r.Header.Get("Origin"),
	// 	r.Header.Get("Referer"))

	respondWithError(w, http.StatusForbidden, "Invalid CSRF token")
}
