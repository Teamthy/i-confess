package api

import (
	"net/http"

	"github.com/Teamthy/i-confess/internal/httpx"
)

// serviceInfo answers GET / with what this server is and where to look next,
// so someone who opens the API origin in a browser gets a pointer rather than
// a page from a retired UI.
func serviceInfo(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"service": "I CONFESS API",
		"openapi": "/openapi.json",
		"health":  "/health/ready",
	})
}

// notFoundJSON is the API's answer for any GET path no route claims.
func notFoundJSON(w http.ResponseWriter, _ *http.Request) {
	writeCode(w, http.StatusNotFound, "NOT_FOUND", "no such endpoint")
}
