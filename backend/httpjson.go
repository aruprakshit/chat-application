package main

import (
	"mime"
	"net/http"
)

// requireJSONContentType rejects requests that are not application/json.
// It writes the error response itself; the caller should return on false.
func requireJSONContentType(w http.ResponseWriter, r *http.Request) bool {
	// ParseMediaType also accepts application/json; charset=utf-8.
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		http.Error(w, "Content-Type must be application/json",
			http.StatusUnsupportedMediaType)
		return false
	}
	return true
}
