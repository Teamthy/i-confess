package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// EncodeJSON writes v as JSON without setting an HTTP status.
func EncodeJSON(w http.ResponseWriter, v any) error {
	return json.NewEncoder(w).Encode(v)
}

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = EncodeJSON(w, v)
}

// WriteError writes a consistent error envelope.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// DecodeJSON decodes the request body, enforcing a size limit and single value.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Ensure there is no trailing data.
	if dec.More() {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}
